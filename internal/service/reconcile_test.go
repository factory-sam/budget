package service

import (
	"testing"

	"github.com/sam/budget/internal/model"
)

// ---------- Transfer Matching ----------

func TestAutoMatchTransfers(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, err := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	if err != nil {
		t.Fatalf("create checking: %v", err)
	}
	acc2, err := svc.CreateAccount("Savings", model.AccountSavings, 100000)
	if err != nil {
		t.Fatalf("create savings: %v", err)
	}

	// Create a transfer-out and transfer-in with same amount, same date
	_, err = svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer to Savings", Type: model.TxTransferOut,
	})
	if err != nil {
		t.Fatalf("create transfer_out: %v", err)
	}
	_, err = svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer from Checking", Type: model.TxTransferIn,
	})
	if err != nil {
		t.Fatalf("create transfer_in: %v", err)
	}

	matches, err := svc.AutoMatchTransfers(3)
	if err != nil {
		t.Fatalf("AutoMatchTransfers: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Confidence != "exact" {
		t.Errorf("confidence = %q, want 'exact'", matches[0].Confidence)
	}
	if matches[0].DaysDiff != 0 {
		t.Errorf("days_diff = %d, want 0", matches[0].DaysDiff)
	}
}

func TestAutoMatchTransfers_LikelyMatch(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 100000)

	// Transfer out on day 1, transfer in on day 3 → "likely"
	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 25000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 25000, Date: "2025-03-03",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	matches, err := svc.AutoMatchTransfers(5)
	if err != nil {
		t.Fatalf("AutoMatchTransfers: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Confidence != "likely" {
		t.Errorf("confidence = %q, want 'likely'", matches[0].Confidence)
	}
	if matches[0].DaysDiff != 2 {
		t.Errorf("days_diff = %d, want 2", matches[0].DaysDiff)
	}
}

func TestAutoMatchTransfers_NoMatchDifferentAmount(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 100000)

	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 30000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	matches, err := svc.AutoMatchTransfers(3)
	if err != nil {
		t.Fatalf("AutoMatchTransfers: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for different amounts, got %d", len(matches))
	}
}

func TestAutoMatchTransfers_SameAccountNoMatch(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)

	// Both transfers on same account → should not match
	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	matches, err := svc.AutoMatchTransfers(3)
	if err != nil {
		t.Fatalf("AutoMatchTransfers: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (same account), got %d", len(matches))
	}
}

// ---------- Link / Unlink Transfer Pair ----------

func TestLinkUnlinkTransferPair(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 100000)

	outTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	inTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	// Link
	if err := svc.LinkTransferPair(outTx.ID, inTx.ID); err != nil {
		t.Fatalf("LinkTransferPair: %v", err)
	}

	// Verify pair IDs are set
	var pairID *int64
	err := svc.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", outTx.ID).Scan(&pairID)
	if err != nil || pairID == nil || *pairID != inTx.ID {
		t.Errorf("out tx pair_id = %v, want %d", pairID, inTx.ID)
	}
	err = svc.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", inTx.ID).Scan(&pairID)
	if err != nil || pairID == nil || *pairID != outTx.ID {
		t.Errorf("in tx pair_id = %v, want %d", pairID, outTx.ID)
	}

	// Unlink from one side should clear both
	if err := svc.UnlinkTransferPair(outTx.ID); err != nil {
		t.Fatalf("UnlinkTransferPair: %v", err)
	}

	err = svc.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", outTx.ID).Scan(&pairID)
	if err != nil {
		t.Fatalf("query out after unlink: %v", err)
	}
	if pairID != nil {
		t.Errorf("out tx pair_id after unlink = %v, want nil", *pairID)
	}

	err = svc.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", inTx.ID).Scan(&pairID)
	if err != nil {
		t.Fatalf("query in after unlink: %v", err)
	}
	if pairID != nil {
		t.Errorf("in tx pair_id after unlink = %v, want nil", *pairID)
	}
}

