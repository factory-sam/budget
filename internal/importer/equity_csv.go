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

const txTypeFee = "fee"

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

// rowFields holds extracted and cleaned fields from a single CSV row.
type rowFields struct {
	date      string
	desc      string
	symbol    string
	qtyStr    string
	priceStr  string
	amountStr string
}

func extractRowFields(row []string, cols equityColMap) rowFields {
	rf := rowFields{}
	if cols.date >= 0 && cols.date < len(row) {
		rf.date = normalizeDate(strings.TrimSpace(row[cols.date]))
	}
	if cols.description >= 0 && cols.description < len(row) {
		rf.desc = strings.TrimSpace(row[cols.description])
	}
	if cols.symbol >= 0 && cols.symbol < len(row) {
		rf.symbol = strings.ToUpper(strings.TrimSpace(row[cols.symbol]))
	}
	if cols.quantity >= 0 && cols.quantity < len(row) {
		rf.qtyStr = strings.TrimSpace(row[cols.quantity])
	}
	if cols.price >= 0 && cols.price < len(row) {
		rf.priceStr = strings.TrimSpace(row[cols.price])
	}
	if cols.amount >= 0 && cols.amount < len(row) {
		rf.amountStr = strings.TrimSpace(row[cols.amount])
	}
	return rf
}

func (e *EquityCSVImporter) Import(path string, accountID *int64) (*EquityImportResult, error) {
	hash, err := fileHash(path)
	if err != nil {
		return nil, err
	}
	var existing int
	if err := e.svc.DB().QueryRow("SELECT COUNT(*) FROM import_records WHERE hash = ?", hash).Scan(&existing); err != nil {
		existing = 0
	}
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
	sortDataRowsChronologically(dataRows, cols)

	result := &EquityImportResult{}

	for i := 0; i < len(dataRows); i++ {
		row := dataRows[i]
		if len(row) <= cols.date {
			continue
		}
		e.processRow(row, cols, accountID, result, i)
	}

	total := result.Purchases + result.Sales + result.Dividends
	if _, err := e.svc.DB().Exec("INSERT INTO import_records (filename, hash, tx_count) VALUES (?, ?, ?)", path, hash, total); err != nil {
		return result, fmt.Errorf("record import: %w", err)
	}
	return result, nil
}

func sortDataRowsChronologically(dataRows [][]string, cols equityColMap) {
	if len(dataRows) <= 1 || cols.date < 0 {
		return
	}
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

func (e *EquityCSVImporter) processRow(row []string, cols equityColMap, accountID *int64, result *EquityImportResult, i int) {
	rf := extractRowFields(row, cols)

	if rf.date == "" || !isValidDate(rf.date) {
		result.Skipped++
		return
	}

	txType := classifyTransaction(rf.desc)

	switch txType {
	case "purchase":
		e.handlePurchase(accountID, rf.symbol, rf.qtyStr, rf.priceStr, rf.desc, rf.date, i, result)
	case "transfer_in":
		e.handleTransferIn(accountID, rf.symbol, rf.qtyStr, rf.priceStr, rf.date, i, result)
	case "sale":
		e.handleSale(accountID, rf.symbol, rf.qtyStr, rf.date, i, result)
	case "dividend":
		e.handleDividend(accountID, rf.symbol, rf.amountStr, rf.desc, rf.date, i, result)
	case "fund_outflow":
		e.handleSimpleTransaction(accountID, rf.amountStr, rf.desc, rf.date, model.TxTransferOut, "Bought Equity", i, result, &result.Purchases)
	case "deposit":
		e.handleDeposit(accountID, rf.amountStr, rf.desc, rf.date, i, result)
	case "interest":
		e.handleSimpleTransaction(accountID, rf.amountStr, rf.desc, rf.date, model.TxIncome, "Bank Interest", i, result, &result.Dividends)
	case txTypeFee:
		e.handleSimpleTransaction(accountID, rf.amountStr, rf.desc, rf.date, model.TxTransferOut, "", i, result, &result.Purchases)
	case "reinvest_shares":
		result.Skipped++ // $0 share allocation entries
	default:
		result.Skipped++
	}
}

func (e *EquityCSVImporter) handlePurchase(accountID *int64, symbol, qtyStr, priceStr, desc, date string, i int, result *EquityImportResult) {
	if symbol == "" || qtyStr == "" || priceStr == "" {
		result.Skipped++
		return
	}
	qty := parseNumber(qtyStr)
	price := parseNumber(priceStr)
	if qty <= 0 || price <= 0 {
		result.Skipped++
		return
	}
	priceCents := int64(math.Round(price * 100))
	note := truncateDesc(desc)
	_, err := e.portfolio.BuyLot(accountID, symbol, qty, priceCents, date, note)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", i+1, err))
		return
	}
	result.Purchases++
}

