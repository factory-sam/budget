package importer

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type CSVImporter struct {
	svc *service.Service
}

func NewCSV(svc *service.Service) *CSVImporter {
	return &CSVImporter{svc: svc}
}

func (c *CSVImporter) Import(path string, accountID int64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// check for duplicate import
	hash, err := fileHash(path)
	if err != nil {
		return 0, err
	}
	var existing int
	if err := c.svc.DB().QueryRow("SELECT COUNT(*) FROM import_records WHERE hash = ?", hash).Scan(&existing); err != nil {
		existing = 0
	}
	if existing > 0 {
		return 0, fmt.Errorf("file already imported (hash: %s)", hash[:12])
	}

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	// Read all records and find the header row
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

	headerIdx := -1
	var colMap columnMap
	for i, rec := range allRecords {
		cm := detectColumns(rec)
		if cm.date >= 0 && cm.amount >= 0 {
			colMap = cm
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return 0, fmt.Errorf("could not detect date and amount columns in CSV")
	}

	count := 0
	for _, record := range allRecords[headerIdx+1:] {
		tx, ok := c.processRecord(record, colMap, accountID)
		if !ok {
			continue
		}
		if _, err := c.svc.CreateTransaction(tx); err != nil {
			continue
		}
		count++
	}

	if _, err := c.svc.DB().Exec("INSERT INTO import_records (filename, hash, tx_count) VALUES (?, ?, ?)", path, hash, count); err != nil {
		return count, fmt.Errorf("record import: %w", err)
	}
	return count, nil
}

func (c *CSVImporter) processRecord(record []string, colMap columnMap, accountID int64) (model.Transaction, bool) {
	date := ""
	if colMap.date >= 0 && colMap.date < len(record) {
		date = normalizeDate(record[colMap.date])
	}

	amountStr := ""
	if colMap.amount >= 0 && colMap.amount < len(record) {
		amountStr = record[colMap.amount]
	}
	amount, txType := parseAmount(amountStr)
	if amount == 0 {
		return model.Transaction{}, false
	}

	payee := ""
	if colMap.payee >= 0 && colMap.payee < len(record) {
		payee = strings.TrimSpace(record[colMap.payee])
	}
	if payee == "" && colMap.description >= 0 && colMap.description < len(record) {
		payee = strings.TrimSpace(record[colMap.description])
	}

	note := ""
	if colMap.memo >= 0 && colMap.memo < len(record) {
		note = strings.TrimSpace(record[colMap.memo])
	}

	catID := c.svc.AutoCategorize(payee)

	// Credit card payments are transfers, not expenses
	if catID != nil {
		cat, _ := c.svc.FindCategoryByName("Credit Card Payment")
		if cat != nil && *catID == cat.ID {
			txType = model.TxTransfer
		}
	}

	tx := model.Transaction{
		AccountID:  accountID,
		CategoryID: catID,
		Amount:     amount,
		Date:       date,
		Payee:      payee,
		Note:       note,
		Type:       txType,
	}
	return tx, true
}

type columnMap struct {
	date        int
	amount      int
	payee       int
	description int
	memo        int
}

func detectColumns(header []string) columnMap {
	m := columnMap{date: -1, amount: -1, payee: -1, description: -1, memo: -1}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		switch {
		case strings.Contains(h, "date"):
			if m.date < 0 {
				m.date = i
			}
		case h == "amount" || h == "debit" || h == "credit":
			if m.amount < 0 {
				m.amount = i
			}
		case strings.Contains(h, "payee") || h == "name":
			m.payee = i
		case strings.Contains(h, "desc") || strings.Contains(h, "memo") || strings.Contains(h, "narrative"):
			if m.description < 0 {
				m.description = i
			}
		case strings.Contains(h, "note") || strings.Contains(h, "memo"):
			m.memo = i
		}
	}
	return m
}

func parseAmount(s string) (int64, model.TxType) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "$", "")

	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
		negative = true
	}
	if strings.HasPrefix(s, "-") {
		s = s[1:]
		negative = true
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, model.TxExpense
	}

	cents := int64(math.Round(f * 100))
	if negative {
		return cents, model.TxExpense
	}
	return cents, model.TxIncome
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	// already YYYY-MM-DD
	if len(s) == 10 && s[4] == '-' {
		return s
	}
	// MM/DD/YYYY
	parts := strings.Split(s, "/")
	if len(parts) == 3 {
		return fmt.Sprintf("%s-%s-%s", parts[2], zeroPad(parts[0]), zeroPad(parts[1]))
	}
	return s
}

func zeroPad(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
