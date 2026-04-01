package equity

import (
	"testing"
	"time"

	"github.com/sam/budget/internal/model"
)

func TestCreateLot(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	grantID := int64(99)
	lot, err := portfolio.CreateLot(nil, "TSLA", 5, 75000, "2025-03-01", "rsu_vest", &grantID, "vested lot")
	if err != nil {
		t.Fatalf("CreateLot: %v", err)
	}
	if lot.ID == 0 {
		t.Error("lot ID should be non-zero")
	}
	if lot.Ticker != "TSLA" {
		t.Errorf("ticker = %q, want TSLA", lot.Ticker)
	}
	if lot.Shares != 5 {
		t.Errorf("shares = %v, want 5", lot.Shares)
	}
	if lot.CostBasis != 75000 {
		t.Errorf("cost_basis = %d, want 75000", lot.CostBasis)
	}
	if lot.Source != "rsu_vest" {
		t.Errorf("source = %q, want rsu_vest", lot.Source)
	}
	if lot.GrantID == nil || *lot.GrantID != 99 {
		t.Errorf("grant_id = %v, want 99", lot.GrantID)
	}
	if lot.Note != "vested lot" {
		t.Errorf("note = %q, want 'vested lot'", lot.Note)
	}
	if !lot.IncludeInNetWorth {
		t.Error("expected IncludeInNetWorth to be true by default")
	}

	// Verify it's persisted
	lots, err := portfolio.ListLots("TSLA", nil)
	if err != nil {
		t.Fatalf("ListLots: %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(lots))
	}
}

func TestSetNetWorthInclusion(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	lot, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", "")
	if err != nil {
		t.Fatal(err)
	}

	// Exclude from net worth
	if err := portfolio.SetNetWorthInclusion(lot.ID, false); err != nil {
		t.Fatalf("SetNetWorthInclusion(false): %v", err)
	}

	lots, err := portfolio.ListLots("AAPL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(lots))
	}
	if lots[0].IncludeInNetWorth {
		t.Error("expected IncludeInNetWorth=false after exclusion")
	}

	// Re-include
	if err := portfolio.SetNetWorthInclusion(lot.ID, true); err != nil {
		t.Fatalf("SetNetWorthInclusion(true): %v", err)
	}

	lots, err = portfolio.ListLots("AAPL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !lots[0].IncludeInNetWorth {
		t.Error("expected IncludeInNetWorth=true after re-inclusion")
	}
}

func TestEquityValueForNetWorth(t *testing.T) {
	t.Parallel()
	prices, portfolio, _ := setup(t)

	// Create lots
	if _, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", ""); err != nil {
		t.Fatal(err)
	}
	lot2, err := portfolio.BuyLot(nil, "GOOG", 5, 28000, "2025-02-01", "")
	if err != nil {
		t.Fatal(err)
	}

	// Set prices
	if err := prices.SetManualPrice("AAPL", "2025-03-01", 16000); err != nil {
		t.Fatal(err)
	}
	if err := prices.SetManualPrice("GOOG", "2025-03-01", 30000); err != nil {
		t.Fatal(err)
	}

	// All included by default
	val, err := portfolio.EquityValueForNetWorth()
	if err != nil {
		t.Fatalf("EquityValueForNetWorth: %v", err)
	}
	// AAPL: 10 * 16000 = 160000, GOOG: 5 * 30000 = 150000, total = 310000
	expected := int64(310000)
	if val != expected {
		t.Errorf("EquityValueForNetWorth = %d, want %d", val, expected)
	}

	// Exclude GOOG lot
	if err := portfolio.SetNetWorthInclusion(lot2.ID, false); err != nil {
		t.Fatal(err)
	}
	val, err = portfolio.EquityValueForNetWorth()
	if err != nil {
		t.Fatal(err)
	}
	// Only AAPL: 160000
	if val != 160000 {
		t.Errorf("EquityValueForNetWorth after exclusion = %d, want 160000", val)
	}
}

func TestEquityValueForAccount(t *testing.T) {
	t.Parallel()
	prices, portfolio, _ := setup(t)

	// We need a real account ID — create lots with account_id
	// First insert an account directly
	acctID := int64(1)
	_, err := portfolio.db.Exec(
		"INSERT INTO accounts (id, name, type, balance) VALUES (?, ?, ?, ?)",
		acctID, "Brokerage", "investment", 0)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	if _, err := portfolio.BuyLot(&acctID, "AAPL", 10, 15000, "2025-01-15", ""); err != nil {
		t.Fatal(err)
	}
	// Lot without account
	if _, err := portfolio.BuyLot(nil, "GOOG", 5, 28000, "2025-02-01", ""); err != nil {
		t.Fatal(err)
	}

	if err := prices.SetManualPrice("AAPL", "2025-03-01", 16000); err != nil {
		t.Fatal(err)
	}
	if err := prices.SetManualPrice("GOOG", "2025-03-01", 30000); err != nil {
		t.Fatal(err)
	}

	val, err := portfolio.EquityValueForAccount(acctID)
	if err != nil {
		t.Fatalf("EquityValueForAccount: %v", err)
	}
	// Only AAPL in this account: 10 * 16000 = 160000
	if val != 160000 {
		t.Errorf("EquityValueForAccount = %d, want 160000", val)
	}

	// Non-existent account should return 0
	val, err = portfolio.EquityValueForAccount(999)
	if err != nil {
		t.Fatal(err)
	}
	if val != 0 {
		t.Errorf("EquityValueForAccount(999) = %d, want 0", val)
	}
}