func (e *EquityCSVImporter) handleTransferIn(accountID *int64, symbol, qtyStr, priceStr, date string, i int, result *EquityImportResult) {
	if symbol == "" || qtyStr == "" {
		result.Skipped++
		return
	}
	qty := parseNumber(qtyStr)
	if qty <= 0 {
		result.Skipped++
		return
	}
	// Use the price if available, otherwise cost basis = 0 (unknown)
	price := parseNumber(priceStr)
	priceCents := int64(math.Round(price * 100))
	note := "Transfer in"
	_, err := e.portfolio.BuyLot(accountID, symbol, qty, priceCents, date, note)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", i+1, err))
		return
	}
	result.Purchases++
}

func (e *EquityCSVImporter) handleSale(accountID *int64, symbol, qtyStr, _ string, i int, result *EquityImportResult) {
	if symbol == "" || qtyStr == "" {
		result.Skipped++
		return
	}
	qty := math.Abs(parseNumber(qtyStr))
	if qty <= 0 {
		result.Skipped++
		return
	}
	lots, _ := e.portfolio.ListLots(symbol, accountID)
	sold := 0.0
	for _, lot := range lots {
		if sold >= qty {
			break
		}
		toSell := math.Min(lot.Shares, qty-sold)
		if err := e.portfolio.SellLot(lot.ID, toSell); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: sell lot: %v", i+1, err))
			continue
		}
		sold += toSell
	}
	result.Sales++
}

func (e *EquityCSVImporter) handleDividend(accountID *int64, symbol, amountStr, desc, date string, i int, result *EquityImportResult) {
	if symbol == "" {
		result.Skipped++
		return
	}
	amount := parseNumber(amountStr)
	if amount == 0 {
		result.Skipped++
		return
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
		// For managed/brokerage accounts, dividends are already reflected
		// in portfolio lot values — insert without balance effect.
		// For cash-like accounts (money_market), use CreateTransaction
		// so dividends affect the account balance.
		acc, _ := e.svc.GetAccount(txAccountID)
		if acc != nil && (acc.Type == model.AccountManaged || acc.Type == model.AccountBrokerage) {
			if _, err := e.svc.DB().Exec(
				"INSERT INTO transactions (account_id, category_id, amount, date, payee, note, type) VALUES (?, ?, ?, ?, ?, ?, ?)",
				tx.AccountID, tx.CategoryID, tx.Amount, tx.Date, tx.Payee, tx.Note, tx.Type); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: insert dividend: %v", i+1, err))
				return
			}
		} else {
			if _, err := e.svc.CreateTransaction(tx); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: create dividend tx: %v", i+1, err))
				return
			}
		}
	}
	result.Dividends++
}

