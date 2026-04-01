package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sam/budget/internal/model"
)

// --- Transfer Matching ---

func (s *Service) AutoMatchTransfers(maxDaysDiff int) ([]model.TransferMatch, error) {
	outTxs, err := s.ListTransactions(model.TxFilter{Type: model.TxTransferOut, Limit: 1000})
	if err != nil {
		return nil, err
	}
	inTxs, err := s.ListTransactions(model.TxFilter{Type: model.TxTransferIn, Limit: 1000})
	if err != nil {
		return nil, err
	}

	// Index already-paired IDs
	paired := make(map[int64]bool)
	for _, tx := range outTxs {
		if tx.TransferPairID != nil {
			paired[tx.ID] = true
			paired[*tx.TransferPairID] = true
		}
	}
	for _, tx := range inTxs {
		if tx.TransferPairID != nil {
			paired[tx.ID] = true
			paired[*tx.TransferPairID] = true
		}
	}

	// Find matches: same amount, within maxDaysDiff days, different accounts
	usedIn := make(map[int64]bool)
	var matches []model.TransferMatch

	for _, out := range outTxs {
		if paired[out.ID] {
			continue
		}
		match, days, ambiguous := findBestTransferMatch(out, inTxs, paired, usedIn, maxDaysDiff)
		if match != nil {
			confidence := "exact"
			if days > 0 {
				confidence = "likely"
			}
			if ambiguous {
				confidence = "ambiguous"
			}
			matches = append(matches, model.TransferMatch{
				OutTx:      out,
				InTx:       *match,
				Confidence: confidence,
				DaysDiff:   days,
			})
			usedIn[match.ID] = true
		}
	}

	return matches, nil
}

func findBestTransferMatch(out model.Transaction, inTxs []model.Transaction, paired, usedIn map[int64]bool, maxDaysDiff int) (*model.Transaction, int, bool) {
	outDate, _ := time.Parse("2006-01-02", out.Date)
	var bestMatch *model.Transaction
	bestDays := maxDaysDiff + 1
	ambiguous := false

	for i := range inTxs {
		in := &inTxs[i]
		if paired[in.ID] || usedIn[in.ID] {
			continue
		}
		if in.Amount != out.Amount || in.AccountID == out.AccountID {
			continue
		}
		inDate, _ := time.Parse("2006-01-02", in.Date)
		days := int(math.Abs(outDate.Sub(inDate).Hours() / 24))
		if days > maxDaysDiff {
			continue
		}
		if bestMatch == nil || days < bestDays {
			bestMatch = in
			bestDays = days
			ambiguous = false
		} else if days == bestDays {
			ambiguous = true
		}
	}
	return bestMatch, bestDays, ambiguous
}

func (s *Service) LinkTransferPair(outID, inID int64) error {
	_, err := s.db.Exec("UPDATE transactions SET transfer_pair_id = ? WHERE id = ?", inID, outID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE transactions SET transfer_pair_id = ? WHERE id = ?", outID, inID)
	return err
}

func (s *Service) UnlinkTransferPair(txID int64) error {
	var pairID *int64
	err := s.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", txID).Scan(&pairID)
	if err != nil || pairID == nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE transactions SET transfer_pair_id = NULL WHERE id = ?", txID); err != nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE transactions SET transfer_pair_id = NULL WHERE id = ?", *pairID); err != nil {
		return err
	}
	return nil
}

