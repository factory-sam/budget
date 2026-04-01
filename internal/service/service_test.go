package service

import (
	"path/filepath"
	"testing"

	"github.com/sam/budget/internal/db"
	"github.com/sam/budget/internal/model"
)

const testCategoryGroceries = "Groceries"

// newTestService creates an isolated Service backed by a temp SQLite DB with seeded categories.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// db.Open() → migrateCategories() may have inserted the "Investments" group,
	// causing SeedCategories() to early-return (count > 0). Clear it first.
	if _, err := sqlDB.Exec("DELETE FROM categories"); err != nil {
		t.Fatalf("delete categories: %v", err)
	}
	if _, err := sqlDB.Exec("DELETE FROM category_groups"); err != nil {
		t.Fatalf("delete category_groups: %v", err)
	}
	if err := db.SeedCategories(sqlDB); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return New(sqlDB)
}

// newTestAccount is a shortcut to create a checking account with zero balance.
func newTestAccount(t *testing.T, svc *Service) *model.Account {
	t.Helper()
	acc, err := svc.CreateAccount("Checking", model.AccountChecking, 0)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return acc
}

// ---------- 1. Account CRUD ----------

func TestAccountCRUD(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Create
	acc, err := svc.CreateAccount("Checking Main", model.AccountChecking, 100000)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.ID == 0 {
		t.Fatal("expected non-zero account ID")
	}
	if acc.Name != "Checking Main" {
		t.Errorf("expected name 'Checking Main', got %q", acc.Name)
	}
	if acc.Balance != 100000 {
		t.Errorf("expected balance 100000, got %d", acc.Balance)
	}

	// Get
	got, err := svc.GetAccount(acc.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.Name != acc.Name {
		t.Errorf("GetAccount name = %q, want %q", got.Name, acc.Name)
	}

	// List
	_, err = svc.CreateAccount("Savings", model.AccountSavings, 50000)
	if err != nil {
		t.Fatalf("CreateAccount savings: %v", err)
	}
	accs, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accs) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(accs))
	}

	// UpdateAccountBalance
	if err := svc.UpdateAccountBalance(acc.ID, -25000); err != nil {
		t.Fatalf("UpdateAccountBalance: %v", err)
	}
	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 75000 {
		t.Errorf("balance after update = %d, want 75000", got.Balance)
	}

	// Delete
	if err := svc.DeleteAccount(acc.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	accs, _ = svc.ListAccounts()
	if len(accs) != 1 {
		t.Errorf("expected 1 account after delete, got %d", len(accs))
	}
}

func TestCreateAccountFull(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	acc, err := svc.CreateAccountFull("Managed Acct", model.AccountManaged, 0, "balance")
	if err != nil {
		t.Fatalf("CreateAccountFull: %v", err)
	}
	if acc.TrackingMode != "balance" {
		t.Errorf("tracking_mode = %q, want 'balance'", acc.TrackingMode)
	}
}

// ---------- 2. Transaction CRUD & balance updates ----------

func TestCreateTransaction_UpdatesBalance(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Expense should decrease balance
	tx, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    5000,
		Date:      "2025-03-15",
		Payee:     "Coffee Shop",
		Type:      model.TxExpense,
	})
	if err != nil {
		t.Fatalf("CreateTransaction expense: %v", err)
	}
	if tx.ID == 0 {
		t.Fatal("expected non-zero tx ID")
	}

	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != -5000 {
		t.Errorf("balance after expense = %d, want -5000", got.Balance)
	}

	// Income should increase balance
	_, err = svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    200000,
		Date:      "2025-03-01",
		Payee:     "Employer Inc",
		Type:      model.TxIncome,
	})
	if err != nil {
		t.Fatalf("CreateTransaction income: %v", err)
	}
	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 195000 {
		t.Errorf("balance after income = %d, want 195000", got.Balance)
	}
}

