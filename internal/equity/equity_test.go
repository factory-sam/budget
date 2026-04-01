package equity

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/sam/budget/internal/db"
	"github.com/sam/budget/internal/model"
)

// setup creates an isolated test DB and returns the three services.
func setup(t *testing.T) (*PriceService, *PortfolioService, *GrantService) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.SeedCategories(sqlDB); err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	prices := NewPriceService(sqlDB)
	portfolio := NewPortfolioService(sqlDB, prices)
	grants := NewGrantService(sqlDB, prices, portfolio)
	return prices, portfolio, grants
}

// ---- Portfolio Tests ----

func TestBuyLot(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	lot, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", "test buy")
	if err != nil {
		t.Fatalf("BuyLot: %v", err)
	}
	if lot.Ticker != "AAPL" {
		t.Errorf("ticker = %q, want AAPL", lot.Ticker)
	}
	if lot.Shares != 10 {
		t.Errorf("shares = %v, want 10", lot.Shares)
	}
	expectedCost := int64(math.Round(float64(15000) * 10))
	if lot.CostBasis != expectedCost {
		t.Errorf("cost_basis = %d, want %d", lot.CostBasis, expectedCost)
	}
	if lot.Source != "buy" {
		t.Errorf("source = %q, want buy", lot.Source)
	}
	if lot.DateAcquired != "2025-01-15" {
		t.Errorf("date_acquired = %q, want 2025-01-15", lot.DateAcquired)
	}
	if lot.Note != "test buy" {
		t.Errorf("note = %q, want 'test buy'", lot.Note)
	}
	if lot.ID == 0 {
		t.Error("lot ID should be non-zero")
	}
}

func TestListLots(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	if _, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "GOOG", 5, 28000, "2025-02-01", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "AAPL", 3, 16000, "2025-03-01", ""); err != nil {
		t.Fatal(err)
	}

	// list all
	all, err := portfolio.ListLots("", nil)
	if err != nil {
		t.Fatalf("ListLots all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListLots all: got %d lots, want 3", len(all))
	}

	// filter by ticker
	appl, err := portfolio.ListLots("AAPL", nil)
	if err != nil {
		t.Fatalf("ListLots AAPL: %v", err)
	}
	if len(appl) != 2 {
		t.Errorf("ListLots AAPL: got %d lots, want 2", len(appl))
	}

	goog, err := portfolio.ListLots("GOOG", nil)
	if err != nil {
		t.Fatalf("ListLots GOOG: %v", err)
	}
	if len(goog) != 1 {
		t.Errorf("ListLots GOOG: got %d lots, want 1", len(goog))
	}
}

func TestSellLotPartial(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	lot, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", "")
	if err != nil {
		t.Fatal(err)
	}
	origCost := lot.CostBasis // 150000

	if err := portfolio.SellLot(lot.ID, 4); err != nil {
		t.Fatalf("SellLot: %v", err)
	}

	lots, err := portfolio.ListLots("AAPL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected 1 lot remaining, got %d", len(lots))
	}

	remaining := lots[0]
	if remaining.Shares != 6 {
		t.Errorf("remaining shares = %v, want 6", remaining.Shares)
	}
	// cost basis should be proportional: original * (6/10)
	expectedCost := int64(float64(origCost) * 6.0 / 10.0)
	if remaining.CostBasis != expectedCost {
		t.Errorf("remaining cost_basis = %d, want %d", remaining.CostBasis, expectedCost)
	}
}

func TestSellLotFull(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	lot, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := portfolio.SellLot(lot.ID, 10); err != nil {
		t.Fatalf("SellLot full: %v", err)
	}

	lots, err := portfolio.ListLots("AAPL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 0 {
		t.Errorf("expected 0 lots after full sell, got %d", len(lots))
	}
}

