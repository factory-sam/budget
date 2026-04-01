package service

import (
	"testing"
	"time"

	"github.com/sam/budget/internal/model"
)

// ---------- UpdateTransaction ----------

func TestExtraUpdateTransaction(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("find category: %v", err)
	}

	tx, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    5000,
		Date:      "2025-03-10",
		Payee:     "Store A",
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	// Update transaction fields
	err = svc.UpdateTransaction(tx.ID, acc.ID, &cat.ID, 7500, "2025-03-11", "Store B", "updated note", model.TxExpense)
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}

	// Verify update via list
	txs, err := svc.ListTransactions(model.TxFilter{Search: "Store B"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txs))
	}
	if txs[0].Amount != 7500 {
		t.Errorf("amount = %d, want 7500", txs[0].Amount)
	}
	if txs[0].Date != "2025-03-11" {
		t.Errorf("date = %q, want 2025-03-11", txs[0].Date)
	}
	if txs[0].Note != "updated note" {
		t.Errorf("note = %q, want 'updated note'", txs[0].Note)
	}
}

// ---------- UpdateTransactionCategory ----------

func TestExtraUpdateTransactionCategory(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("find category: %v", err)
	}

	tx, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    3000,
		Date:      "2025-04-01",
		Payee:     "Uncategorized Store",
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	// Set category
	if err := svc.UpdateTransactionCategory(tx.ID, &cat.ID); err != nil {
		t.Fatalf("UpdateTransactionCategory: %v", err)
	}

	txs, _ := svc.ListTransactions(model.TxFilter{CategoryID: &cat.ID})
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx with category, got %d", len(txs))
	}
	if txs[0].ID != tx.ID {
		t.Errorf("tx ID mismatch: got %d, want %d", txs[0].ID, tx.ID)
	}

	// Clear category
	if err := svc.UpdateTransactionCategory(tx.ID, nil); err != nil {
		t.Fatalf("clear category: %v", err)
	}
	txs, _ = svc.ListTransactions(model.TxFilter{Uncategorized: true})
	found := false
	for _, tt := range txs {
		if tt.ID == tx.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected tx to be uncategorized after clearing category")
	}
}

// ---------- UpdateTransactionType ----------

func TestExtraUpdateTransactionType(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	origGetAccountEquity := GetAccountEquityValue
	GetAccountEquityValue = nil
	t.Cleanup(func() { GetAccountEquityValue = origGetAccountEquity })

	acc := newTestAccount(t, svc)

	tx, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    10000,
		Date:      "2025-03-01",
		Payee:     "Mistaken expense",
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != -10000 {
		t.Fatalf("balance after expense = %d, want -10000", got.Balance)
	}

	// Change from expense to income — should recalculate balance
	if err := svc.UpdateTransactionType(tx.ID, model.TxIncome); err != nil {
		t.Fatalf("UpdateTransactionType: %v", err)
	}

	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 10000 {
		t.Errorf("balance after type change = %d, want 10000", got.Balance)
	}

	// No-op: same type should return nil without error
	if err := svc.UpdateTransactionType(tx.ID, model.TxIncome); err != nil {
		t.Errorf("same-type update should not error: %v", err)
	}
}

// ---------- Recurring Rules ----------

