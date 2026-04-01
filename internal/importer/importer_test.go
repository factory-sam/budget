package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sam/budget/internal/db"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

// ---------------------------------------------------------------------------
// parseAmount
// ---------------------------------------------------------------------------

func TestParseAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantCents int64
		wantType  model.TxType
	}{
		{"positive income", "100.50", 10050, model.TxIncome},
		{"negative expense", "-50.00", 5000, model.TxExpense},
		{"parens expense", "(25.99)", 2599, model.TxExpense},
		{"dollar sign and commas", "$1,234.56", 123456, model.TxIncome},
		{"negative with dollar and commas", "-$1,234.56", 123456, model.TxExpense},
		{"empty string", "", 0, model.TxExpense},
		{"invalid string", "abc", 0, model.TxExpense},
		{"whitespace only", "   ", 0, model.TxExpense},
		{"zero", "0.00", 0, model.TxIncome},
		{"integer no decimals", "42", 4200, model.TxIncome},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cents, txType := parseAmount(tc.input)
			if cents != tc.wantCents {
				t.Errorf("parseAmount(%q) cents = %d, want %d", tc.input, cents, tc.wantCents)
			}
			if txType != tc.wantType {
				t.Errorf("parseAmount(%q) type = %q, want %q", tc.input, txType, tc.wantType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeDate
// ---------------------------------------------------------------------------

func TestNormalizeDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already normalized", "2026-03-15", "2026-03-15"},
		{"MM/DD/YYYY", "03/15/2026", "2026-03-15"},
		{"single digit month and day", "3/5/2026", "2026-03-05"},
		{"whitespace trimmed", "  2026-03-15  ", "2026-03-15"},
		{"single digit day", "12/5/2026", "2026-12-05"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeDate(tc.input)
			if got != tc.want {
				t.Errorf("normalizeDate(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// zeroPad
// ---------------------------------------------------------------------------

func TestZeroPad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"5", "05"},
		{"12", "12"},
		{"0", "00"},
		{"9", "09"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := zeroPad(tc.input)
			if got != tc.want {
				t.Errorf("zeroPad(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// detectColumns
// ---------------------------------------------------------------------------

func TestDetectColumns(t *testing.T) {
	t.Parallel()

	t.Run("standard headers", func(t *testing.T) {
		t.Parallel()
		header := []string{"Date", "Description", "Amount"}
		m := detectColumns(header)
		if m.date != 0 {
			t.Errorf("date = %d, want 0", m.date)
		}
		if m.description != 1 {
			t.Errorf("description = %d, want 1", m.description)
		}
		if m.amount != 2 {
			t.Errorf("amount = %d, want 2", m.amount)
		}
	})

	t.Run("with payee", func(t *testing.T) {
		t.Parallel()
		header := []string{"Date", "Payee", "Memo", "Amount"}
		m := detectColumns(header)
		if m.date != 0 {
			t.Errorf("date = %d, want 0", m.date)
		}
		if m.payee != 1 {
			t.Errorf("payee = %d, want 1", m.payee)
		}
		if m.amount != 3 {
			t.Errorf("amount = %d, want 3", m.amount)
		}
	})

	t.Run("no matching columns", func(t *testing.T) {
		t.Parallel()
		header := []string{"Foo", "Bar", "Baz"}
		m := detectColumns(header)
		if m.date != -1 {
			t.Errorf("date = %d, want -1", m.date)
		}
		if m.amount != -1 {
			t.Errorf("amount = %d, want -1", m.amount)
		}
	})

	t.Run("debit column as amount", func(t *testing.T) {
		t.Parallel()
		header := []string{"Transaction Date", "Debit", "Description"}
		m := detectColumns(header)
		if m.date != 0 {
			t.Errorf("date = %d, want 0", m.date)
		}
		if m.amount != 1 {
			t.Errorf("amount = %d, want 1", m.amount)
		}
		if m.description != 2 {
			t.Errorf("description = %d, want 2", m.description)
		}
	})
}

// ---------------------------------------------------------------------------
// classifyTransaction (equity_csv.go)
// ---------------------------------------------------------------------------

func TestClassifyTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string
		want string
	}{
		{"purchase", "Purchase - AAPL", "purchase"},
		{"sale", "Sale - GOOG", "sale"},
		{"dividend", "Dividend - MSFT", "dividend"},
		{"transfer in", "Security Transfer In - TSLA", "transfer_in"},
		{"funds received", "Wire Funds Received", "deposit"},
		{"withdrawal", "Withdrawal to checking", "deposit"},
		{"transfer adjustment", "Transfer / Adjustment", "deposit"},
		{"advisory fee", "Advisory Fee Debit", "fee"},
		{"annual service fee", "Annual Service Fee", "fee"},
		{"advisory program fee", "Advisory Program Fee", "fee"},
		{"bank interest", "Bank Interest Payment", "interest"},
		{"reinvestment program", "Reinvestment Program - VTSAX", "fund_outflow"},
		{"subscription", "Subscription purchase", "fund_outflow"},
		{"reinvestment shares", "Reinvestment Shares allocated", "reinvest_shares"},
		{"unknown", "Random transaction note", "other"},
		{"empty", "", "other"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyTransaction(tc.desc)
			if got != tc.want {
				t.Errorf("classifyTransaction(%q) = %q, want %q", tc.desc, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseNumber (equity_csv.go)
// ---------------------------------------------------------------------------

func TestParseNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  float64
	}{
		{"dollar with commas", "$1,234.56", 1234.56},
		{"negative", "-100", -100},
		{"empty", "", 0},
		{"invalid", "abc", 0},
		{"plain float", "42.5", 42.5},
		{"spaces around", "  99.99  ", 99.99},
		{"negative with dollar", "-$500.00", -500.00},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseNumber(tc.input)
			if got != tc.want {
				t.Errorf("parseNumber(%q) = %f, want %f", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isValidDate (equity_csv.go)
// ---------------------------------------------------------------------------

func TestIsValidDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid date", "2026-03-15", true},
		{"invalid alpha", "abc", false},
		{"too short", "3/5", false},
		{"no dashes", "20260315", false},
		{"empty", "", false},
		{"wrong separator", "2026/03/15", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isValidDate(tc.input)
			if got != tc.want {
				t.Errorf("isValidDate(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// truncateDesc (equity_csv.go)
// ---------------------------------------------------------------------------

func TestTruncateDesc(t *testing.T) {
	t.Parallel()

	t.Run("short string unchanged", func(t *testing.T) {
		t.Parallel()
		input := "Short description"
		got := truncateDesc(input)
		if got != input {
			t.Errorf("truncateDesc(%q) = %q, want %q", input, got, input)
		}
	})

	t.Run("exactly 80 chars unchanged", func(t *testing.T) {
		t.Parallel()
		input := "12345678901234567890123456789012345678901234567890123456789012345678901234567890"
		got := truncateDesc(input)
		if got != input {
			t.Errorf("truncateDesc should not truncate at exactly 80 chars")
		}
	})

	t.Run("long string truncated with ellipsis", func(t *testing.T) {
		t.Parallel()
		input := "12345678901234567890123456789012345678901234567890123456789012345678901234567890X"
		got := truncateDesc(input)
		if len(got) != 80 {
			t.Errorf("truncateDesc length = %d, want 80", len(got))
		}
		if got[len(got)-3:] != "..." {
			t.Errorf("truncateDesc should end with '...', got %q", got[len(got)-3:])
		}
	})
}

// ---------------------------------------------------------------------------
// Integration: full CSV import
// ---------------------------------------------------------------------------

// csvImportTestEnv holds the shared state for CSV import integration subtests.
type csvImportTestEnv struct {
	svc      *service.Service
	acc      *model.Account
	importer *CSVImporter
	csvPath  string
}

func setupCSVImportTest(t *testing.T) *csvImportTestEnv {
	t.Helper()
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.SeedCategories(database); err != nil {
		t.Fatalf("SeedCategories: %v", err)
	}

	svc := service.New(database)
	acc, err := svc.CreateAccount("Test Checking", model.AccountChecking, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	csvContent := `Date,Description,Amount
2026-03-01,Salary Deposit,2500.00
2026-03-05,Grocery Store,-75.50
2026-03-10,Coffee Shop,-4.99
03/15/2026,Gas Station,-45.00
`
	csvPath := filepath.Join(tmpDir, "transactions.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	return &csvImportTestEnv{svc: svc, acc: acc, importer: NewCSV(svc), csvPath: csvPath}
}

func TestCSVImportIntegration(t *testing.T) {
	t.Parallel()
	env := setupCSVImportTest(t)

	count, err := env.importer.Import(env.csvPath, env.acc.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if count != 4 {
		t.Errorf("imported count = %d, want 4", count)
	}

	txs, err := env.svc.ListTransactions(model.TxFilter{AccountID: &env.acc.ID})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(txs) != 4 {
		t.Fatalf("transaction count = %d, want 4", len(txs))
	}

	t.Run("income transaction", func(t *testing.T) {
		for _, tx := range txs {
			if tx.Payee == "Salary Deposit" {
				if tx.Type != model.TxIncome {
					t.Errorf("Salary type = %q, want %q", tx.Type, model.TxIncome)
				}
				if tx.Amount != 250000 {
					t.Errorf("Salary amount = %d, want 250000", tx.Amount)
				}
				return
			}
		}
		t.Error("did not find Salary Deposit transaction")
	})

	t.Run("expense transaction", func(t *testing.T) {
		for _, tx := range txs {
			if tx.Payee == "Grocery Store" {
				if tx.Type != model.TxExpense {
					t.Errorf("Grocery type = %q, want %q", tx.Type, model.TxExpense)
				}
				if tx.Amount != 7550 {
					t.Errorf("Grocery amount = %d, want 7550", tx.Amount)
				}
				return
			}
		}
		t.Error("did not find Grocery Store transaction")
	})

	t.Run("date normalization", func(t *testing.T) {
		for _, tx := range txs {
			if tx.Payee == "Gas Station" {
				if tx.Date != "2026-03-15" {
					t.Errorf("Gas Station date = %q, want %q", tx.Date, "2026-03-15")
				}
				return
			}
		}
		t.Error("did not find Gas Station transaction")
	})

	t.Run("duplicate import rejected", func(t *testing.T) {
		_, err := env.importer.Import(env.csvPath, env.acc.ID)
		if err == nil {
			t.Error("expected error on duplicate import, got nil")
		}
	})
}