func (e *EquityCSVImporter) handleDeposit(accountID *int64, amountStr, desc, date string, i int, result *EquityImportResult) {
	amount := parseNumber(amountStr)
	if amount == 0 {
		result.Skipped++
		return
	}
	cents := int64(math.Round(math.Abs(amount) * 100))
	var txAccountID int64
	if accountID != nil {
		txAccountID = *accountID
	}
	if txAccountID > 0 {
		txType := model.TxIncome
		if amount < 0 {
			txType = model.TxExpense
		}
		tx := model.Transaction{
			AccountID: txAccountID,
			Amount:    cents,
			Date:      date,
			Payee:     truncateDesc(desc),
			Type:      txType,
		}
		tx.CategoryID = e.svc.AutoCategorize(desc)
		if _, err := e.svc.CreateTransaction(tx); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: create deposit tx: %v", i+1, err))
			return
		}
	}
	result.Dividends++
}

func (e *EquityCSVImporter) handleSimpleTransaction(accountID *int64, amountStr, desc, date string, txType model.TxType, autoCatPayee string, i int, result *EquityImportResult, counter *int) {
	amount := parseNumber(amountStr)
	if amount == 0 {
		result.Skipped++
		return
	}
	cents := int64(math.Round(math.Abs(amount) * 100))
	var txAccountID int64
	if accountID != nil {
		txAccountID = *accountID
	}
	if txAccountID > 0 {
		tx := model.Transaction{
			AccountID: txAccountID,
			Amount:    cents,
			Date:      date,
			Payee:     truncateDesc(desc),
			Type:      txType,
		}
		if autoCatPayee != "" {
			if cat, err := e.svc.FindCategoryByName(autoCatPayee); err == nil {
				tx.CategoryID = &cat.ID
			} else {
				tx.CategoryID = e.svc.AutoCategorize(autoCatPayee)
			}
		}
		if _, err := e.svc.CreateTransaction(tx); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: create %s tx: %v", i+1, txType, err))
			return
		}
	}
	*counter++
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

	// Exact-match lookup table
	exactMap := map[string]func(int){
		"trade date":      func(i int) { m.date = i },
		"settlement date": func(i int) { m.settlement = i },
		"account":         func(i int) { m.account = i },
		"description":     func(i int) { m.description = i },
		"desc":            func(i int) { m.description = i },
		"type":            func(i int) { m.txType = i },
		"action":          func(i int) { m.txType = i },
		"trans type":      func(i int) { m.txType = i },
		"ticker":          func(i int) { m.symbol = i },
		"qty":             func(i int) { m.quantity = i },
		"shares":          func(i int) { m.quantity = i },
		"net amount":      func(i int) { m.amount = i },
		"total":           func(i int) { m.amount = i },
	}

	// Substring-match rules, checked in order
	type substringRule struct {
		substrings []string
		assign     func(int)
	}
	substringRules := []substringRule{
		{[]string{"trade", "date"}, func(i int) { m.date = i }}, // both must match
		{[]string{"settle"}, func(i int) { m.settlement = i }},
		{[]string{"symbol"}, func(i int) { m.symbol = i }},
		{[]string{"cusip"}, func(i int) { m.symbol = i }},
		{[]string{"quantity"}, func(i int) { m.quantity = i }},
		{[]string{"price"}, func(i int) { m.price = i }},
		{[]string{"amount"}, func(i int) { m.amount = i }},
	}

	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		if fn, ok := exactMap[h]; ok {
			fn(i)
			continue
		}
		for _, rule := range substringRules {
			matched := true
			for _, sub := range rule.substrings {
				if !strings.Contains(h, sub) {
					matched = false
					break
				}
			}
			if matched {
				rule.assign(i)
				break
			}
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
	case strings.Contains(d, "funds received") || strings.HasPrefix(d, "withdrawal") || strings.HasPrefix(d, "transfer / adjustment"):
		return "deposit"
	case strings.Contains(d, "annual service fee") || strings.Contains(d, "advisory fee"):
		return txTypeFee
	case strings.Contains(d, "advisory program fee") || strings.Contains(d, "advisory fee"):
		return txTypeFee
	case strings.Contains(d, "bank interest"):
		return "interest"
	case strings.HasPrefix(d, "reinvestment program") || strings.HasPrefix(d, "subscription"):
		return "fund_outflow"
	case strings.HasPrefix(d, "reinvestment share"):
		return "reinvest_shares"
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