func TestExtraRecurringRulesCRUD(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	cat, _ := svc.FindCategoryByName("Rent/Mortgage")

	rule, err := svc.CreateRecurringRule(model.RecurringRule{
		AccountID:  acc.ID,
		CategoryID: &cat.ID,
		Amount:     150000,
		Payee:      "Landlord",
		Note:       "monthly rent",
		Frequency:  model.FreqMonthly,
		StartDate:  "2025-01-01",
		Type:       model.TxExpense,
	})
	if err != nil {
		t.Fatalf("CreateRecurringRule: %v", err)
	}
	if rule.ID == 0 {
		t.Fatal("expected non-zero rule ID")
	}
	if rule.NextDue != "2025-01-01" {
		t.Errorf("NextDue = %q, want 2025-01-01", rule.NextDue)
	}

	// List
	rules, err := svc.ListRecurringRules()
	if err != nil {
		t.Fatalf("ListRecurringRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Payee != "Landlord" {
		t.Errorf("payee = %q, want Landlord", rules[0].Payee)
	}

	// Delete
	if err := svc.DeleteRecurringRule(rule.ID); err != nil {
		t.Fatalf("DeleteRecurringRule: %v", err)
	}
	rules, _ = svc.ListRecurringRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

// ---------- GenerateRecurring ----------

func TestExtraGenerateRecurring(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Create a rule with next_due in the past so it will be generated
	today := time.Now().Format("2006-01-02")
	pastDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	_, err := svc.CreateRecurringRule(model.RecurringRule{
		AccountID: acc.ID,
		Amount:    5000,
		Payee:     "Streaming Service",
		Frequency: model.FreqMonthly,
		StartDate: pastDate,
		NextDue:   pastDate,
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Also create a rule with next_due in the future — should NOT be generated
	futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	_, err = svc.CreateRecurringRule(model.RecurringRule{
		AccountID: acc.ID,
		Amount:    10000,
		Payee:     "Future Bill",
		Frequency: model.FreqMonthly,
		StartDate: futureDate,
		NextDue:   futureDate,
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create future rule: %v", err)
	}

	count, err := svc.GenerateRecurring()
	if err != nil {
		t.Fatalf("GenerateRecurring: %v", err)
	}
	if count != 1 {
		t.Errorf("generated %d transactions, want 1", count)
	}

	// Verify transaction was created
	txs, _ := svc.ListTransactions(model.TxFilter{Search: "Streaming Service"})
	if len(txs) != 1 {
		t.Errorf("expected 1 streaming tx, got %d", len(txs))
	}

	// Verify next_due was advanced
	rules, _ := svc.ListRecurringRules()
	for _, r := range rules {
		if r.Payee == "Streaming Service" {
			if r.NextDue <= today {
				t.Errorf("next_due should have been advanced past %s, got %s", today, r.NextDue)
			}
		}
	}
}

func TestExtraGenerateRecurring_EndDatePassed(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	pastDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, -15).Format("2006-01-02")

	_, err := svc.CreateRecurringRule(model.RecurringRule{
		AccountID: acc.ID,
		Amount:    2000,
		Payee:     "Expired Sub",
		Frequency: model.FreqMonthly,
		StartDate: pastDate,
		NextDue:   pastDate,
		EndDate:   &endDate,
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	count, err := svc.GenerateRecurring()
	if err != nil {
		t.Fatalf("GenerateRecurring: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 (end date passed), got %d", count)
	}
}

// ---------- advanceDate ----------

func TestExtraAdvanceDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		date     string
		freq     model.Frequency
		expected string
	}{
		{"weekly", "2025-03-01", model.FreqWeekly, "2025-03-08"},
		{"biweekly", "2025-03-01", model.FreqBiweekly, "2025-03-15"},
		{"monthly", "2025-01-31", model.FreqMonthly, "2025-03-03"},
		{"monthly_normal", "2025-03-01", model.FreqMonthly, "2025-04-01"},
		{"yearly", "2025-03-01", model.FreqYearly, "2026-03-01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := advanceDate(tc.date, tc.freq)
			if got != tc.expected {
				t.Errorf("advanceDate(%q, %q) = %q, want %q", tc.date, tc.freq, got, tc.expected)
			}
		})
	}
}

// ---------- IncomeReport ----------

func TestExtraIncomeReport(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	salary, _ := svc.FindCategoryByName("Salary")

	// Create income transactions across months
	for _, tx := range []model.Transaction{
		{AccountID: acc.ID, CategoryID: &salary.ID, Amount: 500000, Date: "2025-01-15", Payee: "Employer", Type: model.TxIncome},
		{AccountID: acc.ID, CategoryID: &salary.ID, Amount: 500000, Date: "2025-02-15", Payee: "Employer", Type: model.TxIncome},
		{AccountID: acc.ID, CategoryID: &salary.ID, Amount: 500000, Date: "2025-03-15", Payee: "Employer", Type: model.TxIncome},
	} {
		if _, err := svc.CreateTransaction(tx); err != nil {
			t.Fatalf("create income tx: %v", err)
		}
	}

	// Monthly grouping
	periods, err := svc.IncomeReport("2025-01-01", "2025-03-31", "monthly")
	if err != nil {
		t.Fatalf("IncomeReport monthly: %v", err)
	}
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %d", len(periods))
	}
	for _, p := range periods {
		if p.Total != 500000 {
			t.Errorf("period %s total = %d, want 500000", p.Period, p.Total)
		}
	}

	// Quarterly grouping
	periods, err = svc.IncomeReport("2025-01-01", "2025-03-31", "quarterly")
	if err != nil {
		t.Fatalf("IncomeReport quarterly: %v", err)
	}
	if len(periods) != 1 {
		t.Fatalf("expected 1 quarterly period, got %d", len(periods))
	}
	if periods[0].Total != 1500000 {
		t.Errorf("Q1 total = %d, want 1500000", periods[0].Total)
	}

	// Yearly grouping
	periods, err = svc.IncomeReport("2025-01-01", "2025-12-31", "yearly")
	if err != nil {
		t.Fatalf("IncomeReport yearly: %v", err)
	}
	if len(periods) != 1 {
		t.Fatalf("expected 1 yearly period, got %d", len(periods))
	}
	if periods[0].Total != 1500000 {
		t.Errorf("yearly total = %d, want 1500000", periods[0].Total)
	}
}

// ---------- CashFlowReport ----------

func TestExtraCashFlowReport(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	for _, tx := range []model.Transaction{
		{AccountID: acc.ID, Amount: 500000, Date: "2025-01-15", Payee: "Employer", Type: model.TxIncome},
		{AccountID: acc.ID, Amount: 100000, Date: "2025-01-20", Payee: "Rent", Type: model.TxExpense},
		{AccountID: acc.ID, Amount: 50000, Date: "2025-01-25", Payee: "Groceries", Type: model.TxExpense},
		{AccountID: acc.ID, Amount: 500000, Date: "2025-02-15", Payee: "Employer", Type: model.TxIncome},
		{AccountID: acc.ID, Amount: 120000, Date: "2025-02-20", Payee: "Rent", Type: model.TxExpense},
	} {
		if _, err := svc.CreateTransaction(tx); err != nil {
			t.Fatalf("create tx: %v", err)
		}
	}

	report, err := svc.CashFlowReport("2025-01-01", "2025-02-28")
	if err != nil {
		t.Fatalf("CashFlowReport: %v", err)
	}
	if len(report) != 2 {
		t.Fatalf("expected 2 months, got %d", len(report))
	}

	// January
	jan := report[0]
	if jan.Month != "2025-01" {
		t.Errorf("first month = %q, want 2025-01", jan.Month)
	}
	if jan.Income != 500000 {
		t.Errorf("jan income = %d, want 500000", jan.Income)
	}
	if jan.Expenses != 150000 {
		t.Errorf("jan expenses = %d, want 150000", jan.Expenses)
	}
	if jan.Net != 350000 {
		t.Errorf("jan net = %d, want 350000", jan.Net)
	}

	// February
	feb := report[1]
	if feb.Month != "2025-02" {
		t.Errorf("second month = %q, want 2025-02", feb.Month)
	}
	if feb.Net != 380000 {
		t.Errorf("feb net = %d, want 380000", feb.Net)
	}
}

// ---------- 401k ----------

func TestExtraContribute401k(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc, err := svc.CreateAccount("My 401k", model.Account401k, 0)
	if err != nil {
		t.Fatalf("create 401k account: %v", err)
	}

	// First contribution
	status, err := svc.Contribute401k(acc.ID, 2025, 50000, 25000)
	if err != nil {
		t.Fatalf("Contribute401k: %v", err)
	}
	if status.EmployeeContrib != 50000 {
		t.Errorf("employee contrib = %d, want 50000", status.EmployeeContrib)
	}
	if status.EmployerMatch != 25000 {
		t.Errorf("employer match = %d, want 25000", status.EmployerMatch)
	}

	// Second contribution should accumulate
	status, err = svc.Contribute401k(acc.ID, 2025, 50000, 25000)
	if err != nil {
		t.Fatalf("Contribute401k second: %v", err)
	}
	if status.EmployeeContrib != 100000 {
		t.Errorf("accumulated employee contrib = %d, want 100000", status.EmployeeContrib)
	}
	if status.EmployerMatch != 50000 {
		t.Errorf("accumulated employer match = %d, want 50000", status.EmployerMatch)
	}
	if status.TotalContrib != 150000 {
		t.Errorf("total contrib = %d, want 150000", status.TotalContrib)
	}
}

func TestExtraGet401kStatus_NoRecord(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc, err := svc.CreateAccount("Empty 401k", model.Account401k, 0)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Should return a default status, not error
	status, err := svc.Get401kStatus(acc.ID, 2025)
	if err != nil {
		t.Fatalf("Get401kStatus: %v", err)
	}
	if status.AnnualLimit != 2350000 {
		t.Errorf("default annual limit = %d, want 2350000", status.AnnualLimit)
	}
	if status.EmployeeContrib != 0 {
		t.Errorf("expected 0 contrib, got %d", status.EmployeeContrib)
	}
}

func TestExtraSet401kMatchPercent(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc, err := svc.CreateAccount("Match 401k", model.Account401k, 0)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if err := svc.Set401kMatchPercent(acc.ID, 2025, 6.0); err != nil {
		t.Fatalf("Set401kMatchPercent: %v", err)
	}

	status, err := svc.Get401kStatus(acc.ID, 2025)
	if err != nil {
		t.Fatalf("Get401kStatus: %v", err)
	}
	if status.MatchPercent != 6.0 {
		t.Errorf("match percent = %f, want 6.0", status.MatchPercent)
	}
}

func TestExtraSet401kAnnualLimit(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc, err := svc.CreateAccount("Limit 401k", model.Account401k, 0)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if err := svc.Set401kAnnualLimit(acc.ID, 2025, 3000000); err != nil {
		t.Fatalf("Set401kAnnualLimit: %v", err)
	}

	status, err := svc.Get401kStatus(acc.ID, 2025)
	if err != nil {
		t.Fatalf("Get401kStatus: %v", err)
	}
	if status.AnnualLimit != 3000000 {
		t.Errorf("annual limit = %d, want 3000000", status.AnnualLimit)
	}
}

// ---------- GetAccountBalanceHistory ----------

func TestExtraGetAccountBalanceHistory(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	origGetEquity := GetEquityValue
	origGetAccountEquity := GetAccountEquityValue
	GetEquityValue = nil
	GetAccountEquityValue = nil
	t.Cleanup(func() {
		GetEquityValue = origGetEquity
		GetAccountEquityValue = origGetAccountEquity
	})

	acc, err := svc.CreateAccount("History Checking", model.AccountChecking, 0)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Snapshot creates balance history entries
	_, err = svc.SnapshotNetWorth()
	if err != nil {
		t.Fatalf("SnapshotNetWorth: %v", err)
	}

	balances, err := svc.GetAccountBalanceHistory(acc.ID, 30)
	if err != nil {
		t.Fatalf("GetAccountBalanceHistory: %v", err)
	}
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance entry, got %d", len(balances))
	}
	if balances[0] != 0 {
		t.Errorf("balance = %d, want 0", balances[0])
	}
}

func TestExtraGetAccountBalanceHistory_Empty(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	balances, err := svc.GetAccountBalanceHistory(acc.ID, 30)
	if err != nil {
		t.Fatalf("GetAccountBalanceHistory: %v", err)
	}
	if len(balances) != 0 {
		t.Errorf("expected 0 balances for no history, got %d", len(balances))
	}
}

// ---------- ListInvestmentAccounts ----------

func TestExtraListInvestmentAccounts(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Create various account types
	if _, err := svc.CreateAccount("Checking", model.AccountChecking, 0); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	if _, err := svc.CreateAccount("Savings", model.AccountSavings, 0); err != nil {
		t.Fatalf("create savings: %v", err)
	}
	if _, err := svc.CreateAccount("Brokerage", model.AccountBrokerage, 0); err != nil {
		t.Fatalf("create brokerage: %v", err)
	}
	if _, err := svc.CreateAccount("My 401k", model.Account401k, 0); err != nil {
		t.Fatalf("create 401k: %v", err)
	}
	if _, err := svc.CreateAccount("Investment", model.AccountInvestment, 0); err != nil {
		t.Fatalf("create investment: %v", err)
	}
	if _, err := svc.CreateAccountFull("Managed", model.AccountManaged, 0, "balance"); err != nil {
		t.Fatalf("create managed: %v", err)
	}

	invAccs, err := svc.ListInvestmentAccounts()
	if err != nil {
		t.Fatalf("ListInvestmentAccounts: %v", err)
	}
	// Should include: brokerage, 401k, investment, managed (not checking, savings)
	if len(invAccs) != 4 {
		t.Errorf("expected 4 investment accounts, got %d", len(invAccs))
		for _, a := range invAccs {
			t.Logf("  %s (%s)", a.Name, a.Type)
		}
	}

	// Verify no non-investment accounts
	for _, a := range invAccs {
		switch a.Type {
		case model.AccountInvestment, model.AccountBrokerage, model.Account401k, model.AccountManaged:
			// ok
		default:
			t.Errorf("unexpected account type %q in investment list", a.Type)
		}
	}
}

// ---------- DeleteCategory ----------

func TestExtraDeleteCategory(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Create a custom category, then delete it
	cat, err := svc.CreateCategory("Test Group", "Deletable Cat")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	if err := svc.DeleteCategory(cat.ID); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	_, err = svc.FindCategoryByName("Deletable Cat")
	if err == nil {
		t.Error("expected error finding deleted category, got nil")
	}
}

func TestExtraDeleteCategory_NonExistent(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Deleting non-existent ID should not error (SQLite DELETE is idempotent)
	if err := svc.DeleteCategory(99999); err != nil {
		t.Errorf("DeleteCategory non-existent: %v", err)
	}
}