func TestGetDistinctTickers(t *testing.T) {
	t.Parallel()
	_, portfolio, _ := setup(t)

	// Empty portfolio
	tickers, err := portfolio.GetDistinctTickers()
	if err != nil {
		t.Fatalf("GetDistinctTickers empty: %v", err)
	}
	if len(tickers) != 0 {
		t.Errorf("expected 0 tickers, got %d", len(tickers))
	}

	// Add lots with duplicate tickers
	if _, err := portfolio.BuyLot(nil, "AAPL", 10, 15000, "2025-01-15", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "GOOG", 5, 28000, "2025-02-01", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := portfolio.BuyLot(nil, "AAPL", 3, 16000, "2025-03-01", ""); err != nil {
		t.Fatal(err)
	}

	tickers, err = portfolio.GetDistinctTickers()
	if err != nil {
		t.Fatalf("GetDistinctTickers: %v", err)
	}
	if len(tickers) != 2 {
		t.Errorf("expected 2 distinct tickers, got %d", len(tickers))
	}
	// Should be sorted alphabetically
	if len(tickers) == 2 {
		if tickers[0] != "AAPL" {
			t.Errorf("tickers[0] = %q, want AAPL", tickers[0])
		}
		if tickers[1] != "GOOG" {
			t.Errorf("tickers[1] = %q, want GOOG", tickers[1])
		}
	}
}

func TestVestGrant(t *testing.T) {
	t.Parallel()
	prices, _, grants := setup(t)

	// Create an RSU grant with cliff in the past
	pastDate := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "rsu",
		TotalShares:     1200,
		GrantDate:       pastDate,
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "quarterly",
	})
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	// Set manual price so FMV can resolve (used as override anyway)
	if err := prices.SetManualPrice("ACME", time.Now().Format("2006-01-02"), 5000); err != nil {
		t.Fatal(err)
	}

	fmv := int64(5000)
	count, err := grants.VestGrant(grant.ID, &fmv)
	if err != nil {
		t.Fatalf("VestGrant: %v", err)
	}
	if count == 0 {
		t.Error("expected at least some vest events to be processed")
	}

	// Verify vest events were marked as vested
	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	vestedCount := 0
	for _, e := range events {
		if e.Status == "vested" {
			vestedCount++
		}
	}
	if vestedCount != count {
		t.Errorf("vested events = %d, want %d", vestedCount, count)
	}
}

func TestExerciseISO(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	// Create ISO grant with cliff in the past
	pastDate := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "iso",
		TotalShares:     1000,
		GrantDate:       pastDate,
		StrikePrice:     int64Ptr(1000),
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "monthly",
	})
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	// Vest with FMV override (avoid network)
	fmv := int64(2000)
	count, err := grants.VestGrant(grant.ID, &fmv)
	if err != nil {
		t.Fatalf("VestGrant: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one vest event to be processed")
	}

	// Find a vested event to exercise
	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	var vestedEventID int64
	var vestedShares float64
	for _, e := range events {
		if e.Status == "vested" {
			vestedEventID = e.ID
			vestedShares = e.Shares
			break
		}
	}
	if vestedEventID == 0 {
		t.Fatal("no vested event found to exercise")
	}

	// Exercise
	fmvAtExercise := int64(3000)
	lot, err := grants.ExerciseISO(vestedEventID, fmvAtExercise)
	if err != nil {
		t.Fatalf("ExerciseISO: %v", err)
	}
	if lot == nil {
		t.Fatal("ExerciseISO returned nil lot")
	}
	if lot.Ticker != "ACME" {
		t.Errorf("lot ticker = %q, want ACME", lot.Ticker)
	}
	if lot.Source != "iso_exercise" {
		t.Errorf("lot source = %q, want iso_exercise", lot.Source)
	}
	if lot.Shares != vestedShares {
		t.Errorf("lot shares = %v, want %v", lot.Shares, vestedShares)
	}
	// Cost basis = strike * shares = 1000 * shares
	expectedCost := int64(1000 * vestedShares)
	if lot.CostBasis != expectedCost {
		t.Errorf("lot cost_basis = %d, want %d", lot.CostBasis, expectedCost)
	}

	// Verify event is now exercised
	events, err = grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.ID == vestedEventID {
			if e.Status != "exercised" {
				t.Errorf("event status = %q, want exercised", e.Status)
			}
			break
		}
	}
}

func TestExerciseISONotVested(t *testing.T) {
	t.Parallel()
	_, _, grants := setup(t)

	// Create a future grant so no events are vested
	futureDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	grant, err := grants.CreateGrant(model.EquityGrant{
		Ticker:          "ACME",
		GrantType:       "iso",
		TotalShares:     1000,
		GrantDate:       futureDate,
		StrikePrice:     int64Ptr(1000),
		CliffMonths:     12,
		VestingMonths:   48,
		VestingInterval: "monthly",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get a pending event
	events, err := grants.GetVestSchedule(grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected vest events")
	}

	// Try to exercise a pending event — should fail
	_, err = grants.ExerciseISO(events[0].ID, 3000)
	if err == nil {
		t.Error("expected error exercising a pending event")
	}
}