func TestGetPortfolio(t *testing.T) {
	t.Parallel()
	prices, portfolio, _ := setup(t)

	// Set prices so market value can be computed
	if err := prices.SetManualPrice("AAPL", "2025-01-15", 15500); err != nil {
		t.Fatal(err)
	}
	if err := prices.SetManualPrice("GOOG", "2025-01-15", 29000); err != nil {
		t.Fatal(err)
	}

	if _, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "AAPL", 5, 15200, "2025-02-01", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "GOOG", 3, 28000, "2025-01-20", ""); err != nil {
		t.Fatal(err)
	}

	summary, err := portfolio.GetPortfolio(nil)
	if err != nil {
		t.Fatalf("GetPortfolio: %v", err)
	}

	if len(summary.Positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(summary.Positions))
	}

	// Verify totals are non-zero (prices are set)
	if summary.TotalValue == 0 {
		t.Error("TotalValue should be > 0 with prices set")
	}
	if summary.TotalCostBasis == 0 {
		t.Error("TotalCostBasis should be > 0")
	}

	// Find positions by ticker
	posMap := make(map[string]model.PositionSummary)
	for _, p := range summary.Positions {
		posMap[p.Ticker] = p
	}
	aaplPos, ok := posMap["AAPL"]
	if !ok {
		t.Fatal("AAPL position not found")
	}
	if aaplPos.TotalShares != 15 {
		t.Errorf("AAPL total shares = %v, want 15", aaplPos.TotalShares)
	}
	googPos, ok := posMap["GOOG"]
	if !ok {
		t.Fatal("GOOG position not found")
	}
	if googPos.TotalShares != 3 {
		t.Errorf("GOOG total shares = %v, want 3", googPos.TotalShares)
	}
}

// ---- Grant Tests ----

func TestCreateGrant(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "iso",
		TotalShares:     4800,
		GrantDate:       "2025-01-01",
		StrikePrice:     int64Ptr(1000),
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "monthly",
		Note:            "initial grant",
	})
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if grant.ID == 0 {
		t.Error("grant ID should be non-zero")
	}
	if grant.Ticker != "ACME" {
		t.Errorf("ticker = %q, want ACME", grant.Ticker)
	}
	if grant.GrantType != "iso" {
		t.Errorf("grant_type = %q, want iso", grant.GrantType)
	}

	// Verify vest events were created
	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatalf("GetVestSchedule: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected vest events to be created")
	}

	// 1 cliff + 36 monthly = 37 total events
	// cliff at 12 months, then monthly for remaining 36 months
	expectedEvents := 37
	if len(events) != expectedEvents {
		t.Errorf("vest events: got %d, want %d", len(events), expectedEvents)
	}

	// Sum of all event shares should equal total grant shares
	var totalShares float64
	for _, e := range events {
		totalShares += e.Shares
	}
	if math.Abs(totalShares-4800) > 0.01 {
		t.Errorf("sum of vest shares = %v, want 4800", totalShares)
	}
}

func TestGetVestSchedule(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "rsu",
		TotalShares:     1200,
		GrantDate:       "2025-01-01",
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "quarterly",
		Note:            "",
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatal(err)
	}

	// quarterly vesting: cliff at 12 months, then quarterly for 36 months = 12 quarters + 1 cliff = 13
	expectedEvents := 13
	if len(events) != expectedEvents {
		t.Errorf("vest events: got %d, want %d", len(events), expectedEvents)
	}

	// First event should be the cliff (12 months from grant)
	if len(events) > 0 {
		cliffDate, _ := time.Parse("2006-01-02", "2026-01-01")
		if events[0].Date != cliffDate.Format("2006-01-02") {
			t.Errorf("cliff date = %q, want %q", events[0].Date, cliffDate.Format("2006-01-02"))
		}
		if events[0].Status != "pending" {
			t.Errorf("cliff status = %q, want pending", events[0].Status)
		}
	}

	// All events should be pending
	for i, e := range events {
		if e.Status != "pending" {
			t.Errorf("event[%d] status = %q, want pending", i, e.Status)
		}
		if e.Shares <= 0 {
			t.Errorf("event[%d] shares = %v, want > 0", i, e.Shares)
		}
	}

	// Sum of shares should equal total
	var totalShares float64
	for _, e := range events {
		totalShares += e.Shares
	}
	if math.Abs(totalShares-1200) > 0.01 {
		t.Errorf("sum of vest shares = %v, want 1200", totalShares)
	}
}

