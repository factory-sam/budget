package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type EquityCSVImporter struct {
	svc       *service.Service
	portfolio *equity.PortfolioService
}

func NewEquityCSV(svc *service.Service, portfolio *equity.PortfolioService) *EquityCSVImporter {
	return &EquityCSVImporter{svc: svc, portfolio: portfolio}
}

type EquityImportResult struct {
	Purchases int
	Sales     int
	Dividends int
	Skipped   int
	Errors    []string
}

func (r EquityImportResult) String() string {
	return fmt.Sprintf("Imported: %d purchases, %d sales, %d dividends (%d skipped)",
		r.Purchases, r.Sales, r.Dividends, r.Skipped)
}

func (e *EquityCSVImporter) Import(path string, accountID *int64) (*EquityImportResult, error) {
	hash, err := fileHash(path)
	if err != nil {
		return nil, err
	}
	var existing int
	e.svc.DB().QueryRow("SELECT COUNT(*) FROM import_records WHERE hash = ?", hash).Scan(&existing)
	if existing > 0 {
		return nil, fmt.Errorf("file already imported (hash: %s)", hash[:12])
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, headerIdx, err := e.parseRecords(f)
	if err != nil {
		return nil, err
	}

	cols := e.detectEquityCols(records[headerIdx])
	if cols.date < 0 || cols.symbol < 0 {
		return nil, fmt.Errorf("could not detect required columns (need Trade Date and Symbol)")
	}

	// Sort data rows chronologically (CSVs are often newest-first)
	dataRows := records[headerIdx+1:]
	if len(dataRows) > 1 && cols.date >= 0 {
		// Find first and last rows with actual dates (skip blank rows)
		var firstDate, lastDate string
		for _, row := range dataRows {
			if cols.date < len(row) {
				d := normalizeDate(strings.TrimSpace(row[cols.date]))
				if isValidDate(d) {
					firstDate = d
					break
				}
			}
		}
		for i := len(dataRows) - 1; i >= 0; i-- {
			if cols.date < len(dataRows[i]) {
				d := normalizeDate(strings.TrimSpace(dataRows[i][cols.date]))
				if isValidDate(d) {
					lastDate = d
					break
				}
			}
		}
		if firstDate > lastDate {
			for left, right := 0, len(dataRows)-1; left < right; left, right = left+1, right-1 {
				dataRows[left], dataRows[right] = dataRows[right], dataRows[left]
			}
		}
	}

	result := &EquityImportResult{}

	for i := 0; i < len(dataRows); i++ {
		row := dataRows[i]
		if len(row) <= cols.date {
			continue
		}

		date := normalizeDate(strings.TrimSpace(row[cols.date]))
		if date == "" || !isValidDate(date) {
			result.Skipped++
			continue
		}

		desc := ""
		if cols.description >= 0 && cols.description < len(row) {
			desc = strings.TrimSpace(row[cols.description])
		}

		symbol := ""
		if cols.symbol >= 0 && cols.symbol < len(row) {
			symbol = strings.ToUpper(strings.TrimSpace(row[cols.symbol]))
		}

		qtyStr := ""
		if cols.quantity >= 0 && cols.quantity < len(row) {
			qtyStr = strings.TrimSpace(row[cols.quantity])
		}

		priceStr := ""
		if cols.price >= 0 && cols.price < len(row) {
			priceStr = strings.TrimSpace(row[cols.price])
		}

		amountStr := ""
		if cols.amount >= 0 && cols.amount < len(row) {
			amountStr = strings.TrimSpace(row[cols.amount])
		}

		txType := classifyTransaction(desc)

		switch txType {
		case "purchase":
			if symbol == "" || qtyStr == "" || priceStr == "" {
				result.Skipped++
				continue
			}
			qty := parseNumber(qtyStr)
			price := parseNumber(priceStr)
			if qty <= 0 || price <= 0 {
				result.Skipped++
				continue
			}
			priceCents := int64(math.Round(price * 100))
			note := truncateDesc(desc)
			_, err := e.portfolio.BuyLot(accountID, symbol, qty, priceCents, date, note)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", i+1, err))
				continue
			}
			result.Purchases++

		case "transfer_in":
			if symbol == "" || qtyStr == "" {
				result.Skipped++
				continue
			}
			qty := parseNumber(qtyStr)
			if qty <= 0 {
				result.Skipped++
				continue
			}
			// Use the price if available, otherwise cost basis = 0 (unknown)
			price := parseNumber(priceStr)
			priceCents := int64(math.Round(price * 100))
			note := "Transfer in"
			_, err := e.portfolio.BuyLot(accountID, symbol, qty, priceCents, date, note)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", i+1, err))
				continue
			}
			result.Purchases++

		case "sale":
			if symbol == "" || qtyStr == "" {
				result.Skipped++
				continue
			}
			qty := math.Abs(parseNumber(qtyStr))
			if qty <= 0 {
				result.Skipped++
				continue
			}
			lots, _ := e.portfolio.ListLots(symbol, accountID)
			sold := 0.0
			for _, lot := range lots {
				if sold >= qty {
					break
				}
				toSell := math.Min(lot.Shares, qty-sold)
				e.portfolio.SellLot(lot.ID, toSell)
				sold += toSell
			}
			result.Sales++

		case "dividend":
			if symbol == "" {
				result.Skipped++
				continue
			}
			amount := parseNumber(amountStr)
			if amount == 0 {
				result.Skipped++
				continue
			}
			cents := int64(math.Round(math.Abs(amount) * 100))
			catID := e.svc.AutoCategorize("Dividend " + symbol)
			// find an account to record against; use the linked account or first available
			var txAccountID int64
			if accountID != nil {
				txAccountID = *accountID
			} else {
				accs, _ := e.svc.ListAccounts()
				if len(accs) > 0 {
					txAccountID = accs[0].ID
				}
			}
			if txAccountID > 0 {
				tx := transactionForDividend(txAccountID, catID, cents, date, symbol, desc)
				// Insert directly without updating account balance — dividend income
				// from brokerage CSVs is already reflected in portfolio lot values
				e.svc.DB().Exec(
					"INSERT INTO transactions (account_id, category_id, amount, date, payee, note, type) VALUES (?, ?, ?, ?, ?, ?, ?)",
					tx.AccountID, tx.CategoryID, tx.Amount, tx.Date, tx.Payee, tx.Note, tx.Type)
			}
			result.Dividends++

		default:
			result.Skipped++
		}
	}

	total := result.Purchases + result.Sales + result.Dividends
	e.svc.DB().Exec("INSERT INTO import_records (filename, hash, tx_count) VALUES (?, ?, ?)", path, hash, total)
	return result, nil
}

