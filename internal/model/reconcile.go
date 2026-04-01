package model

type TransferMatch struct {
	OutTx      Transaction `json:"out_tx"`
	InTx       Transaction `json:"in_tx"`
	Confidence string      `json:"confidence"` // "exact", "likely", "ambiguous"
	DaysDiff   int         `json:"days_diff"`
}

type UnmatchedTransfer struct {
	Tx        Transaction `json:"tx"`
	Direction string      `json:"direction"` // "out" or "in"
}

type DuplicateGroup struct {
	Transactions []Transaction `json:"transactions"`
	Reason       string        `json:"reason"` // "same_date_amount_payee", "same_date_amount"
}

type ReconciliationReport struct {
	AccountID      int64  `json:"account_id"`
	AccountName    string `json:"account_name"`
	CalculatedBal  int64  `json:"calculated_balance"`
	ActualBal      int64  `json:"actual_balance"`
	Discrepancy    int64  `json:"discrepancy"`
	LastReconciled string `json:"last_reconciled,omitempty"`
}

type Paycheck struct {
	ID              int64  `json:"id"`
	Date            string `json:"date"`
	Employer        string `json:"employer"`
	GrossPay        int64  `json:"gross_pay"`
	FederalTax      int64  `json:"federal_tax"`
	StateTax        int64  `json:"state_tax"`
	SocialSecurity  int64  `json:"social_security"`
	Medicare        int64  `json:"medicare"`
	HealthInsurance int64  `json:"health_insurance"`
	Retirement401k  int64  `json:"retirement_401k"`
	OtherDeductions int64  `json:"other_deductions"`
	NetPay          int64  `json:"net_pay"`
	TxID            *int64 `json:"tx_id,omitempty"` // linked bank deposit
	Note            string `json:"note"`
}

func (p Paycheck) TotalDeductions() int64 {
	return p.FederalTax + p.StateTax + p.SocialSecurity + p.Medicare +
		p.HealthInsurance + p.Retirement401k + p.OtherDeductions
}