func TestListGrantsAndGetGrant(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	g1, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "iso",
		TotalShares:     1000,
		GrantDate:       "2025-01-01",
		StrikePrice:     int64Ptr(500),
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "monthly",
	})
	if err != nil {
		t.Fatal(err)
	}

	g2, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "rsu",
		TotalShares:     2000,
		GrantDate:       "2025-06-01",
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "quarterly",
	})
	if err != nil {
		t.Fatal(err)
	}

	// ListGrants
	list, err := grants.ListGrants()
	if err != nil {
		t.Fatalf("ListGrants: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListGrants: got %d, want 2", len(list))
	}

	// GetGrant for each
	got1, err := grants.GetGrant(g1.ID)
	if err != nil {
		t.Fatalf("GetGrant(%d): %v", g1.ID, err)
	}
	if got1.GrantType != "iso" {
		t.Errorf("grant 1 type = %q, want iso", got1.GrantType)
	}
	if got1.TotalShares != 1000 {
		t.Errorf("grant 1 total_shares = %v, want 1000", got1.TotalShares)
	}

	got2, err := grants.GetGrant(g2.ID)
	if err != nil {
		t.Fatalf("GetGrant(%d): %v", g2.ID, err)
	}
	if got2.GrantType != "rsu" {
		t.Errorf("grant 2 type = %q, want rsu", got2.GrantType)
	}
	if got2.TotalShares != 2000 {
		t.Errorf("grant 2 total_shares = %v, want 2000", got2.TotalShares)
	}
}

func TestDeleteGrant(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "rsu",
		TotalShares:     500,
		GrantDate:       "2025-01-01",
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "monthly",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := grants.DeleteGrant(grant.ID); err != nil {
		t.Fatalf("DeleteGrant: %v", err)
	}

	// Verify grant is gone
	_, err = grants.GetGrant(grant.ID)
	if err == nil {
		t.Error("expected error getting deleted grant, got nil")
	}

	// Verify vest events are also deleted
	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatalf("GetVestSchedule after delete: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 vest events after delete, got %d", len(events))
	}
}

// ---- Price Tests ----

func TestSetManualPrice(t *testing.T) {
	t.Parallel()
	prices, _, _ := setup(t)

	if err := prices.SetManualPrice("TSLA", "2025-03-15", 25000); err != nil {
		t.Fatalf("SetManualPrice: %v", err)
	}

	got, err := prices.GetCachedPrice("TSLA")
	if err != nil {
		t.Fatalf("GetCachedPrice: %v", err)
	}
	if got != 25000 {
		t.Errorf("cached price = %d, want 25000", got)
	}
}

func TestGetHistoricalPrices(t *testing.T) {
	t.Parallel()
	prices, _, _ := setup(t)

	// Set multiple prices for same ticker on different dates
	dates := []struct {
		date  string
		price int64
	}{
		{"2025-01-01", 10000},
		{"2025-01-02", 10100},
		{"2025-01-03", 10200},
		{"2025-01-04", 10050},
		{"2025-01-05", 10300},
	}
	for _, d := range dates {
		if err := prices.SetManualPrice("AAPL", d.date, d.price); err != nil {
			t.Fatalf("SetManualPrice(%s): %v", d.date, err)
		}
	}

	// Get all historical prices
	history, err := prices.GetHistoricalPrices("AAPL", 10)
	if err != nil {
		t.Fatalf("GetHistoricalPrices: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("history length = %d, want 5", len(history))
	}

	// Results are ordered DESC by date
	if len(history) > 0 && history[0].Date != "2025-01-05" {
		t.Errorf("first history date = %q, want 2025-01-05", history[0].Date)
	}
	if len(history) > 0 && history[0].Price != 10300 {
		t.Errorf("first history price = %d, want 10300", history[0].Price)
	}

	// Verify limit works
	limited, err := prices.GetHistoricalPrices("AAPL", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 3 {
		t.Errorf("limited history length = %d, want 3", len(limited))
	}
}

// ---- Helpers ----

func int64Ptr(v int64) *int64 {
	return &v
}