func TestListTransactions_Filters(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Create transactions across months
	for _, tx := range []model.Transaction{
		{AccountID: acc.ID, Amount: 1000, Date: "2025-01-10", Payee: "Grocery Store", Type: model.TxExpense},
		{AccountID: acc.ID, Amount: 2000, Date: "2025-01-20", Payee: "Gas Station", Type: model.TxExpense},
		{AccountID: acc.ID, Amount: 3000, Date: "2025-02-05", Payee: "Electric Company", Type: model.TxExpense},
		{AccountID: acc.ID, Amount: 500000, Date: "2025-02-01", Payee: "Employer Payroll", Type: model.TxIncome},
	} {
		if _, err := svc.CreateTransaction(tx); err != nil {
			t.Fatalf("create tx: %v", err)
		}
	}

	// Filter by month
	txs, err := svc.ListTransactions(model.TxFilter{Month: "2025-01"})
	if err != nil {
		t.Fatalf("ListTransactions month filter: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("month filter: got %d txs, want 2", len(txs))
	}

	// Filter by search
	txs, err = svc.ListTransactions(model.TxFilter{Search: "grocery"})
	if err != nil {
		t.Fatalf("ListTransactions search filter: %v", err)
	}
	if len(txs) != 1 {
		t.Errorf("search filter: got %d txs, want 1", len(txs))
	}

	// Filter by limit
	txs, err = svc.ListTransactions(model.TxFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListTransactions limit: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("limit filter: got %d txs, want 2", len(txs))
	}

	// Filter by type
	txs, err = svc.ListTransactions(model.TxFilter{Type: model.TxIncome})
	if err != nil {
		t.Fatalf("ListTransactions type filter: %v", err)
	}
	if len(txs) != 1 {
		t.Errorf("type filter: got %d txs, want 1", len(txs))
	}
}

// ---------- 3. DeleteTransaction ----------

func TestDeleteTransaction_ReversesBalance(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	tx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    10000,
		Date:      "2025-03-10",
		Payee:     "Amazon",
		Type:      model.TxExpense,
	})
	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != -10000 {
		t.Fatalf("balance before delete = %d, want -10000", got.Balance)
	}

	if err := svc.DeleteTransaction(tx.ID); err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}
	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 0 {
		t.Errorf("balance after delete = %d, want 0", got.Balance)
	}

	// Also verify the tx is gone
	txs, _ := svc.ListTransactions(model.TxFilter{})
	if len(txs) != 0 {
		t.Errorf("expected 0 txs after delete, got %d", len(txs))
	}
}

func TestDeleteTransaction_Income(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	tx, _ := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID,
		Amount:    50000,
		Date:      "2025-03-01",
		Payee:     "Paycheck",
		Type:      model.TxIncome,
	})
	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != 50000 {
		t.Fatalf("balance after income = %d, want 50000", got.Balance)
	}

	if err := svc.DeleteTransaction(tx.ID); err != nil {
		t.Fatalf("DeleteTransaction income: %v", err)
	}
	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 0 {
		t.Errorf("balance after deleting income = %d, want 0", got.Balance)
	}
}

// ---------- 4. Category operations ----------

func TestListCategories(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cats, err := svc.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("expected seeded categories, got none")
	}

	// Verify some known seeded categories exist
	found := map[string]bool{}
	for _, c := range cats {
		found[c.Name] = true
	}
	for _, name := range []string{testCategoryGroceries, "Salary", "Rent/Mortgage", "Transfer"} {
		if !found[name] {
			t.Errorf("expected seeded category %q not found", name)
		}
	}
}

func TestFindCategoryByName_Exact(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("FindCategoryByName exact: %v", err)
	}
	if cat.Name != testCategoryGroceries {
		t.Errorf("expected %q, got %q", testCategoryGroceries, cat.Name)
	}
	if cat.GroupName != "Food & Drink" {
		t.Errorf("expected group 'Food & Drink', got %q", cat.GroupName)
	}
}

func TestFindCategoryByName_CaseInsensitive(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName("groceries")
	if err != nil {
		t.Fatalf("FindCategoryByName case insensitive: %v", err)
	}
	if cat.Name != testCategoryGroceries {
		t.Errorf("expected 'Groceries', got %q", cat.Name)
	}
}

func TestFindCategoryByName_Prefix(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName("Groc")
	if err != nil {
		t.Fatalf("FindCategoryByName prefix: %v", err)
	}
	if cat.Name != testCategoryGroceries {
		t.Errorf("expected 'Groceries', got %q", cat.Name)
	}
}

func TestFindCategoryByName_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	_, err := svc.FindCategoryByName("NonexistentCategory12345")
	if err == nil {
		t.Error("expected error for nonexistent category, got nil")
	}
}

// ---------- 5. Budget operations ----------

