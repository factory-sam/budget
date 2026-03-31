# budget

A personal finance manager for the terminal. Manage accounts, track transactions, set budgets, monitor investments, and generate reports — all from the command line or an interactive TUI.

Built with Go, SQLite, and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

```sh
go install github.com/sam/budget@latest
```

Or build from source:

```sh
git clone https://github.com/sam/budget.git
cd budget
go build -o budget .
```

## Usage

Run `budget` with no arguments to launch the interactive TUI, or use subcommands for CLI access.

```
budget                         # launch TUI
budget tui                     # launch TUI (explicit)
budget --db /path/to/file.db   # use a custom database path
```

The database is stored at `~/.budget/budget.db` by default.

### Accounts

```sh
budget account add --name "Checking" --type checking --balance 5000
budget account add --name "Brokerage" --type brokerage
budget account list
budget account delete 1
```

Account types: `checking`, `savings`, `credit_card`, `investment`, `loan`, `cash`, `brokerage`, `401k`, `money_market`, `managed`.

### Transactions

```sh
budget tx add --amount 45.99 --payee "Grocery Store" --cat Groceries --account Checking
budget tx add --amount 3000 --payee "Employer" --type income --account Checking
budget tx list --month 2026-03
budget tx list --search "coffee" --limit 20
budget tx delete 42
```

### Import & Export

```sh
budget import transactions.csv --account Checking
budget import bank.ofx --account Checking
budget export --format csv --from 2026-01-01 --to 2026-03-31
budget export --format json
```

Supported import formats: CSV, OFX, QFX.

### Budgets

```sh
budget set Groceries 600
budget set Groceries 600 --month 2026-04
budget status
budget status --month 2026-03 --format json
```

### Categories

```sh
budget category list
budget category add --name "Subscriptions" --group "Personal"
budget category delete 15
```

Payees are auto-categorized based on learned rules.

### Recurring Transactions

```sh
budget recurring add --payee "Netflix" --amount 15.49 --cat Subscriptions --account Checking --freq monthly --start 2026-01-01
budget recurring list
budget recurring generate
budget recurring delete 3
```

Frequencies: `weekly`, `biweekly`, `monthly`, `yearly`.

### Reports

```sh
budget report spending --month 2026-03
budget report networth
budget report cashflow --from 2025-09-01 --to 2026-03-31
budget report income --from 2025-01-01 --group quarterly
```

All report commands support `--format json` for programmatic use.

### Equity Portfolio

```sh
budget equity buy --ticker AAPL --shares 10 --price 185.50 --account Brokerage
budget equity sell --ticker AAPL --shares 5 --price 195.00 --account Brokerage
budget equity portfolio
budget equity positions
budget equity price set --ticker AAPL --price 192.00
budget equity price history --ticker AAPL
```

### Stock Grants (ISOs & RSUs)

```sh
budget equity grant add --ticker ACME --type iso --shares 10000 --strike 5.00 --grant-date 2024-01-15 --cliff 12 --vesting 48
budget equity grant list
budget equity grant schedule 1
budget equity exercise --grant 1 --shares 1000 --account Brokerage
```

### 401(k) Tracking

```sh
budget 401k contribute --account "My 401k" --amount 1500 --match 750
budget 401k status --account "My 401k"
budget 401k limit
```

## Project Structure

```
.
├── main.go              # entrypoint
├── cmd/                 # CLI commands (cobra)
├── tui/                 # interactive terminal UI (bubble tea)
├── internal/
│   ├── db/              # SQLite database, migrations, seed data
│   ├── model/           # domain types
│   ├── service/         # business logic
│   ├── equity/          # portfolio, pricing, grants
│   ├── importer/        # CSV/OFX/QFX import
│   └── recurring/       # recurring transaction engine
└── budget               # compiled binary
```

## License

MIT