type equityColMap struct {
	date        int
	settlement  int
	account     int
	description int
	txType      int
	symbol      int
	quantity    int
	price       int
	amount      int
}

func (e *EquityCSVImporter) detectEquityCols(header []string) equityColMap {
	m := equityColMap{
		date: -1, settlement: -1, account: -1, description: -1,
		txType: -1, symbol: -1, quantity: -1, price: -1, amount: -1,
	}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		switch {
		case h == "trade date" || (strings.Contains(h, "trade") && strings.Contains(h, "date")):
			m.date = i
		case h == "settlement date" || strings.Contains(h, "settle"):
			m.settlement = i
		case h == "account":
			m.account = i
		case h == "description" || h == "desc":
			m.description = i
		case h == "type" || h == "action" || h == "trans type":
			m.txType = i
		case strings.Contains(h, "symbol") || strings.Contains(h, "cusip") || h == "ticker":
			m.symbol = i
		case strings.Contains(h, "quantity") || h == "qty" || h == "shares":
			m.quantity = i
		case strings.Contains(h, "price"):
			m.price = i
		case strings.Contains(h, "amount") || h == "net amount" || h == "total":
			m.amount = i
		}
	}
	return m
}

func (e *EquityCSVImporter) parseRecords(r io.Reader) ([][]string, int, error) {
	// Read raw content and clean up malformed quoting (e.g. `"value" ,` → `"value",`)
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, -1, err
	}
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\" ,", "\",")
	// Remove trailing empty quoted column (e.g. `,"" ` at end of lines)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Strip trailing ,"" or ," " columns
		for strings.HasSuffix(strings.TrimSpace(line), ",\"\"") || strings.HasSuffix(strings.TrimSpace(line), ",\" \"") {
			line = strings.TrimSpace(line)
			lastComma := strings.LastIndex(line, ",")
			if lastComma > 0 {
				line = line[:lastComma]
			}
		}
		lines[i] = line
	}
	content = strings.Join(lines, "\n")

	reader := csv.NewReader(strings.NewReader(content))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	var allRecords [][]string
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		allRecords = append(allRecords, rec)
	}

	// find the header row (the row containing "Trade Date" or "Symbol")
	headerIdx := -1
	for i, rec := range allRecords {
		for _, field := range rec {
			f := strings.ToLower(strings.TrimSpace(field))
			if strings.Contains(f, "trade date") || strings.Contains(f, "symbol") || strings.Contains(f, "ticker") {
				headerIdx = i
				break
			}
		}
		if headerIdx >= 0 {
			break
		}
	}
	if headerIdx < 0 {
		// fallback: try first row as header
		if len(allRecords) > 0 {
			headerIdx = 0
		} else {
			return nil, -1, fmt.Errorf("no data found in CSV")
		}
	}

	return allRecords, headerIdx, nil
}

func classifyTransaction(desc string) string {
	d := strings.ToLower(desc)
	switch {
	case strings.HasPrefix(d, "purchase"):
		return "purchase"
	case strings.HasPrefix(d, "sale"):
		return "sale"
	case strings.HasPrefix(d, "dividend"):
		return "dividend"
	case strings.HasPrefix(d, "security transfer in"):
		return "transfer_in"
	case strings.Contains(d, "funds received"):
		return "deposit"
	case strings.Contains(d, "advisory program fee") || strings.Contains(d, "advisory fee"):
		return "fee"
	case strings.Contains(d, "bank interest"):
		return "interest"
	default:
		return "other"
	}
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func isValidDate(s string) bool {
	if len(s) < 8 {
		return false
	}
	// should be YYYY-MM-DD after normalization
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return true
	}
	return false
}

func truncateDesc(desc string) string {
	if len(desc) > 80 {
		return desc[:77] + "..."
	}
	return desc
}

func transactionForDividend(accountID int64, catID *int64, cents int64, date, symbol, desc string) model.Transaction {
	return model.Transaction{
		AccountID:  accountID,
		CategoryID: catID,
		Amount:     cents,
		Date:       date,
		Payee:      fmt.Sprintf("Dividend — %s", symbol),
		Note:       truncateDesc(desc),
		Type:       "income",
	}
}
