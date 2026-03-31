# agents.md

## Project Overview

**budget** is a personal finance CLI and TUI application written in Go. It uses SQLite for storage, Cobra for CLI commands, and Bubble Tea / Lip Gloss for the terminal UI.

## Tech Stack

- **Language:** Go 1.25+
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **CLI framework:** `github.com/spf13/cobra`
- **TUI framework:** `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss`
- **Charts:** `github.com/NimbleMarkets/ntcharts`
- **Bank imports:** `github.com/aclindsa/ofxgo` (OFX/QFX parsing)

## Architecture

### Directory Layout

| Path | Purpose |
|---|---|
| `main.go` | Entrypoint, calls `cmd.Execute()` |
| `cmd/` | Cobra command definitions — one file per resource (account, tx, report, equity, recurring, etc.) |
| `tui/` | Bubble Tea TUI — `app.go` is the root model; separate files for dashboard, transactions, accounts, budgets, portfolio, reports, recurring views |
| `internal/model/` | Domain types (`Account`, `Transaction`, `Budget`, `EquityLot`, `EquityGrant`, etc.) |
| `internal/service/` | Business logic layer; all DB queries go through `Service` |
| `internal/db/` | Database open/migrate, schema, seed data, default path (`~/.budget/budget.db`) |
| `internal/equity/` | Portfolio, pricing, and grant services for investment tracking |
| `internal/importer/` | CSV and OFX/QFX file importers |
| `internal/recurring/` | Recurring transaction generation logic |

### Key Patterns

- **Amounts are stored in cents** (`int64`). Display conversion happens at the CLI/TUI layer.
- **Service layer:** `internal/service.Service` wraps `*sql.DB` and provides all CRUD + report methods. Both CLI commands and TUI views share the same service instance.
- **Equity wiring:** `equity.PortfolioService` is wired into the service layer via function variables (`svc.GetEquityValue`, `svc.GetAccountEquityValue`) so net worth calculations include investment positions.
- **Database:** Auto-migrated on open. WAL mode + foreign keys enabled. Default location is `~/.budget/budget.db`, overridable with `--db` flag.
- **CLI output:** All list/report commands support `--format json` alongside the default table output.
- **TUI:** The root `App` model in `tui/app.go` manages view switching. Each view (dashboard, transactions, accounts, etc.) is its own Bubble Tea model.

### Account Types

`checking`, `savings`, `credit_card`, `investment`, `loan`, `cash`, `brokerage`, `401k`, `money_market`, `managed`

### Transaction Types

`expense`, `income`, `transfer`

## Build & Run

```sh
go build -o budget .
./budget          # launches TUI
./budget --help   # shows all commands
```

## Adding a New CLI Command

1. Create a new file in `cmd/` (e.g., `cmd/newcmd.go`).
2. Define Cobra commands and register them with `rootCmd.AddCommand(...)` in `init()`.
3. Use `service` (set up in `rootCmd.PersistentPreRunE`) for all data access.
4. Support `--format json` for machine-readable output.

## Adding a New TUI View

1. Create a new file in `tui/` implementing the `tea.Model` interface.
2. Use `lipgloss` styles from `tui/styles.go`.
3. Register the view in `tui/app.go`'s view switching logic.

## Common Pitfalls

- The `budget` binary is a compiled file at the repo root — do not confuse it with a Go package.
- Always use cents for amount storage/logic; only convert to dollars at the display boundary.
- The `service` variable in `cmd/` is `nil` until `PersistentPreRunE` runs — do not access it in `init()`.