// ---------- Apply Transfer Matches ----------

func TestApplyTransferMatches(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 100000)

	outTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	inTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	matches := []model.TransferMatch{
		{OutTx: *outTx, InTx: *inTx, Confidence: "exact", DaysDiff: 0},
	}

	count, err := svc.ApplyTransferMatches(matches)
	if err != nil {
		t.Fatalf("ApplyTransferMatches: %v", err)
	}
	if count != 1 {
		t.Errorf("applied count = %d, want 1", count)
	}

	// Verify they're linked
	var pairID *int64
	svc.db.QueryRow("SELECT transfer_pair_id FROM transactions WHERE id = ?", outTx.ID).Scan(&pairID)
	if pairID == nil || *pairID != inTx.ID {
		t.Error("transfer pair was not applied")
	}
}

func TestApplyTransferMatches_SkipsAmbiguous(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 0)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 0)

	outTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 10000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferOut,
	})
	inTx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 10000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxTransferIn,
	})

	matches := []model.TransferMatch{
		{OutTx: *outTx, InTx: *inTx, Confidence: "ambiguous", DaysDiff: 1},
	}

	count, err := svc.ApplyTransferMatches(matches)
	if err != nil {
		t.Fatalf("ApplyTransferMatches: %v", err)
	}
	if count != 0 {
		t.Errorf("applied count = %d, want 0 (ambiguous should be skipped)", count)
	}
}

// ---------- Find Duplicates ----------

func TestFindDuplicates(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Create duplicate transactions (same date, amount, payee, account)
	for i := 0; i < 3; i++ {
		if _, err := svc.CreateTransaction(model.Transaction{
			AccountID: acc.ID, Amount: 5000, Date: "2025-03-15",
			Payee: "Coffee Shop", Type: model.TxExpense,
		}); err != nil {
			t.Fatalf("create dup tx %d: %v", i, err)
		}
	}

	// Create a non-duplicate
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 10000, Date: "2025-03-16",
		Payee: "Bookstore", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create unique tx: %v", err)
	}

	groups, err := svc.FindDuplicates()
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if len(groups[0].Transactions) != 3 {
		t.Errorf("group has %d txs, want 3", len(groups[0].Transactions))
	}
	if groups[0].Reason != "same_date_amount_payee" {
		t.Errorf("reason = %q, want 'same_date_amount_payee'", groups[0].Reason)
	}
}

func TestFindDuplicates_NoDuplicates(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 5000, Date: "2025-03-15",
		Payee: "Coffee Shop", Type: model.TxExpense,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 6000, Date: "2025-03-15",
		Payee: "Tea Shop", Type: model.TxExpense,
	})

	groups, err := svc.FindDuplicates()
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 duplicate groups, got %d", len(groups))
	}
}

// ---------- Reconcile Account ----------

func TestReconcileAccount(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Clear equity funcs
	origGetAccountEquity := GetAccountEquityValue
	GetAccountEquityValue = nil
	t.Cleanup(func() { GetAccountEquityValue = origGetAccountEquity })

	acc, _ := svc.CreateAccount("Checking", model.AccountChecking, 0)

	// Add some transactions
	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 100000, Date: "2025-03-01",
		Payee: "Salary", Type: model.TxIncome,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 30000, Date: "2025-03-05",
		Payee: "Rent", Type: model.TxExpense,
	})
	// Calculated balance: 100000 - 30000 = 70000

	// Reconcile with actual balance of 68000 (discrepancy of -2000)
	report, err := svc.ReconcileAccount(acc.ID, 68000, "monthly check")
	if err != nil {
		t.Fatalf("ReconcileAccount: %v", err)
	}
	if report.CalculatedBal != 70000 {
		t.Errorf("calculated_balance = %d, want 70000", report.CalculatedBal)
	}
	if report.ActualBal != 68000 {
		t.Errorf("actual_balance = %d, want 68000", report.ActualBal)
	}
	if report.Discrepancy != -2000 {
		t.Errorf("discrepancy = %d, want -2000", report.Discrepancy)
	}
}

