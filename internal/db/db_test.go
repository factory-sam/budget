package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// openBareDB creates a DB with only the schema (no migrations/migrateCategories)
// so SeedCategories can be tested in isolation.
func openBareDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "bare.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("exec schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil *sql.DB")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSchemaTablesExist(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)

	expectedTables := []string{
		"accounts",
		"transactions",
		"categories",
		"category_groups",
		"budgets",
		"recurring_rules",
		"equity_lots",
		"equity_grants",
		"equity_vest_events",
		"equity_prices",
		"networth_snapshots",
		"import_records",
		"auto_cat_rules",
		"account_balance_history",
		"contributions_401k",
	}

	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestSeedCategories(t *testing.T) {
	t.Parallel()
	db := openBareDB(t)

	if err := SeedCategories(db); err != nil {
		t.Fatalf("SeedCategories: %v", err)
	}

	// Verify category_groups populated (10 default groups)
	var groupCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM category_groups").Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if groupCount != 10 {
		t.Errorf("expected 10 category_groups, got %d", groupCount)
	}

	// Verify categories populated
	var catCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&catCount); err != nil {
		t.Fatal(err)
	}
	if catCount < 30 {
		t.Errorf("expected at least 30 categories, got %d", catCount)
	}

	// Check specific group names
	for _, name := range []string{"Housing", "Income", "Food & Drink", "Transfers"} {
		var found int
		err := db.QueryRow("SELECT COUNT(*) FROM category_groups WHERE name=?", name).Scan(&found)
		if err != nil || found == 0 {
			t.Errorf("category_group %q not found", name)
		}
	}

	// Check specific category names
	for _, name := range []string{"Groceries", "Salary", "Rent/Mortgage"} {
		var found int
		err := db.QueryRow("SELECT COUNT(*) FROM categories WHERE name=?", name).Scan(&found)
		if err != nil || found == 0 {
			t.Errorf("category %q not found", name)
		}
	}
}

func TestSeedCategoriesIdempotent(t *testing.T) {
	t.Parallel()
	db := openBareDB(t)

	if err := SeedCategories(db); err != nil {
		t.Fatalf("first SeedCategories: %v", err)
	}
	var countBefore int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	var groupsBefore int
	if err := db.QueryRow("SELECT COUNT(*) FROM category_groups").Scan(&groupsBefore); err != nil {
		t.Fatal(err)
	}

	if err := SeedCategories(db); err != nil {
		t.Fatalf("second SeedCategories: %v", err)
	}
	var countAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	var groupsAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM category_groups").Scan(&groupsAfter); err != nil {
		t.Fatal(err)
	}

	if countBefore != countAfter {
		t.Errorf("categories not idempotent: before=%d after=%d", countBefore, countAfter)
	}
	if groupsBefore != groupsAfter {
		t.Errorf("category_groups not idempotent: before=%d after=%d", groupsBefore, groupsAfter)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open on same path: %v", err)
	}
	defer db2.Close()

	if err := db2.Ping(); err != nil {
		t.Fatalf("Ping after second Open: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()
	p := DefaultPath()
	if p == "" {
		t.Fatal("DefaultPath returned empty string")
	}
	if !strings.HasSuffix(p, filepath.Join(".budget", "budget.db")) {
		t.Errorf("DefaultPath = %q; want suffix .budget/budget.db", p)
	}
}