func (s *Service) ApplyTransferMatches(matches []model.TransferMatch) (int, error) {
	count := 0
	for _, m := range matches {
		if m.Confidence == "ambiguous" {
			continue
		}
		if err := s.LinkTransferPair(m.OutTx.ID, m.InTx.ID); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func (s *Service) GetUnmatchedTransfers() ([]model.UnmatchedTransfer, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.account_id, COALESCE(a.name,''), t.category_id, COALESCE(c.name,''),
			t.amount, t.date, t.payee, t.note, t.type, t.created_at
		FROM transactions t
		LEFT JOIN accounts a ON t.account_id = a.id
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE t.type IN ('transfer_out', 'transfer_in')
		AND t.transfer_pair_id IS NULL
		ORDER BY t.date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.UnmatchedTransfer
	for rows.Next() {
		var tx model.Transaction
		if err := rows.Scan(&tx.ID, &tx.AccountID, &tx.AccountName, &tx.CategoryID, &tx.CategoryName,
			&tx.Amount, &tx.Date, &tx.Payee, &tx.Note, &tx.Type, &tx.CreatedAt); err != nil {
			continue
		}
		dir := "out"
		if tx.Type == model.TxTransferIn {
			dir = "in"
		}
		result = append(result, model.UnmatchedTransfer{Tx: tx, Direction: dir})
	}
	return result, nil
}

// --- Duplicate Detection ---

func (s *Service) FindDuplicates() ([]model.DuplicateGroup, error) {
	// Exact duplicates: same date, amount, payee, account
	rows, err := s.db.Query(`
		SELECT t.id, t.account_id, COALESCE(a.name,''), t.category_id, COALESCE(c.name,''),
			t.amount, t.date, t.payee, t.note, t.type, t.created_at
		FROM transactions t
		LEFT JOIN accounts a ON t.account_id = a.id
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE (t.date, t.amount, t.payee, t.account_id) IN (
			SELECT date, amount, payee, account_id FROM transactions
			GROUP BY date, amount, payee, account_id
			HAVING COUNT(*) > 1
		)
		ORDER BY t.date DESC, t.amount, t.payee, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allTxs []model.Transaction
	for rows.Next() {
		var tx model.Transaction
		if err := rows.Scan(&tx.ID, &tx.AccountID, &tx.AccountName, &tx.CategoryID, &tx.CategoryName,
			&tx.Amount, &tx.Date, &tx.Payee, &tx.Note, &tx.Type, &tx.CreatedAt); err != nil {
			continue
		}
		allTxs = append(allTxs, tx)
	}

	// Group by key
	type groupKey struct {
		date, payee string
		amount, aid int64
	}
	groups := make(map[groupKey][]model.Transaction)
	var order []groupKey
	for _, tx := range allTxs {
		k := groupKey{tx.Date, tx.Payee, tx.Amount, tx.AccountID}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], tx)
	}

	result := make([]model.DuplicateGroup, 0, len(order))
	for _, k := range order {
		result = append(result, model.DuplicateGroup{
			Transactions: groups[k],
			Reason:       "same_date_amount_payee",
		})
	}
	return result, nil
}

// --- Balance Reconciliation ---

func (s *Service) ReconcileAccount(accountID int64, actualBalanceCents int64, note string) (*model.ReconciliationReport, error) {
	if _, err := s.GetAccount(accountID); err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	if err := s.RecalculateAccountBalance(accountID); err != nil {
		return nil, err
	}
	acc, _ := s.GetAccount(accountID)

	discrepancy := actualBalanceCents - acc.Balance
	today := time.Now().Format("2006-01-02")

	_, err := s.db.Exec(`INSERT INTO reconciliations (account_id, date, actual_balance, calculated_balance, discrepancy, note)
		VALUES (?, ?, ?, ?, ?, ?)`, accountID, today, actualBalanceCents, acc.Balance, discrepancy, note)
	if err != nil {
		return nil, err
	}

	return &model.ReconciliationReport{
		AccountID:      acc.ID,
		AccountName:    acc.Name,
		CalculatedBal:  acc.Balance,
		ActualBal:      actualBalanceCents,
		Discrepancy:    discrepancy,
		LastReconciled: today,
	}, nil
}

func (s *Service) GetLastReconciliation(accountID int64) (*model.ReconciliationReport, error) {
	var r model.ReconciliationReport
	err := s.db.QueryRow(`SELECT account_id, date, actual_balance, calculated_balance, discrepancy
		FROM reconciliations WHERE account_id = ? ORDER BY date DESC LIMIT 1`, accountID).
		Scan(&r.AccountID, &r.LastReconciled, &r.ActualBal, &r.CalculatedBal, &r.Discrepancy)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// --- Paycheck Management ---

func (s *Service) CreatePaycheck(p model.Paycheck) (*model.Paycheck, error) {
	res, err := s.db.Exec(`INSERT INTO paychecks 
		(date, employer, gross_pay, federal_tax, state_tax, social_security, medicare,
		 health_insurance, retirement_401k, other_deductions, net_pay, tx_id, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Date, p.Employer, p.GrossPay, p.FederalTax, p.StateTax, p.SocialSecurity, p.Medicare,
		p.HealthInsurance, p.Retirement401k, p.OtherDeductions, p.NetPay, p.TxID, p.Note)
	if err != nil {
		return nil, err
	}
	p.ID, _ = res.LastInsertId()
	return &p, nil
}

func (s *Service) ListPaychecks(limit int) ([]model.Paycheck, error) {
	q := "SELECT id, date, employer, gross_pay, federal_tax, state_tax, social_security, medicare, health_insurance, retirement_401k, other_deductions, net_pay, tx_id, note FROM paychecks ORDER BY date DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Paycheck
	for rows.Next() {
		var p model.Paycheck
		if err := rows.Scan(&p.ID, &p.Date, &p.Employer, &p.GrossPay, &p.FederalTax, &p.StateTax,
			&p.SocialSecurity, &p.Medicare, &p.HealthInsurance, &p.Retirement401k, &p.OtherDeductions,
			&p.NetPay, &p.TxID, &p.Note); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

func (s *Service) LinkPaycheckToTransaction(paycheckID, txID int64) error {
	_, err := s.db.Exec("UPDATE paychecks SET tx_id = ? WHERE id = ?", txID, paycheckID)
	return err
}

func (s *Service) MatchPaychecksToDeposits() ([]struct {
	Paycheck model.Paycheck
	Tx       model.Transaction
}, error) {
	paychecks, err := s.ListPaychecks(0)
	if err != nil {
		return nil, err
	}

	var matches []struct {
		Paycheck model.Paycheck
		Tx       model.Transaction
	}

	for _, p := range paychecks {
		if p.TxID != nil {
			continue
		}
		// Find income transactions within 3 days of paycheck date matching net pay
		rows, err := s.db.Query(`
			SELECT t.id, t.account_id, COALESCE(a.name,''), t.category_id, COALESCE(c.name,''),
				t.amount, t.date, t.payee, t.note, t.type, t.created_at
			FROM transactions t
			LEFT JOIN accounts a ON t.account_id = a.id
			LEFT JOIN categories c ON t.category_id = c.id
			WHERE t.type = 'income' AND t.amount = ?
			AND ABS(JULIANDAY(t.date) - JULIANDAY(?)) <= 3
			ORDER BY ABS(JULIANDAY(t.date) - JULIANDAY(?))
			LIMIT 1`, p.NetPay, p.Date, p.Date)
		if err != nil {
			continue
		}
		if rows.Next() {
			var tx model.Transaction
			if err := rows.Scan(&tx.ID, &tx.AccountID, &tx.AccountName, &tx.CategoryID, &tx.CategoryName,
				&tx.Amount, &tx.Date, &tx.Payee, &tx.Note, &tx.Type, &tx.CreatedAt); err != nil {
				rows.Close()
				continue
			}
			if strings.Contains(strings.ToUpper(tx.Payee), strings.ToUpper(p.Employer)) || p.Employer == "" {
				matches = append(matches, struct {
					Paycheck model.Paycheck
					Tx       model.Transaction
				}{p, tx})
			}
		}
		rows.Close()
	}
	return matches, nil
}
