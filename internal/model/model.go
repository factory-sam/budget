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
	EquityValue      int64  `json:"equity_value"`
	NetWorth         int64  `json:"net_worth"`
}

type AutoCatRule struct {
	ID         int64  `json:"id"`
	Pattern    string `json:"pattern"` // substring match on payee
	CategoryID int64  `json:"category_id"`
	CategoryName string `json:"category_name,omitempty"`
}

// --- Equity ---

type EquityPrice struct {
	ID     int64  `json:"id"`
	Ticker string `json:"ticker"`
	Date   string `json:"date"`
	Price  int64  `json:"price"` // cents
}

type EquityLot struct {
	ID                int64   `json:"id"`
	Ticker            string  `json:"ticker"`
	Shares            float64 `json:"shares"`
	CostBasis         int64   `json:"cost_basis"` // total cents for this lot
	DateAcquired      string  `json:"date_acquired"`
	Source            string  `json:"source"` // buy, iso_exercise, rsu_vest
	GrantID           *int64  `json:"grant_id,omitempty"`
	IncludeInNetWorth bool    `json:"include_in_networth"`
	Note              string  `json:"note"`
	// computed
	CurrentPrice int64   `json:"current_price,omitempty"`
	MarketValue  int64   `json:"market_value,omitempty"`
	GainLoss     int64   `json:"gain_loss,omitempty"`
	GainPct      float64 `json:"gain_pct,omitempty"`
}

type EquityGrant struct {
	ID              int64   `json:"id"`
	Ticker          string  `json:"ticker"`
	GrantType       string  `json:"grant_type"` // iso, rsu
	TotalShares     float64 `json:"total_shares"`
	GrantDate       string  `json:"grant_date"`
	StrikePrice     *int64  `json:"strike_price,omitempty"` // cents, ISOs only
	ExpirationDate  *string `json:"expiration_date,omitempty"`
	CliffMonths     int     `json:"cliff_months"`
	VestingMonths   int     `json:"vesting_months"`
	VestingInterval string  `json:"vesting_interval"` // monthly, quarterly
	Note            string  `json:"note"`
	// computed
	VestedShares   float64 `json:"vested_shares,omitempty"`
	UnvestedShares float64 `json:"unvested_shares,omitempty"`
	NextVestDate   string  `json:"next_vest_date,omitempty"`
}

type VestEvent struct {
	ID          int64   `json:"id"`
	GrantID     int64   `json:"grant_id"`
	Date        string  `json:"date"`
	Shares      float64 `json:"shares"`
	FMVPerShare *int64  `json:"fmv_per_share,omitempty"` // cents
	Status      string  `json:"status"`                  // pending, vested, exercised
	LotID       *int64  `json:"lot_id,omitempty"`
}

type PositionSummary struct {
	Ticker       string      `json:"ticker"`
	TotalShares  float64     `json:"total_shares"`
	AvgCostBasis int64       `json:"avg_cost_basis"` // cents per share
	CurrentPrice int64       `json:"current_price"`
	MarketValue  int64       `json:"market_value"`
	GainLoss     int64       `json:"gain_loss"`
	GainPct      float64     `json:"gain_pct"`
	Lots         []EquityLot `json:"lots,omitempty"`
}

type PortfolioSummary struct {
	TotalValue    int64             `json:"total_value"`
	TotalCostBasis int64           `json:"total_cost_basis"`
	TotalGainLoss int64            `json:"total_gain_loss"`
	Positions     []PositionSummary `json:"positions"`
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
