package service

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sam/budget/internal/model"
)

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) DB() *sql.DB { return s.db }

// --- Accounts ---

func (s *Service) CreateAccount(name string, typ model.AccountType, balanceCents int64) (*model.Account, error) {
	return s.CreateAccountFull(name, typ, balanceCents, "holdings")
}

func (s *Service) CreateAccountFull(name string, typ model.AccountType, balanceCents int64, trackingMode string) (*model.Account, error) {
	if trackingMode == "" {
		trackingMode = "holdings"
	}
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO accounts (name, type, balance, tracking_mode, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, typ, balanceCents, trackingMode, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	id, _ := res.LastInsertId()
	return &model.Account{ID: id, Name: name, Type: typ, Balance: balanceCents, Currency: "USD", TrackingMode: trackingMode, CreatedAt: now, UpdatedAt: now}, nil
}

func scanAccount(scanner interface{ Scan(...interface{}) error }) (*model.Account, error) {
	var a model.Account
	err := scanner.Scan(&a.ID, &a.Name, &a.Type, &a.Balance, &a.Currency, &a.TrackingMode, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const accountCols = "id, name, type, balance, currency, tracking_mode, created_at, updated_at"

func (s *Service) ListAccounts() ([]model.Account, error) {
	rows, err := s.db.Query("SELECT " + accountCols + " FROM accounts ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accs []model.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accs = append(accs, *a)
	}
	return accs, nil
}

func (s *Service) GetAccount(id int64) (*model.Account, error) {
	return scanAccount(s.db.QueryRow("SELECT "+accountCols+" FROM accounts WHERE id = ?", id))
}

func (s *Service) GetAccountByName(name string) (*model.Account, error) {
	return scanAccount(s.db.QueryRow("SELECT "+accountCols+" FROM accounts WHERE LOWER(name) = LOWER(?)", name))
}

func (s *Service) ListInvestmentAccounts() ([]model.Account, error) {
	rows, err := s.db.Query("SELECT " + accountCols + " FROM accounts WHERE type IN ('investment','brokerage','401k','managed') ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accs []model.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accs = append(accs, *a)
	}
	return accs, nil
}

func (s *Service) DeleteAccount(id int64) error {
	_, err := s.db.Exec("DELETE FROM accounts WHERE id = ?", id)
	return err
}

func (s *Service) UpdateAccountBalance(id int64, delta int64) error {
	_, err := s.db.Exec("UPDATE accounts SET balance = balance + ?, updated_at = ? WHERE id = ?", delta, time.Now(), id)
	return err
}

func (s *Service) RecalculateAccountBalance(id int64) error {
	acc, err := s.GetAccount(id)
	if err != nil {
		return err
	}

	// For managed/brokerage accounts, the balance IS the portfolio value.
	// Transactions (dividends) were inserted without balance effect.
	if (acc.Type == model.AccountManaged || acc.Type == model.AccountBrokerage) && GetAccountEquityValue != nil {
		eqVal, err := GetAccountEquityValue(id)
		if err != nil {
			return err
		}
		_, err = s.db.Exec("UPDATE accounts SET balance = ?, updated_at = ? WHERE id = ?", eqVal, time.Now(), id)
		return err
	}

	// For cash-like accounts, sum all transactions
	var balance int64
	err = s.db.QueryRow(`
		SELECT COALESCE(
			SUM(CASE WHEN type IN ('income', 'transfer_in') THEN amount ELSE -amount END),
			0)
		FROM transactions WHERE account_id = ?`, id).Scan(&balance)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE accounts SET balance = ?, updated_at = ? WHERE id = ?", balance, time.Now(), id)
	return err
}

func (s *Service) RecalculateAllBalances() error {
	accs, err := s.ListAccounts()
	if err != nil {
		return err
	}
	for _, a := range accs {
		if err := s.RecalculateAccountBalance(a.ID); err != nil {
			return err
		}
	}
	return nil
}

// --- Categories ---

func (s *Service) ListCategories() ([]model.Category, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.group_id, g.name, c.name, c.icon, c.sort_order
		FROM categories c JOIN category_groups g ON c.group_id = g.id
		ORDER BY g.sort_order, c.sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.GroupID, &c.GroupName, &c.Name, &c.Icon, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (s *Service) ListCategoryGroups() ([]model.CategoryGroup, error) {
	rows, err := s.db.Query("SELECT id, name, sort_order FROM category_groups ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.CategoryGroup
	for rows.Next() {
		var g model.CategoryGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Service) FindCategoryByName(name string) (*model.Category, error) {
	// try exact match first
	var c model.Category
	err := s.db.QueryRow(`
		SELECT c.id, c.group_id, g.name, c.name, c.icon, c.sort_order
		FROM categories c JOIN category_groups g ON c.group_id = g.id
		WHERE LOWER(c.name) = LOWER(?)`, name).
		Scan(&c.ID, &c.GroupID, &c.GroupName, &c.Name, &c.Icon, &c.SortOrder)
	if err == nil {
		return &c, nil
	}

	// fall back to prefix/substring match
	err = s.db.QueryRow(`
		SELECT c.id, c.group_id, g.name, c.name, c.icon, c.sort_order
		FROM categories c JOIN category_groups g ON c.group_id = g.id
		WHERE LOWER(c.name) LIKE LOWER(?) || '%'
		ORDER BY LENGTH(c.name) LIMIT 1`, name).
		Scan(&c.ID, &c.GroupID, &c.GroupName, &c.Name, &c.Icon, &c.SortOrder)
	if err == nil {
		return &c, nil
	}

	return nil, fmt.Errorf("category %q not found", name)
}

func (s *Service) CreateCategory(groupName, catName string) (*model.Category, error) {
	var groupID int64
	err := s.db.QueryRow("SELECT id FROM category_groups WHERE LOWER(name) = LOWER(?)", groupName).Scan(&groupID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec("INSERT INTO category_groups (name, sort_order) VALUES (?, (SELECT COALESCE(MAX(sort_order),0)+1 FROM category_groups))", groupName)
		if err != nil {
			return nil, err
		}
		groupID, _ = res.LastInsertId()
	} else if err != nil {
		return nil, err
	}

	res, err := s.db.Exec("INSERT INTO categories (group_id, name, sort_order) VALUES (?, ?, (SELECT COALESCE(MAX(sort_order),0)+1 FROM categories WHERE group_id = ?))", groupID, catName, groupID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Category{ID: id, GroupID: groupID, GroupName: groupName, Name: catName}, nil
}

func (s *Service) DeleteCategory(id int64) error {
	_, err := s.db.Exec("DELETE FROM categories WHERE id = ?", id)
	return err
}

// --- Transactions ---

func (s *Service) CreateTransaction(tx model.Transaction) (*model.Transaction, error) {
	if tx.Date == "" {
		tx.Date = time.Now().Format("2006-01-02")
	}
	res, err := s.db.Exec(
		"INSERT INTO transactions (account_id, category_id, amount, date, payee, note, type) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tx.AccountID, tx.CategoryID, tx.Amount, tx.Date, tx.Payee, tx.Note, tx.Type,
	)
	if err != nil {
		return nil, fmt.Errorf("create tx: %w", err)
	}
	tx.ID, _ = res.LastInsertId()
	tx.CreatedAt = time.Now()

	// update account balance
	delta := tx.Amount
	if !tx.Type.IsCredit() {
		delta = -delta
	}
	if err := s.UpdateAccountBalance(tx.AccountID, delta); err != nil {
		return nil, fmt.Errorf("update balance: %w", err)
	}

	return &tx, nil
}

func (s *Service) ListTransactions(f model.TxFilter) ([]model.Transaction, error) {
	q := `SELECT t.id, t.account_id, COALESCE(a.name,''), t.category_id, COALESCE(c.name,''),
		t.amount, t.date, t.payee, t.note, t.type, t.created_at
		FROM transactions t
		LEFT JOIN accounts a ON t.account_id = a.id
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE 1=1`
	var args []interface{}

	if f.Month != "" {
		q += " AND t.date LIKE ?"
		args = append(args, f.Month+"%")
	}
	if f.From != "" {
		q += " AND t.date >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND t.date <= ?"
		args = append(args, f.To)
	}
	if f.AccountID != nil {
		q += " AND t.account_id = ?"
		args = append(args, *f.AccountID)
	}
	if f.CategoryID != nil {
		q += " AND t.category_id = ?"
		args = append(args, *f.CategoryID)
	}
	if f.Uncategorized {
		q += " AND t.category_id IS NULL"
	}
	if f.Payee != "" {
		q += " AND LOWER(t.payee) LIKE ?"
		args = append(args, "%"+strings.ToLower(f.Payee)+"%")
	}
	if f.Type != "" {
		q += " AND t.type = ?"
		args = append(args, f.Type)
	}
	if f.Search != "" {
		q += " AND (LOWER(t.payee) LIKE ? OR LOWER(t.note) LIKE ?)"
		s := "%" + strings.ToLower(f.Search) + "%"
		args = append(args, s, s)
	}
	q += " ORDER BY t.date DESC, t.id DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		q += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var txs []model.Transaction
	for rows.Next() {
		var t model.Transaction
		if err := rows.Scan(&t.ID, &t.AccountID, &t.AccountName, &t.CategoryID, &t.CategoryName,
			&t.Amount, &t.Date, &t.Payee, &t.Note, &t.Type, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, nil
}

func (s *Service) DeleteTransaction(id int64) error {
	var accountID, amount int64
	var txType string
	err := s.db.QueryRow("SELECT account_id, amount, type FROM transactions WHERE id = ?", id).Scan(&accountID, &amount, &txType)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM transactions WHERE id = ?", id)
	if err != nil {
		return err
	}
	// reverse balance change
	delta := amount
	if model.TxType(txType).IsCredit() {
		delta = -delta
	}
	return s.UpdateAccountBalance(accountID, delta)
}

func (s *Service) UpdateTransactionCategory(txID int64, categoryID *int64) error {
	_, err := s.db.Exec("UPDATE transactions SET category_id = ? WHERE id = ?", categoryID, txID)
	return err
}

func (s *Service) UpdateTransactionType(txID int64, newType model.TxType) error {
	var accountID int64
	var oldType string
	err := s.db.QueryRow("SELECT account_id, type FROM transactions WHERE id = ?", txID).
		Scan(&accountID, &oldType)
	if err != nil {
		return err
	}
	if model.TxType(oldType) == newType {
		return nil
	}
	_, err = s.db.Exec("UPDATE transactions SET type = ? WHERE id = ?", newType, txID)
	if err != nil {
		return err
	}
	return s.RecalculateAccountBalance(accountID)
}

// --- Budgets ---

func (s *Service) SetBudget(categoryID int64, year, month int, amountCents int64) error {
	_, err := s.db.Exec(`
		INSERT INTO budgets (category_id, year, month, amount_limit) VALUES (?, ?, ?, ?)
		ON CONFLICT(category_id, year, month) DO UPDATE SET amount_limit = excluded.amount_limit`,
		categoryID, year, month, amountCents)
	return err
}

func (s *Service) GetBudgetStatus(year, month int) ([]model.BudgetStatus, error) {
	monthStr := fmt.Sprintf("%04d-%02d", year, month)
	// Show all expense categories (from non-income/transfer groups) with their
	// budget limit (if set) and actual spending for the month.
	rows, err := s.db.Query(`
		SELECT c.id, c.name, 
			COALESCE(b.id, 0),
			COALESCE(b.amount_limit, 0),
			COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.category_id = c.id AND t.date LIKE ? AND t.type = 'expense'), 0)
		FROM categories c
		JOIN category_groups g ON c.group_id = g.id
		LEFT JOIN budgets b ON b.category_id = c.id AND b.year = ? AND b.month = ?
		WHERE g.name NOT IN ('Income', 'Transfers', 'Uncategorized')
		ORDER BY g.sort_order, c.sort_order`, monthStr+"%", year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var statuses []model.BudgetStatus
	for rows.Next() {
		var bs model.BudgetStatus
		if err := rows.Scan(&bs.CategoryID, &bs.CategoryName, &bs.ID, &bs.AmountLimit, &bs.Spent); err != nil {
			return nil, err
		}
		bs.Year = year
		bs.Month = month
		bs.Remaining = bs.AmountLimit - bs.Spent
		if bs.AmountLimit > 0 {
			bs.Percent = float64(bs.Spent) / float64(bs.AmountLimit) * 100
		}
		statuses = append(statuses, bs)
	}
	return statuses, nil
}

// --- Recurring ---

func (s *Service) CreateRecurringRule(r model.RecurringRule) (*model.RecurringRule, error) {
	if r.NextDue == "" {
		r.NextDue = r.StartDate
	}
	res, err := s.db.Exec(
		"INSERT INTO recurring_rules (account_id, category_id, amount, payee, note, frequency, start_date, end_date, next_due, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		r.AccountID, r.CategoryID, r.Amount, r.Payee, r.Note, r.Frequency, r.StartDate, r.EndDate, r.NextDue, r.Type,
	)
	if err != nil {
		return nil, err
	}
	r.ID, _ = res.LastInsertId()
	return &r, nil
}

func (s *Service) ListRecurringRules() ([]model.RecurringRule, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.account_id, COALESCE(a.name,''), r.category_id, COALESCE(c.name,''),
			r.amount, r.payee, r.note, r.frequency, r.start_date, r.end_date, r.next_due, r.type
		FROM recurring_rules r
		LEFT JOIN accounts a ON r.account_id = a.id
		LEFT JOIN categories c ON r.category_id = c.id
		ORDER BY r.next_due`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.RecurringRule
	for rows.Next() {
		var r model.RecurringRule
		if err := rows.Scan(&r.ID, &r.AccountID, &r.AccountName, &r.CategoryID, &r.CategoryName,
			&r.Amount, &r.Payee, &r.Note, &r.Frequency, &r.StartDate, &r.EndDate, &r.NextDue, &r.Type); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Service) DeleteRecurringRule(id int64) error {
	_, err := s.db.Exec("DELETE FROM recurring_rules WHERE id = ?", id)
	return err
}

func (s *Service) GenerateRecurring() (int, error) {
	today := time.Now().Format("2006-01-02")
	rules, err := s.ListRecurringRules()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range rules {
		if r.NextDue > today {
			continue
		}
		if r.EndDate != nil && *r.EndDate < today {
			continue
		}
		// create the transaction
		tx := model.Transaction{
			AccountID:  r.AccountID,
			CategoryID: r.CategoryID,
			Amount:     r.Amount,
			Date:       r.NextDue,
			Payee:      r.Payee,
			Note:       r.Note,
			Type:       r.Type,
		}
		if _, err := s.CreateTransaction(tx); err != nil {
			return count, err
		}
		count++

		// advance next_due
		next := advanceDate(r.NextDue, r.Frequency)
		if _, err := s.db.Exec("UPDATE recurring_rules SET next_due = ? WHERE id = ?", next, r.ID); err != nil {
			return count, fmt.Errorf("advance recurring rule: %w", err)
		}
	}
	return count, nil
}

func advanceDate(dateStr string, freq model.Frequency) string {
	t, _ := time.Parse("2006-01-02", dateStr)
	switch freq {
	case model.FreqWeekly:
		t = t.AddDate(0, 0, 7)
	case model.FreqBiweekly:
		t = t.AddDate(0, 0, 14)
	case model.FreqMonthly:
		t = t.AddDate(0, 1, 0)
	case model.FreqYearly:
		t = t.AddDate(1, 0, 0)
	}
	return t.Format("2006-01-02")
}

// --- Auto-categorization ---

func (s *Service) AddAutoCatRule(pattern string, categoryID int64) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO auto_cat_rules (pattern, category_id) VALUES (?, ?)", strings.ToLower(pattern), categoryID)
	return err
}

func (s *Service) ListAutoCatRules() ([]model.AutoCatRule, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.pattern, r.category_id, COALESCE(c.name,'')
		FROM auto_cat_rules r
		LEFT JOIN categories c ON r.category_id = c.id
		ORDER BY r.pattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.AutoCatRule
	for rows.Next() {
		var r model.AutoCatRule
		if err := rows.Scan(&r.ID, &r.Pattern, &r.CategoryID, &r.CategoryName); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Service) AutoCategorize(payee string) *int64 {
	rules, _ := s.ListAutoCatRules()
	lower := strings.ToLower(payee)
	for _, r := range rules {
		if strings.Contains(lower, r.Pattern) {
			return &r.CategoryID
		}
	}
	// built-in pattern matching
	builtins := []struct {
		patterns []string
		catName  string
	}{
		{[]string{"dividend"}, "Dividend Income"},
		{[]string{"american express", "amex", "applecard", "chase card", "citi card", "capital one"}, "Credit Card Payment"},
		{[]string{"payroll", "direct dep"}, "Salary"},
		{[]string{"interest earned", "bank interest"}, "Interest"},
		{[]string{"refund", "rebate", "rewards"}, "Refunds"},
	}
	for _, b := range builtins {
		for _, p := range b.patterns {
			if strings.Contains(lower, p) {
				cat, err := s.FindCategoryByName(b.catName)
				if err == nil {
					return &cat.ID
				}
			}
		}
	}
	return nil
}

// --- Net Worth ---

type EquityValueFunc func() (int64, error)
type AccountEquityValueFunc func(accountID int64) (int64, error)

var GetEquityValue EquityValueFunc
var GetAccountEquityValue AccountEquityValueFunc

func (s *Service) SnapshotNetWorth() (*model.NetWorthSnapshot, error) {
	// Refresh investment account balances from portfolio values
	if err := s.RecalculateAllBalances(); err != nil {
		return nil, fmt.Errorf("recalculate balances: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	rows, err := s.db.Query("SELECT type, balance FROM accounts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets, liabilities int64
	for rows.Next() {
		var typ string
		var bal int64
		if err := rows.Scan(&typ, &bal); err != nil {
			continue
		}
		if model.IsLiability(model.AccountType(typ)) {
			liabilities += bal
		} else {
			assets += bal
		}
	}
	// Get equity value for display breakdown (already included in
	// managed/brokerage account balances via RecalculateAccountBalance)
	var equityValue int64
	if GetEquityValue != nil {
		equityValue, _ = GetEquityValue()
	}
	nw := assets - liabilities
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO networth_snapshots (date, total_assets, total_liabilities, equity_value, net_worth) VALUES (?, ?, ?, ?, ?)`,
		today, assets, liabilities, equityValue, nw); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}
	// Snapshot each account's balance for sparkline history
	s.snapshotAccountBalances(today)

	return &model.NetWorthSnapshot{Date: today, TotalAssets: assets, TotalLiabilities: liabilities, EquityValue: equityValue, NetWorth: nw}, nil
}

func (s *Service) snapshotAccountBalances(date string) {
	accs, err := s.ListAccounts()
	if err != nil {
		return
	}
	for _, a := range accs {
		if _, err := s.db.Exec(`INSERT OR REPLACE INTO account_balance_history (account_id, date, balance) VALUES (?, ?, ?)`,
			a.ID, date, a.Balance); err != nil {
			continue
		}
	}
}

func (s *Service) GetAccountBalanceHistory(accountID int64, days int) ([]int64, error) {
	rows, err := s.db.Query(`
		SELECT balance FROM account_balance_history
		WHERE account_id = ? ORDER BY date DESC LIMIT ?`, accountID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var balances []int64
	for rows.Next() {
		var b int64
		if err := rows.Scan(&b); err != nil {
			continue
		}
		balances = append(balances, b)
	}
	// Reverse to chronological order
	for i, j := 0, len(balances)-1; i < j; i, j = i+1, j-1 {
		balances[i], balances[j] = balances[j], balances[i]
	}
	return balances, nil
}

func (s *Service) GetNetWorthHistory(limit int) ([]model.NetWorthSnapshot, error) {
	rows, err := s.db.Query("SELECT id, date, total_assets, total_liabilities, equity_value, net_worth FROM networth_snapshots ORDER BY date DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snaps []model.NetWorthSnapshot
	for rows.Next() {
		var n model.NetWorthSnapshot
		if err := rows.Scan(&n.ID, &n.Date, &n.TotalAssets, &n.TotalLiabilities, &n.EquityValue, &n.NetWorth); err != nil {
			continue
		}
		snaps = append(snaps, n)
	}
	return snaps, nil
}

// --- Reports ---

type SpendingByCategory struct {
	CategoryName string  `json:"category_name"`
	Amount       int64   `json:"amount"`
	Percent      float64 `json:"percent"`
}

func (s *Service) SpendingReport(year, month int) ([]SpendingByCategory, error) {
	monthStr := fmt.Sprintf("%04d-%02d", year, month)
	rows, err := s.db.Query(`
		SELECT COALESCE(c.name, 'Uncategorized'), SUM(t.amount)
		FROM transactions t
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE t.date LIKE ? AND t.type = 'expense'
		GROUP BY c.name
		ORDER BY SUM(t.amount) DESC`, monthStr+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SpendingByCategory
	var total int64
	for rows.Next() {
		var r SpendingByCategory
		if err := rows.Scan(&r.CategoryName, &r.Amount); err != nil {
			continue
		}
		total += r.Amount
		results = append(results, r)
	}
	for i := range results {
		if total > 0 {
			results[i].Percent = float64(results[i].Amount) / float64(total) * 100
		}
	}
	return results, nil
}

type IncomeBySource struct {
	CategoryName string  `json:"category"`
	Amount       int64   `json:"amount"`
	Percent      float64 `json:"percent"`
}

type IncomePeriod struct {
	Period  string           `json:"period"`
	Total   int64            `json:"total"`
	Sources []IncomeBySource `json:"sources"`
}

func (s *Service) IncomeReport(from, to, groupBy string) ([]IncomePeriod, error) {
	var periodExpr string
	switch groupBy {
	case "quarterly":
		periodExpr = "SUBSTR(date,1,4) || '-Q' || ((CAST(SUBSTR(date,6,2) AS INTEGER)-1)/3 + 1)"
	case "yearly":
		periodExpr = "SUBSTR(date,1,4)"
	default:
		periodExpr = "SUBSTR(date,1,7)"
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT %s as period, COALESCE(c.name, 'Uncategorized'), SUM(t.amount)
		FROM transactions t
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE t.type = 'income' AND t.date >= ? AND t.date <= ?
		GROUP BY period, c.name
		ORDER BY period, SUM(t.amount) DESC`, periodExpr), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periodMap := make(map[string]*IncomePeriod)
	var periodOrder []string
	for rows.Next() {
		var period, cat string
		var amount int64
		if err := rows.Scan(&period, &cat, &amount); err != nil {
			continue
		}
		if _, ok := periodMap[period]; !ok {
			periodMap[period] = &IncomePeriod{Period: period}
			periodOrder = append(periodOrder, period)
		}
		p := periodMap[period]
		p.Total += amount
		p.Sources = append(p.Sources, IncomeBySource{CategoryName: cat, Amount: amount})
	}

	results := make([]IncomePeriod, 0, len(periodOrder))
	for _, key := range periodOrder {
		p := periodMap[key]
		for i := range p.Sources {
			if p.Total > 0 {
				p.Sources[i].Percent = float64(p.Sources[i].Amount) / float64(p.Total) * 100
			}
		}
		results = append(results, *p)
	}
	return results, nil
}

type CashFlowReport struct {
	Month    string `json:"month"`
	Income   int64  `json:"income"`
	Expenses int64  `json:"expenses"`
	Net      int64  `json:"net"`
}

func (s *Service) CashFlowReport(from, to string) ([]CashFlowReport, error) {
	rows, err := s.db.Query(`
		SELECT SUBSTR(date, 1, 7) as month, type, SUM(amount)
		FROM transactions
		WHERE date >= ? AND date <= ?
		GROUP BY month, type
		ORDER BY month`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]*CashFlowReport)
	for rows.Next() {
		var month, typ string
		var amount int64
		if err := rows.Scan(&month, &typ, &amount); err != nil {
			continue
		}
		if _, ok := m[month]; !ok {
			m[month] = &CashFlowReport{Month: month}
		}
		switch model.TxType(typ) {
		case model.TxIncome:
			m[month].Income += amount
		case model.TxExpense:
			m[month].Expenses += amount
		}
	}
	results := make([]CashFlowReport, 0, len(m))
	for _, v := range m {
		v.Net = v.Income - v.Expenses
		results = append(results, *v)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Month < results[j].Month
	})
	return results, nil
}

// --- 401k Contributions ---

func (s *Service) Contribute401k(accountID int64, year int, employeeAmount, employerMatch int64) (*model.Contribution401k, error) {
	_, err := s.db.Exec(`
		INSERT INTO contributions_401k (account_id, year, employee_contrib, employer_match)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(account_id, year) DO UPDATE SET
			employee_contrib = employee_contrib + excluded.employee_contrib,
			employer_match = employer_match + excluded.employer_match`,
		accountID, year, employeeAmount, employerMatch)
	if err != nil {
		return nil, err
	}
	return s.Get401kStatus(accountID, year)
}

func (s *Service) Get401kStatus(accountID int64, year int) (*model.Contribution401k, error) {
	var c model.Contribution401k
	err := s.db.QueryRow(`
		SELECT c.id, c.account_id, COALESCE(a.name,''), c.year, c.employee_contrib, c.employer_match, c.match_percent, c.annual_limit
		FROM contributions_401k c
		LEFT JOIN accounts a ON c.account_id = a.id
		WHERE c.account_id = ? AND c.year = ?`, accountID, year).
		Scan(&c.ID, &c.AccountID, &c.AccountName, &c.Year, &c.EmployeeContrib, &c.EmployerMatch, &c.MatchPercent, &c.AnnualLimit)
	if err != nil {
		return &model.Contribution401k{AccountID: accountID, Year: year, AnnualLimit: 2350000}, nil
	}
	c.TotalContrib = c.EmployeeContrib + c.EmployerMatch
	c.Remaining = c.AnnualLimit - c.EmployeeContrib
	if c.Remaining < 0 {
		c.Remaining = 0
	}
	if c.AnnualLimit > 0 {
		c.Percent = float64(c.EmployeeContrib) / float64(c.AnnualLimit) * 100
	}
	return &c, nil
}

func (s *Service) Set401kMatchPercent(accountID int64, year int, pct float64) error {
	_, err := s.db.Exec(`
		INSERT INTO contributions_401k (account_id, year, match_percent)
		VALUES (?, ?, ?)
		ON CONFLICT(account_id, year) DO UPDATE SET match_percent = excluded.match_percent`,
		accountID, year, pct)
	return err
}

func (s *Service) Set401kAnnualLimit(accountID int64, year int, limitCents int64) error {
	_, err := s.db.Exec(`
		INSERT INTO contributions_401k (account_id, year, annual_limit)
		VALUES (?, ?, ?)
		ON CONFLICT(account_id, year) DO UPDATE SET annual_limit = excluded.annual_limit`,
		accountID, year, limitCents)
	return err
}
