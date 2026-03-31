package model

import "time"

type AccountType string

const (
	AccountChecking   AccountType = "checking"
	AccountSavings    AccountType = "savings"
	AccountCreditCard AccountType = "credit_card"
	AccountInvestment AccountType = "investment"
	AccountLoan       AccountType = "loan"
	AccountCash       AccountType = "cash"
)

func ValidAccountTypes() []AccountType {
	return []AccountType{AccountChecking, AccountSavings, AccountCreditCard, AccountInvestment, AccountLoan, AccountCash}
}

func IsLiability(t AccountType) bool {
	return t == AccountCreditCard || t == AccountLoan
}

type TxType string

const (
	TxExpense  TxType = "expense"
	TxIncome   TxType = "income"
	TxTransfer TxType = "transfer"
)

type Frequency string

const (
	FreqWeekly    Frequency = "weekly"
	FreqBiweekly  Frequency = "biweekly"
	FreqMonthly   Frequency = "monthly"
	FreqYearly    Frequency = "yearly"
)

type Account struct {
	ID        int64       `json:"id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Balance   int64       `json:"balance"` // cents
	Currency  string      `json:"currency"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type CategoryGroup struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type Category struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name,omitempty"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
}

type Transaction struct {
	ID             int64     `json:"id"`
	AccountID      int64     `json:"account_id"`
	AccountName    string    `json:"account_name,omitempty"`
	CategoryID     *int64    `json:"category_id"`
	CategoryName   string    `json:"category_name,omitempty"`
	Amount         int64     `json:"amount"` // cents, positive
	Date           string    `json:"date"`   // YYYY-MM-DD
	Payee          string    `json:"payee"`
	Note           string    `json:"note"`
	Type           TxType    `json:"type"`
	TransferPairID *int64    `json:"transfer_pair_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Budget struct {
	ID          int64  `json:"id"`
	CategoryID  int64  `json:"category_id"`
	CategoryName string `json:"category_name,omitempty"`
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	AmountLimit int64  `json:"amount_limit"` // cents
}

type BudgetStatus struct {
	Budget
	Spent     int64   `json:"spent"`
	Remaining int64   `json:"remaining"`
	Percent   float64 `json:"percent"`
}

type RecurringRule struct {
	ID         int64     `json:"id"`
	AccountID  int64     `json:"account_id"`
	AccountName string   `json:"account_name,omitempty"`
	CategoryID *int64    `json:"category_id"`
	CategoryName string  `json:"category_name,omitempty"`
	Amount     int64     `json:"amount"`
	Payee      string    `json:"payee"`
	Note       string    `json:"note"`
	Frequency  Frequency `json:"frequency"`
	StartDate  string    `json:"start_date"`
	EndDate    *string   `json:"end_date,omitempty"`
	NextDue    string    `json:"next_due"`
	Type       TxType    `json:"type"`
}

type NetWorthSnapshot struct {
	ID               int64  `json:"id"`
	Date             string `json:"date"`
	TotalAssets      int64  `json:"total_assets"`
	TotalLiabilities int64  `json:"total_liabilities"`
	NetWorth         int64  `json:"net_worth"`
}

type AutoCatRule struct {
	ID         int64  `json:"id"`
	Pattern    string `json:"pattern"` // substring match on payee
	CategoryID int64  `json:"category_id"`
	CategoryName string `json:"category_name,omitempty"`
}

type TxFilter struct {
	Month     string // YYYY-MM
	From      string // YYYY-MM-DD
	To        string // YYYY-MM-DD
	AccountID *int64
	CategoryID *int64
	Payee     string
	Type      TxType
	Search    string
	Limit     int
	Offset    int
}