func TestSetBudgetAndGetStatus(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	// Find Groceries category
	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("FindCategoryByName: %v", err)
	}

	// Set a budget for March 2025
	if err := svc.SetBudget(cat.ID, 2025, 3, 50000); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	// Create some grocery expenses
	for _, amt := range []int64{15000, 8000, 12000} {
		_, err := svc.CreateTransaction(model.Transaction{
			AccountID:  acc.ID,
			CategoryID: &cat.ID,
			Amount:     amt,
			Date:       "2025-03-10",
			Payee:      "Grocery Store",
			Type:       model.TxExpense,
		})
		if err != nil {
			t.Fatalf("create expense: %v", err)
		}
	}

	statuses, err := svc.GetBudgetStatus(2025, 3)
	if err != nil {
		t.Fatalf("GetBudgetStatus: %v", err)
	}

	// Find the Groceries status
	var groceryStatus *model.BudgetStatus
	for i, s := range statuses {
		if s.CategoryName == testCategoryGroceries {
			groceryStatus = &statuses[i]
			break
		}
	}
	if groceryStatus == nil {
		t.Fatal("Groceries not found in budget status")
	}

	if groceryStatus.AmountLimit != 50000 {
		t.Errorf("budget limit = %d, want 50000", groceryStatus.AmountLimit)
	}
	if groceryStatus.Spent != 35000 {
		t.Errorf("spent = %d, want 35000", groceryStatus.Spent)
	}
	if groceryStatus.Remaining != 15000 {
		t.Errorf("remaining = %d, want 15000", groceryStatus.Remaining)
	}
	if groceryStatus.Percent != 70.0 {
		t.Errorf("percent = %.1f, want 70.0", groceryStatus.Percent)
	}
}

func TestSetBudget_Upsert(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("FindCategoryByName: %v", err)
	}

	if err := svc.SetBudget(cat.ID, 2025, 3, 50000); err != nil {
		t.Fatalf("SetBudget initial: %v", err)
	}
	// Update the same budget
	if err := svc.SetBudget(cat.ID, 2025, 3, 75000); err != nil {
		t.Fatalf("SetBudget upsert: %v", err)
	}

	statuses, _ := svc.GetBudgetStatus(2025, 3)
	for _, s := range statuses {
		if s.CategoryName == testCategoryGroceries {
			if s.AmountLimit != 75000 {
				t.Errorf("upserted budget limit = %d, want 75000", s.AmountLimit)
			}
			return
		}
	}
	t.Fatal("Groceries not found in budget status after upsert")
}

// ---------- 6. RecalculateAccountBalance ----------

func TestRecalculateAccountBalance(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Clear the equity value funcs for this test to avoid interference
	origGetAccountEquity := GetAccountEquityValue
	GetAccountEquityValue = nil
	t.Cleanup(func() { GetAccountEquityValue = origGetAccountEquity })

	acc := newTestAccount(t, svc)

	// Create transactions
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 100000, Date: "2025-03-01",
		Payee: "Salary", Type: model.TxIncome,
	}); err != nil {
		t.Fatalf("create salary tx: %v", err)
	}
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 30000, Date: "2025-03-05",
		Payee: "Rent", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create rent tx: %v", err)
	}
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 5000, Date: "2025-03-10",
		Payee: "Coffee", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create coffee tx: %v", err)
	}

	// Verify balance is correct: 100000 - 30000 - 5000 = 65000
	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != 65000 {
		t.Fatalf("balance before corruption = %d, want 65000", got.Balance)
	}

	// Corrupt the balance directly
	if _, err := svc.db.Exec("UPDATE accounts SET balance = 999999 WHERE id = ?", acc.ID); err != nil {
		t.Fatalf("corrupt balance: %v", err)
	}
	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 999999 {
		t.Fatal("failed to corrupt balance")
	}

	// Recalculate
	if err := svc.RecalculateAccountBalance(acc.ID); err != nil {
		t.Fatalf("RecalculateAccountBalance: %v", err)
	}

	got, _ = svc.GetAccount(acc.ID)
	if got.Balance != 65000 {
		t.Errorf("balance after recalculate = %d, want 65000", got.Balance)
	}
}

// ---------- 7. AutoCategorize ----------

func TestAutoCategorize(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("FindCategoryByName: %v", err)
	}

	// Add auto-cat rule
	if err := svc.AddAutoCatRule("whole foods", cat.ID); err != nil {
		t.Fatalf("AddAutoCatRule: %v", err)
	}

	// Test matching payee
	catID := svc.AutoCategorize("WHOLE FOODS MARKET #123")
	if catID == nil {
		t.Fatal("AutoCategorize returned nil for matching payee")
	}
	if *catID != cat.ID {
		t.Errorf("AutoCategorize returned category %d, want %d", *catID, cat.ID)
	}

	// Test non-matching payee
	catID = svc.AutoCategorize("Target")
	if catID != nil {
		t.Errorf("expected nil for non-matching payee, got %d", *catID)
	}
}

