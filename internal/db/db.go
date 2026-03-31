package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".budget")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "budget.db")
}

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS category_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	group_id INTEGER NOT NULL REFERENCES category_groups(id),
	name TEXT NOT NULL,
	icon TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	UNIQUE(group_id, name)
);

CREATE TABLE IF NOT EXISTS accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL DEFAULT 'checking',
	balance INTEGER NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'USD',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transactions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	account_id INTEGER NOT NULL REFERENCES accounts(id),
	category_id INTEGER REFERENCES categories(id),
	amount INTEGER NOT NULL,
	date TEXT NOT NULL,
	payee TEXT NOT NULL DEFAULT '',
	note TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT 'expense',
	transfer_pair_id INTEGER,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(date);
CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category_id);

CREATE TABLE IF NOT EXISTS budgets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	category_id INTEGER NOT NULL REFERENCES categories(id),
	year INTEGER NOT NULL,
	month INTEGER NOT NULL,
	amount_limit INTEGER NOT NULL,
	UNIQUE(category_id, year, month)
);

CREATE TABLE IF NOT EXISTS recurring_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	account_id INTEGER NOT NULL REFERENCES accounts(id),
	category_id INTEGER REFERENCES categories(id),
	amount INTEGER NOT NULL,
	payee TEXT NOT NULL DEFAULT '',
	note TEXT NOT NULL DEFAULT '',
	frequency TEXT NOT NULL DEFAULT 'monthly',
	start_date TEXT NOT NULL,
	end_date TEXT,
	next_due TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'expense'
);

CREATE TABLE IF NOT EXISTS networth_snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	date TEXT NOT NULL UNIQUE,
	total_assets INTEGER NOT NULL DEFAULT 0,
	total_liabilities INTEGER NOT NULL DEFAULT 0,
	net_worth INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS import_records (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	filename TEXT NOT NULL,
	hash TEXT NOT NULL UNIQUE,
	imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	tx_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS auto_cat_rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	pattern TEXT NOT NULL,
	category_id INTEGER NOT NULL REFERENCES categories(id),
	UNIQUE(pattern)
);
`

func SeedCategories(db *sql.DB) error {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM category_groups").Scan(&count)
	if count > 0 {
		return nil
	}

	defaults := map[string][]string{
		"Housing":        {"Rent/Mortgage", "Utilities", "Insurance", "Maintenance"},
		"Food & Drink":   {"Groceries", "Restaurants", "Coffee", "Bars"},
		"Transportation": {"Gas", "Public Transit", "Parking", "Car Payment", "Car Insurance"},
		"Shopping":       {"Clothing", "Electronics", "Home Goods", "Gifts"},
		"Entertainment":  {"Streaming", "Games", "Movies", "Music", "Events"},
		"Health":         {"Doctor", "Pharmacy", "Gym", "Dental", "Vision"},
		"Personal":       {"Haircut", "Education", "Subscriptions"},
		"Income":         {"Salary", "Freelance", "Interest", "Refunds", "Other Income"},
		"Transfers":      {"Transfer"},
		"Uncategorized":  {"Uncategorized"},
	}

	order := []string{"Income", "Housing", "Food & Drink", "Transportation", "Shopping", "Entertainment", "Health", "Personal", "Transfers", "Uncategorized"}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, group := range order {
		res, err := tx.Exec("INSERT INTO category_groups (name, sort_order) VALUES (?, ?)", group, i)
		if err != nil {
			return err
		}
		gid, _ := res.LastInsertId()
		for j, cat := range defaults[group] {
			_, err := tx.Exec("INSERT INTO categories (group_id, name, sort_order) VALUES (?, ?, ?)", gid, cat, j)
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
