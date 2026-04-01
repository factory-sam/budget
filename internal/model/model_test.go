package model

import (
	"testing"
)

func TestValidAccountTypes(t *testing.T) {
	t.Parallel()

	types := ValidAccountTypes()

	if len(types) != 10 {
		t.Fatalf("expected 10 account types, got %d", len(types))
	}

	expected := []AccountType{
		AccountChecking, AccountSavings, AccountCreditCard, AccountInvestment,
		AccountLoan, AccountCash, AccountBrokerage, Account401k,
		AccountMoneyMarket, AccountManaged,
	}

	for _, exp := range expected {
		t.Run(string(exp), func(t *testing.T) {
			t.Parallel()
			found := false
			for _, at := range types {
				if at == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q to be in ValidAccountTypes()", exp)
			}
		})
	}
}

func TestIsLiability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		acctType AccountType
		want     bool
	}{
		{"credit_card is liability", AccountCreditCard, true},
		{"loan is liability", AccountLoan, true},
		{"checking is not liability", AccountChecking, false},
		{"savings is not liability", AccountSavings, false},
		{"investment is not liability", AccountInvestment, false},
		{"cash is not liability", AccountCash, false},
		{"brokerage is not liability", AccountBrokerage, false},
		{"401k is not liability", Account401k, false},
		{"money_market is not liability", AccountMoneyMarket, false},
		{"managed is not liability", AccountManaged, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsLiability(tc.acctType)
			if got != tc.want {
				t.Errorf("IsLiability(%q) = %v, want %v", tc.acctType, got, tc.want)
			}
		})
	}
}

func TestIsInvestmentAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		acctType AccountType
		want     bool
	}{
		{"investment is investment", AccountInvestment, true},
		{"brokerage is investment", AccountBrokerage, true},
		{"401k is investment", Account401k, true},
		{"managed is investment", AccountManaged, true},
		{"checking is not investment", AccountChecking, false},
		{"savings is not investment", AccountSavings, false},
		{"credit_card is not investment", AccountCreditCard, false},
		{"loan is not investment", AccountLoan, false},
		{"cash is not investment", AccountCash, false},
		{"money_market is not investment", AccountMoneyMarket, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsInvestmentAccount(tc.acctType)
			if got != tc.want {
				t.Errorf("IsInvestmentAccount(%q) = %v, want %v", tc.acctType, got, tc.want)
			}
		})
	}
}

func TestTxType_IsTransfer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tx   TxType
		want bool
	}{
		{"transfer is transfer", TxTransfer, true},
		{"transfer_out is transfer", TxTransferOut, true},
		{"transfer_in is transfer", TxTransferIn, true},
		{"expense is not transfer", TxExpense, false},
		{"income is not transfer", TxIncome, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.tx.IsTransfer()
			if got != tc.want {
				t.Errorf("TxType(%q).IsTransfer() = %v, want %v", tc.tx, got, tc.want)
			}
		})
	}
}

func TestTxType_IsCredit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tx   TxType
		want bool
	}{
		{"income is credit", TxIncome, true},
		{"transfer_in is credit", TxTransferIn, true},
		{"expense is not credit", TxExpense, false},
		{"transfer is not credit", TxTransfer, false},
		{"transfer_out is not credit", TxTransferOut, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.tx.IsCredit()
			if got != tc.want {
				t.Errorf("TxType(%q).IsCredit() = %v, want %v", tc.tx, got, tc.want)
			}
		})
	}
}