func TestReconcileAccount_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	_, err := svc.ReconcileAccount(9999, 0, "")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

// ---------- Get Last Reconciliation ----------

func TestGetLastReconciliation(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	origGetAccountEquity := GetAccountEquityValue
	GetAccountEquityValue = nil
	t.Cleanup(func() { GetAccountEquityValue = origGetAccountEquity })

	acc, _ := svc.CreateAccount("Checking", model.AccountChecking, 0)

	// No reconciliation yet
	_, err := svc.GetLastReconciliation(acc.ID)
	if err == nil {
		t.Error("expected error when no reconciliation exists")
	}

	// Reconcile
	svc.ReconcileAccount(acc.ID, 0, "initial")

	r, err := svc.GetLastReconciliation(acc.ID)
	if err != nil {
		t.Fatalf("GetLastReconciliation: %v", err)
	}
	if r.AccountID != acc.ID {
		t.Errorf("account_id = %d, want %d", r.AccountID, acc.ID)
	}
	if r.LastReconciled == "" {
		t.Error("expected non-empty last_reconciled date")
	}
}

// ---------- Paycheck Management ----------

func TestCreatePaycheck(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	p, err := svc.CreatePaycheck(model.Paycheck{
		Date:            "2025-03-15",
		Employer:        "Acme Corp",
		GrossPay:        500000,
		FederalTax:      75000,
		StateTax:        25000,
		SocialSecurity:  31000,
		Medicare:        7250,
		HealthInsurance: 15000,
		Retirement401k:  25000,
		OtherDeductions: 5000,
		NetPay:          316750,
		Note:            "March paycheck",
	})
	if err != nil {
		t.Fatalf("CreatePaycheck: %v", err)
	}
	if p.ID == 0 {
		t.Error("expected non-zero paycheck ID")
	}
	if p.Employer != "Acme Corp" {
		t.Errorf("employer = %q, want 'Acme Corp'", p.Employer)
	}
	if p.NetPay != 316750 {
		t.Errorf("net_pay = %d, want 316750", p.NetPay)
	}
}

func TestListPaychecks(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	for _, date := range []string{"2025-01-15", "2025-02-15", "2025-03-15"} {
		if _, err := svc.CreatePaycheck(model.Paycheck{
			Date: date, Employer: "Acme Corp", GrossPay: 500000, NetPay: 350000,
		}); err != nil {
			t.Fatalf("create paycheck %s: %v", date, err)
		}
	}

	// List all
	all, err := svc.ListPaychecks(0)
	if err != nil {
		t.Fatalf("ListPaychecks(0): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 paychecks, got %d", len(all))
	}

	// List with limit
	limited, err := svc.ListPaychecks(2)
	if err != nil {
		t.Fatalf("ListPaychecks(2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 paychecks with limit, got %d", len(limited))
	}

	// Verify DESC order (most recent first)
	if len(all) >= 2 && all[0].Date < all[1].Date {
		t.Error("expected paychecks ordered by date DESC")
	}
}

func TestLinkPaycheckToTransaction(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	tx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 350000, Date: "2025-03-15",
		Payee: "Acme Corp", Type: model.TxIncome,
	})
	p, _ := svc.CreatePaycheck(model.Paycheck{
		Date: "2025-03-15", Employer: "Acme Corp", GrossPay: 500000, NetPay: 350000,
	})

	if err := svc.LinkPaycheckToTransaction(p.ID, tx.ID); err != nil {
		t.Fatalf("LinkPaycheckToTransaction: %v", err)
	}

	// Verify the link
	paychecks, _ := svc.ListPaychecks(0)
	if len(paychecks) != 1 {
		t.Fatalf("expected 1 paycheck, got %d", len(paychecks))
	}
	if paychecks[0].TxID == nil || *paychecks[0].TxID != tx.ID {
		t.Errorf("paycheck tx_id = %v, want %d", paychecks[0].TxID, tx.ID)
	}
}

