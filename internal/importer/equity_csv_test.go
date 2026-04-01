package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sam/budget/internal/db"
	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type equityTestEnv struct {
	svc       *service.Service
	portfolio *equity.PortfolioService
	importer  *EquityCSVImporter
	account   *model.Account
	tmpDir    string
}

func setupEquityTest(t *testing.T) *equityTestEnv {
	t.Helper()
	tmpDir := t.TempDir()

	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.SeedCategories(database); err != nil {
		t.Fatalf("SeedCategories: %v", err)
	}

	svc := service.New(database)
	prices := equity.NewPriceService(database)
	portfolio := equity.NewPortfolioService(database, prices)

	acc, err := svc.CreateAccount("Test Brokerage", model.AccountBrokerage, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	return &equityTestEnv{
		svc:       svc,
		portfolio: portfolio,
		importer:  NewEquityCSV(svc, portfolio),
		account:   acc,
		tmpDir:    tmpDir,
	}
}

// writeCSV writes content to a temp CSV file and returns its path.
func writeCSV(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// EquityImportResult.String
// ---------------------------------------------------------------------------

func TestEquityImportResultString(t *testing.T) {
	t.Parallel()
	r := EquityImportResult{Purchases: 3, Sales: 1, Dividends: 2, Skipped: 5}
	got := r.String()
	if !strings.Contains(got, "3 purchases") {
		t.Errorf("String() = %q, want '3 purchases'", got)
	}
	if !strings.Contains(got, "1 sales") {
		t.Errorf("String() = %q, want '1 sales'", got)
	}
	if !strings.Contains(got, "2 dividends") {
		t.Errorf("String() = %q, want '2 dividends'", got)
	}
	if !strings.Contains(got, "5 skipped") {
		t.Errorf("String() = %q, want '5 skipped'", got)
	}
}

// ---------------------------------------------------------------------------
// extractRowFields
// ---------------------------------------------------------------------------

func TestExtractRowFields(t *testing.T) {
	t.Parallel()

	cols := equityColMap{
		date: 0, description: 1, symbol: 2, quantity: 3, price: 4, amount: 5,
		settlement: -1, account: -1, txType: -1,
	}
	row := []string{"03/15/2026", " Purchase - AAPL ", " aapl ", " 10 ", " 150.00 ", " 1500.00 "}

	rf := extractRowFields(row, cols)

	if rf.date != "2026-03-15" {
		t.Errorf("date = %q, want 2026-03-15", rf.date)
	}
	if rf.desc != "Purchase - AAPL" {
		t.Errorf("desc = %q, want 'Purchase - AAPL'", rf.desc)
	}
	if rf.symbol != "AAPL" {
		t.Errorf("symbol = %q, want AAPL", rf.symbol)
	}
	if rf.qtyStr != "10" {
		t.Errorf("qtyStr = %q, want '10'", rf.qtyStr)
	}
	if rf.priceStr != "150.00" {
		t.Errorf("priceStr = %q, want '150.00'", rf.priceStr)
	}
	if rf.amountStr != "1500.00" {
		t.Errorf("amountStr = %q, want '1500.00'", rf.amountStr)
	}
}

func TestExtractRowFields_OutOfBounds(t *testing.T) {
	t.Parallel()

	cols := equityColMap{
		date: 0, description: 5, symbol: 10, quantity: -1, price: -1, amount: -1,
		settlement: -1, account: -1, txType: -1,
	}
	row := []string{"2026-03-15", "some data"}

	rf := extractRowFields(row, cols)

	if rf.date != "2026-03-15" {
		t.Errorf("date = %q, want 2026-03-15", rf.date)
	}
	// description index 5 is out of bounds for 2-element row
	if rf.desc != "" {
		t.Errorf("desc = %q, want empty", rf.desc)
	}
	if rf.symbol != "" {
		t.Errorf("symbol = %q, want empty", rf.symbol)
	}
}

// ---------------------------------------------------------------------------
// detectEquityCols
// ---------------------------------------------------------------------------

func TestDetectEquityCols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header []string
		check  func(t *testing.T, m equityColMap)
	}{
		{
			name:   "standard brokerage headers",
			header: []string{"Trade Date", "Settlement Date", "Account", "Description", "Symbol", "Qty", "Price", "Net Amount"},
			check: func(t *testing.T, m equityColMap) {
				if m.date != 0 {
					t.Errorf("date = %d, want 0", m.date)
				}
				if m.settlement != 1 {
					t.Errorf("settlement = %d, want 1", m.settlement)
				}
				if m.account != 2 {
					t.Errorf("account = %d, want 2", m.account)
				}
				if m.description != 3 {
					t.Errorf("description = %d, want 3", m.description)
				}
				if m.symbol != 4 {
					t.Errorf("symbol = %d, want 4", m.symbol)
				}
				if m.quantity != 5 {
					t.Errorf("quantity = %d, want 5", m.quantity)
				}
				if m.price != 6 {
					t.Errorf("price = %d, want 6", m.price)
				}
				if m.amount != 7 {
					t.Errorf("amount = %d, want 7", m.amount)
				}
			},
		},
		{
			name:   "ticker and shares aliases",
			header: []string{"Trade Date", "Desc", "Ticker", "Shares", "Price", "Total"},
			check: func(t *testing.T, m equityColMap) {
				if m.date != 0 {
					t.Errorf("date = %d, want 0", m.date)
				}
				if m.description != 1 {
					t.Errorf("description = %d, want 1", m.description)
				}
				if m.symbol != 2 {
					t.Errorf("symbol = %d, want 2", m.symbol)
				}
				if m.quantity != 3 {
					t.Errorf("quantity = %d, want 3", m.quantity)
				}
				if m.amount != 5 {
					t.Errorf("amount = %d, want 5", m.amount)
				}
			},
		},
		{
			name:   "action and trans type",
			header: []string{"Trade Date", "Action", "Symbol", "Quantity", "Amount"},
			check: func(t *testing.T, m equityColMap) {
				if m.txType != 1 {
					t.Errorf("txType = %d, want 1", m.txType)
				}
				if m.quantity != 3 {
					t.Errorf("quantity = %d, want 3", m.quantity)
				}
			},
		},
		{
			name:   "no matching columns",
			header: []string{"Foo", "Bar", "Baz"},
			check: func(t *testing.T, m equityColMap) {
				if m.date != -1 {
					t.Errorf("date = %d, want -1", m.date)
				}
				if m.symbol != -1 {
					t.Errorf("symbol = %d, want -1", m.symbol)
				}
			},
		},
		{
			name:   "cusip as symbol",
			header: []string{"Trade Date", "CUSIP", "Amount"},
			check: func(t *testing.T, m equityColMap) {
				if m.symbol != 1 {
					t.Errorf("symbol (cusip) = %d, want 1", m.symbol)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			imp := &EquityCSVImporter{}
			m := imp.detectEquityCols(tc.header)
			tc.check(t, m)
		})
	}
}

// ---------------------------------------------------------------------------
// parseRecords
// ---------------------------------------------------------------------------

func TestParseRecords(t *testing.T) {
	t.Parallel()

	csv := `Account Name,Account Number
Brokerage,1234

Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-15,AAPL,Purchase - AAPL,10,150.00,1500.00
2026-02-01,GOOG,Sale - GOOG,-5,200.00,-1000.00
`

	imp := &EquityCSVImporter{}
	records, headerIdx, err := imp.parseRecords(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parseRecords: %v", err)
	}

	// Header row should be detected at the row containing "Trade Date"
	if headerIdx < 0 {
		t.Fatal("headerIdx < 0")
	}
	headerRow := records[headerIdx]
	found := false
	for _, f := range headerRow {
		if strings.Contains(strings.ToLower(f), "trade date") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("header row does not contain 'Trade Date': %v", headerRow)
	}

	dataRows := records[headerIdx+1:]
	if len(dataRows) != 2 {
		t.Errorf("data rows = %d, want 2", len(dataRows))
	}
}

func TestParseRecords_FallbackFirstRow(t *testing.T) {
	t.Parallel()

	csv := `Date,Symbol,Amount
2026-01-15,AAPL,1500.00
`

	imp := &EquityCSVImporter{}
	records, headerIdx, err := imp.parseRecords(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parseRecords: %v", err)
	}

	// No "Trade Date" keyword, but "Symbol" should trigger detection
	if headerIdx < 0 {
		t.Fatal("headerIdx should be >= 0")
	}
	if len(records) < 2 {
		t.Fatalf("expected at least 2 records, got %d", len(records))
	}
}

func TestParseRecords_Empty(t *testing.T) {
	t.Parallel()

	imp := &EquityCSVImporter{}
	_, _, err := imp.parseRecords(strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

// ---------------------------------------------------------------------------
// sortDataRowsChronologically
// ---------------------------------------------------------------------------

func TestSortDataRowsChronologically(t *testing.T) {
	t.Parallel()

	cols := equityColMap{date: 0, symbol: 1, description: -1, quantity: -1, price: -1, amount: -1, settlement: -1, account: -1, txType: -1}

	t.Run("newest-first gets reversed", func(t *testing.T) {
		rows := [][]string{
			{"2026-03-15", "AAPL"},
			{"2026-02-01", "GOOG"},
			{"2026-01-10", "MSFT"},
		}
		sortDataRowsChronologically(rows, cols)
		if rows[0][0] != "2026-01-10" {
			t.Errorf("first row date = %q, want 2026-01-10", rows[0][0])
		}
		if rows[2][0] != "2026-03-15" {
			t.Errorf("last row date = %q, want 2026-03-15", rows[2][0])
		}
	})

	t.Run("already chronological stays same", func(t *testing.T) {
		rows := [][]string{
			{"2026-01-10", "MSFT"},
			{"2026-02-01", "GOOG"},
			{"2026-03-15", "AAPL"},
		}
		sortDataRowsChronologically(rows, cols)
		if rows[0][0] != "2026-01-10" {
			t.Errorf("first row date = %q, want 2026-01-10", rows[0][0])
		}
	})

	t.Run("single row no-op", func(t *testing.T) {
		rows := [][]string{{"2026-01-10", "AAPL"}}
		sortDataRowsChronologically(rows, cols)
		if rows[0][0] != "2026-01-10" {
			t.Errorf("row date = %q, want 2026-01-10", rows[0][0])
		}
	})

	t.Run("empty rows no-op", func(t *testing.T) {
		rows := [][]string{}
		sortDataRowsChronologically(rows, cols) // should not panic
	})
}

// ---------------------------------------------------------------------------
// transactionForDividend
// ---------------------------------------------------------------------------

func TestTransactionForDividend(t *testing.T) {
	t.Parallel()

	catID := int64(42)
	tx := transactionForDividend(1, &catID, 5000, "2026-03-15", "AAPL", "Dividend - AAPL Q1")

	if tx.AccountID != 1 {
		t.Errorf("AccountID = %d, want 1", tx.AccountID)
	}
	if *tx.CategoryID != 42 {
		t.Errorf("CategoryID = %d, want 42", *tx.CategoryID)
	}
	if tx.Amount != 5000 {
		t.Errorf("Amount = %d, want 5000", tx.Amount)
	}
	if tx.Date != "2026-03-15" {
		t.Errorf("Date = %q, want 2026-03-15", tx.Date)
	}
	if !strings.Contains(tx.Payee, "AAPL") {
		t.Errorf("Payee = %q, want to contain AAPL", tx.Payee)
	}
	if tx.Type != "income" {
		t.Errorf("Type = %q, want income", tx.Type)
	}
}

// ---------------------------------------------------------------------------
// Integration: full equity CSV import
// ---------------------------------------------------------------------------

func TestEquityCSVImport_Purchases(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-15,AAPL,Purchase - AAPL,10,150.00,1500.00
2026-02-01,GOOG,Purchase - GOOG,5,200.00,1000.00
`
	path := writeCSV(t, env.tmpDir, "purchases.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Purchases != 2 {
		t.Errorf("Purchases = %d, want 2", result.Purchases)
	}

	// Verify lots were created
	lots, err := env.portfolio.ListLots("AAPL", &env.account.ID)
	if err != nil {
		t.Fatalf("ListLots AAPL: %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("AAPL lots = %d, want 1", len(lots))
	}
	if lots[0].Shares != 10 {
		t.Errorf("AAPL shares = %v, want 10", lots[0].Shares)
	}

	googLots, err := env.portfolio.ListLots("GOOG", &env.account.ID)
	if err != nil {
		t.Fatalf("ListLots GOOG: %v", err)
	}
	if len(googLots) != 1 {
		t.Fatalf("GOOG lots = %d, want 1", len(googLots))
	}
}

func TestEquityCSVImport_Sales(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	// First buy some shares
	_, err := env.portfolio.BuyLot(&env.account.ID, "AAPL", 10, 15000, "2026-01-01", "pre-buy")
	if err != nil {
		t.Fatalf("BuyLot: %v", err)
	}

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-02-15,AAPL,Sale - AAPL,-5,160.00,-800.00
`
	path := writeCSV(t, env.tmpDir, "sales.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Sales != 1 {
		t.Errorf("Sales = %d, want 1", result.Sales)
	}

	lots, _ := env.portfolio.ListLots("AAPL", &env.account.ID)
	if len(lots) != 1 {
		t.Fatalf("AAPL lots = %d, want 1", len(lots))
	}
	if lots[0].Shares != 5 {
		t.Errorf("AAPL remaining shares = %v, want 5", lots[0].Shares)
	}
}

func TestEquityCSVImport_Dividends(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-03-01,AAPL,Dividend - AAPL,,,25.50
`
	path := writeCSV(t, env.tmpDir, "dividends.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Dividends != 1 {
		t.Errorf("Dividends = %d, want 1", result.Dividends)
	}
}

func TestEquityCSVImport_Deposits(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	// Use a money_market account to test deposit via CreateTransaction path
	mmAcc, err := env.svc.CreateAccount("Money Market", model.AccountMoneyMarket, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-03-01,,Wire Funds Received,,,5000.00
`
	path := writeCSV(t, env.tmpDir, "deposits.csv", csv)

	result, err := env.importer.Import(path, &mmAcc.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Dividends != 1 { // deposits increment Dividends counter
		t.Errorf("Dividends (deposit count) = %d, want 1", result.Dividends)
	}
}

func TestEquityCSVImport_MixedTransactions(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-10,AAPL,Purchase - AAPL,10,150.00,1500.00
2026-01-15,AAPL,Dividend - AAPL,,,12.50
2026-02-01,,Wire Funds Received,,,2000.00
2026-02-15,,Random unknown transaction,,,100.00
`
	path := writeCSV(t, env.tmpDir, "mixed.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Purchases != 1 {
		t.Errorf("Purchases = %d, want 1", result.Purchases)
	}
	if result.Dividends != 2 { // 1 dividend + 1 deposit
		t.Errorf("Dividends = %d, want 2", result.Dividends)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestEquityCSVImport_DuplicateRejected(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-15,AAPL,Purchase - AAPL,10,150.00,1500.00
`
	path := writeCSV(t, env.tmpDir, "dup.csv", csv)

	_, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}

	_, err = env.importer.Import(path, &env.account.ID)
	if err == nil {
		t.Error("expected error on duplicate import, got nil")
	}
	if !strings.Contains(err.Error(), "already imported") {
		t.Errorf("error = %q, want to contain 'already imported'", err.Error())
	}
}

func TestEquityCSVImport_MissingRequiredColumns(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Foo,Bar,Baz
a,b,c
`
	path := writeCSV(t, env.tmpDir, "bad.csv", csv)

	_, err := env.importer.Import(path, &env.account.ID)
	if err == nil {
		t.Error("expected error for missing required columns")
	}
}

func TestEquityCSVImport_TransferIn(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-15,TSLA,Security Transfer In - TSLA,20,250.00,5000.00
`
	path := writeCSV(t, env.tmpDir, "transfer_in.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Purchases != 1 { // transfer_in counts as purchase
		t.Errorf("Purchases = %d, want 1", result.Purchases)
	}

	lots, _ := env.portfolio.ListLots("TSLA", &env.account.ID)
	if len(lots) != 1 {
		t.Fatalf("TSLA lots = %d, want 1", len(lots))
	}
	if lots[0].Shares != 20 {
		t.Errorf("TSLA shares = %v, want 20", lots[0].Shares)
	}
}

func TestEquityCSVImport_SkippedRows(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-01-15,AAPL,Purchase - AAPL,,,
2026-01-16,AAPL,Reinvestment Shares allocated,,,0.00
`
	path := writeCSV(t, env.tmpDir, "skipped.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Purchase with no qty/price should be skipped; reinvest_shares also skipped
	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
	if result.Purchases != 0 {
		t.Errorf("Purchases = %d, want 0", result.Purchases)
	}
}

func TestEquityCSVImport_NewestFirstSorted(t *testing.T) {
	t.Parallel()
	env := setupEquityTest(t)

	// CSV in reverse chronological order; import should still work
	csv := `Trade Date,Symbol,Description,Qty,Price,Net Amount
2026-03-15,GOOG,Purchase - GOOG,5,200.00,1000.00
2026-01-10,AAPL,Purchase - AAPL,10,150.00,1500.00
`
	path := writeCSV(t, env.tmpDir, "reversed.csv", csv)

	result, err := env.importer.Import(path, &env.account.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Purchases != 2 {
		t.Errorf("Purchases = %d, want 2", result.Purchases)
	}
}
