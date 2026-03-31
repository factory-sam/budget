package importer

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/aclindsa/ofxgo"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type OFXImporter struct {
	svc *service.Service
}

func NewOFX(svc *service.Service) *OFXImporter {
	return &OFXImporter{svc: svc}
}

func (o *OFXImporter) Import(path string, accountID int64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	hash, err := fileHash(path)
	if err != nil {
		return 0, err
	}
	var existing int
	if err := o.svc.DB().QueryRow("SELECT COUNT(*) FROM import_records WHERE hash = ?", hash).Scan(&existing); err != nil {
		existing = 0
	}
	if existing > 0 {
		return 0, fmt.Errorf("file already imported (hash: %s)", hash[:12])
	}

	resp, err := ofxgo.ParseResponse(f)
	if err != nil {
		return 0, fmt.Errorf("parse OFX: %w", err)
	}

	count := 0
	for _, msg := range resp.Bank {
		if stmt, ok := msg.(*ofxgo.StatementResponse); ok {
			for _, t := range stmt.BankTranList.Transactions {
				amount := t.TrnAmt.Rat
				f64, _ := amount.Float64()
				cents := int64(math.Round(math.Abs(f64) * 100))

				txType := model.TxIncome
				if f64 < 0 {
					txType = model.TxExpense
				}

				date := t.DtPosted.Time.Format("2006-01-02")
				payee := strings.TrimSpace(string(t.Name))
				if payee == "" {
					payee = strings.TrimSpace(string(t.Memo))
				}

				catID := o.svc.AutoCategorize(payee)

				tx := model.Transaction{
					AccountID:  accountID,
					CategoryID: catID,
					Amount:     cents,
					Date:       date,
					Payee:      payee,
					Type:       txType,
				}
				if _, err := o.svc.CreateTransaction(tx); err != nil {
					continue
				}
				count++
			}
		}
	}

	for _, msg := range resp.CreditCard {
		if stmt, ok := msg.(*ofxgo.CCStatementResponse); ok {
			for _, t := range stmt.BankTranList.Transactions {
				amount := t.TrnAmt.Rat
				f64, _ := amount.Float64()
				cents := int64(math.Round(math.Abs(f64) * 100))

				txType := model.TxExpense
				if f64 > 0 {
					txType = model.TxIncome // payment/credit
				}

				date := t.DtPosted.Time.Format("2006-01-02")
				payee := strings.TrimSpace(string(t.Name))

				catID := o.svc.AutoCategorize(payee)

				tx := model.Transaction{
					AccountID:  accountID,
					CategoryID: catID,
					Amount:     cents,
					Date:       date,
					Payee:      payee,
					Type:       txType,
				}
				if _, err := o.svc.CreateTransaction(tx); err != nil {
					continue
				}
				count++
			}
		}
	}

	if _, err := o.svc.DB().Exec("INSERT INTO import_records (filename, hash, tx_count) VALUES (?, ?, ?)", path, hash, count); err != nil {
		return count, fmt.Errorf("record import: %w", err)
	}
	return count, nil
}