func TestMatchPaychecksToDeposits(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Create an income transaction
	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 350000, Date: "2025-03-15",
		Payee: "ACME CORP DIRECT DEP", Type: model.TxIncome,
	})

	// Create a matching paycheck (same net pay, same date, employer substring match)
	svc.CreatePaycheck(model.Paycheck{
		Date: "2025-03-15", Employer: "Acme Corp", GrossPay: 500000, NetPay: 350000,
	})

	matches, err := svc.MatchPaychecksToDeposits()
	if err != nil {
		t.Fatalf("MatchPaychecksToDeposits: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Paycheck.Employer != "Acme Corp" {
		t.Errorf("matched employer = %q, want 'Acme Corp'", matches[0].Paycheck.Employer)
	}
	if matches[0].Tx.Amount != 350000 {
		t.Errorf("matched tx amount = %d, want 350000", matches[0].Tx.Amount)
	}
}

func TestMatchPaychecksToDeposits_SkipsLinked(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	tx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 350000, Date: "2025-03-15",
		Payee: "ACME CORP", Type: model.TxIncome,
	})
	p, _ := svc.CreatePaycheck(model.Paycheck{
		Date: "2025-03-15", Employer: "Acme Corp", GrossPay: 500000, NetPay: 350000,
	})

	// Link the paycheck first
	svc.LinkPaycheckToTransaction(p.ID, tx.ID)

	matches, err := svc.MatchPaychecksToDeposits()
	if err != nil {
		t.Fatalf("MatchPaychecksToDeposits: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (already linked), got %d", len(matches))
	}
}

func TestMatchPaychecksToDeposits_NoMatchDifferentAmount(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 400000, Date: "2025-03-15",
		Payee: "ACME CORP", Type: model.TxIncome,
	})
	svc.CreatePaycheck(model.Paycheck{
		Date: "2025-03-15", Employer: "Acme Corp", GrossPay: 500000, NetPay: 350000,
	})

	matches, err := svc.MatchPaychecksToDeposits()
	if err != nil {
		t.Fatalf("MatchPaychecksToDeposits: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (different amount), got %d", len(matches))
	}
}

// ---------- GetUnmatchedTransfers ----------

func TestGetUnmatchedTransfers(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc1, _ := svc.CreateAccount("Checking", model.AccountChecking, 100000)
	acc2, _ := svc.CreateAccount("Savings", model.AccountSavings, 100000)

	// Create unmatched transfers
	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 50000, Date: "2025-03-01",
		Payee: "Transfer Out", Type: model.TxTransferOut,
	})
	svc.CreateTransaction(model.Transaction{
		AccountID: acc2.ID, Amount: 30000, Date: "2025-03-02",
		Payee: "Transfer In", Type: model.TxTransferIn,
	})

	// Create a regular expense (should not appear)
	svc.CreateTransaction(model.Transaction{
		AccountID: acc1.ID, Amount: 1000, Date: "2025-03-03",
		Payee: "Coffee", Type: model.TxExpense,
	})

	unmatched, err := svc.GetUnmatchedTransfers()
	if err != nil {
		t.Fatalf("GetUnmatchedTransfers: %v", err)
	}
	if len(unmatched) != 2 {
		t.Fatalf("expected 2 unmatched transfers, got %d", len(unmatched))
	}

	// Check directions
	dirs := map[string]bool{}
	for _, u := range unmatched {
		dirs[u.Direction] = true
	}
	if !dirs["out"] || !dirs["in"] {
		t.Errorf("expected both 'out' and 'in' directions, got %v", dirs)
	}
}