func TestAutoCategorize_BuiltinPatterns(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// "dividend" should match the built-in pattern → Dividend Income category
	catID := svc.AutoCategorize("VANGUARD DIVIDEND REINVESTMENT")
	if catID == nil {
		t.Fatal("AutoCategorize returned nil for 'dividend' payee")
	}

	cat, _ := svc.FindCategoryByName("Dividend Income")
	if *catID != cat.ID {
		t.Errorf("dividend match: got category %d, want %d", *catID, cat.ID)
	}
}

func TestListAutoCatRules(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.FindCategoryByName(testCategoryGroceries)
	if err != nil {
		t.Fatalf("FindCategoryByName: %v", err)
	}
	if err := svc.AddAutoCatRule("costco", cat.ID); err != nil {
		t.Fatalf("AddAutoCatRule costco: %v", err)
	}
	if err = svc.AddAutoCatRule("trader joe", cat.ID); err != nil {
		t.Fatalf("AddAutoCatRule trader joe: %v", err)
	}

	rules, err := svc.ListAutoCatRules()
	if err != nil {
		t.Fatalf("ListAutoCatRules: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
}

// ---------- 8. Net Worth Snapshot ----------

func TestSnapshotNetWorth(t *testing.T) {
	svc := newTestService(t)

	// Clear equity funcs for this test (not parallel — writes package-level vars)
	origGetEquity := GetEquityValue
	origGetAccountEquity := GetAccountEquityValue
	GetEquityValue = nil
	GetAccountEquityValue = nil
	t.Cleanup(func() {
		GetEquityValue = origGetEquity
		GetAccountEquityValue = origGetAccountEquity
	})

	// Create asset accounts
	checking, err := svc.CreateAccount("Checking", model.AccountChecking, 500000)
	if err != nil {
		t.Fatalf("CreateAccount checking: %v", err)
	}
	savings, err := svc.CreateAccount("Savings", model.AccountSavings, 1000000)
	if err != nil {
		t.Fatalf("CreateAccount savings: %v", err)
	}

	// Create a liability
	cc, err := svc.CreateAccount("Credit Card", model.AccountCreditCard, 50000)
	if err != nil {
		t.Fatalf("CreateAccount cc: %v", err)
	}

	// Create some transactions to set correct balances
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: checking.ID, Amount: 500000, Date: "2025-03-01",
		Payee: "Salary", Type: model.TxIncome,
	}); err != nil {
		t.Fatalf("create checking tx: %v", err)
	}
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: savings.ID, Amount: 1000000, Date: "2025-03-01",
		Payee: "Transfer", Type: model.TxIncome,
	}); err != nil {
		t.Fatalf("create savings tx: %v", err)
	}
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: cc.ID, Amount: 50000, Date: "2025-03-05",
		Payee: "Store", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create cc tx: %v", err)
	}

	snap, err := svc.SnapshotNetWorth()
	if err != nil {
		t.Fatalf("SnapshotNetWorth: %v", err)
	}

	// After recalculate: checking = 500000, savings = 1000000, cc = -50000
	// Assets = checking + savings = 1500000
	// Liabilities = cc (balance is -50000, but that's the stored value)
	// Net worth = assets - liabilities

	// The credit card account started at 50000, then an expense of 50000
	// decreases it by 50000, so balance = 0
	// Actually re-check: CreateAccount sets balance to 50000, then CreateTransaction
	// expense subtracts 50000 → balance = 0. But RecalculateAccountBalance
	// recalculates from transactions only (sum of income - expenses).
	// For cc: one expense of 50000 → sum = -50000
	// So after recalculate: cc balance = -50000

	// checking: one income 500000, initial balance was 0 + 500000 = 500000
	// But RecalculateAccountBalance recalculates from transactions:
	// checking income 500000 → balance = 500000
	// savings income 1000000 → balance = 1000000
	// cc expense 50000 → balance = -50000

	expectedAssets := int64(500000 + 1000000) // checking + savings
	expectedLiabilities := int64(-50000)      // cc after recalculate
	expectedNetWorth := expectedAssets - expectedLiabilities

	if snap.TotalAssets != expectedAssets {
		t.Errorf("total_assets = %d, want %d", snap.TotalAssets, expectedAssets)
	}
	if snap.TotalLiabilities != expectedLiabilities {
		t.Errorf("total_liabilities = %d, want %d", snap.TotalLiabilities, expectedLiabilities)
	}
	if snap.NetWorth != expectedNetWorth {
		t.Errorf("net_worth = %d, want %d", snap.NetWorth, expectedNetWorth)
	}
}

func TestSnapshotNetWorth_History(t *testing.T) {
	svc := newTestService(t)

	// Not parallel — writes package-level vars
	origGetEquity := GetEquityValue
	origGetAccountEquity := GetAccountEquityValue
	GetEquityValue = nil
	GetAccountEquityValue = nil
	t.Cleanup(func() {
		GetEquityValue = origGetEquity
		GetAccountEquityValue = origGetAccountEquity
	})

	if _, err := svc.CreateAccount("Checking", model.AccountChecking, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	_, err := svc.SnapshotNetWorth()
	if err != nil {
		t.Fatalf("SnapshotNetWorth: %v", err)
	}

	history, err := svc.GetNetWorthHistory(10)
	if err != nil {
		t.Fatalf("GetNetWorthHistory: %v", err)
	}
	if len(history) == 0 {
		t.Error("expected at least 1 snapshot in history")
	}
}

// ---------- Additional edge cases ----------

func TestGetAccountByName(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	if _, err := svc.CreateAccount("My Checking", model.AccountChecking, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	acc, err := svc.GetAccountByName("my checking")
	if err != nil {
		t.Fatalf("GetAccountByName: %v", err)
	}
	if acc.Name != "My Checking" {
		t.Errorf("got %q, want 'My Checking'", acc.Name)
	}
}

func TestListCategoryGroups(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	groups, err := svc.ListCategoryGroups()
	if err != nil {
		t.Fatalf("ListCategoryGroups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected seeded category groups, got none")
	}

	// Verify Income group exists
	found := false
	for _, g := range groups {
		if g.Name == "Income" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Income' group in category groups")
	}
}

func TestCreateCategory(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	cat, err := svc.CreateCategory("Custom Group", "Custom Cat")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if cat.Name != "Custom Cat" {
		t.Errorf("name = %q, want 'Custom Cat'", cat.Name)
	}
	if cat.GroupName != "Custom Group" {
		t.Errorf("group = %q, want 'Custom Group'", cat.GroupName)
	}

	// Verify it's findable
	found, err := svc.FindCategoryByName("Custom Cat")
	if err != nil {
		t.Fatalf("find created category: %v", err)
	}
	if found.ID != cat.ID {
		t.Errorf("found ID = %d, want %d", found.ID, cat.ID)
	}
}

func TestTransferIn_IsCredit(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	_, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, Amount: 25000, Date: "2025-03-01",
		Payee: "Transfer from Savings", Type: model.TxTransferIn,
	})
	if err != nil {
		t.Fatalf("CreateTransaction transfer_in: %v", err)
	}

	got, _ := svc.GetAccount(acc.ID)
	if got.Balance != 25000 {
		t.Errorf("balance after transfer_in = %d, want 25000", got.Balance)
	}
}

func TestSpendingReport(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	acc := newTestAccount(t, svc)

	groceries, _ := svc.FindCategoryByName(testCategoryGroceries)
	restaurants, _ := svc.FindCategoryByName("Restaurants")

	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, CategoryID: &groceries.ID,
		Amount: 10000, Date: "2025-03-10", Payee: "Store", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create grocery tx: %v", err)
	}
	if _, err := svc.CreateTransaction(model.Transaction{
		AccountID: acc.ID, CategoryID: &restaurants.ID,
		Amount: 5000, Date: "2025-03-15", Payee: "Diner", Type: model.TxExpense,
	}); err != nil {
		t.Fatalf("create restaurant tx: %v", err)
	}

	report, err := svc.SpendingReport(2025, 3)
	if err != nil {
		t.Fatalf("SpendingReport: %v", err)
	}
	if len(report) != 2 {
		t.Fatalf("expected 2 spending categories, got %d", len(report))
	}

	// Total should be 15000, groceries 66.7%, restaurants 33.3%
	var total int64
	for _, r := range report {
		total += r.Amount
	}
	if total != 15000 {
		t.Errorf("total spending = %d, want 15000", total)
	}
}
