package cmd

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/sam/budget/internal/db"
	"github.com/sam/budget/internal/equity"
	svc "github.com/sam/budget/internal/service"
	"github.com/spf13/cobra"
)

var (
	dbPath  string
	service *svc.Service
	database *sql.DB
)

var rootCmd = &cobra.Command{
	Use:   "budget",
	Short: "Personal finance manager — CLI & TUI",
	Long:  "A comprehensive budgeting tool for managing personal finances from the terminal.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" || cmd.Name() == "version" {
			return nil
		}
		var err error
		database, err = db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		if err := db.SeedCategories(database); err != nil {
			return fmt.Errorf("seed categories: %w", err)
		}
		service = svc.New(database)
		// wire up equity value for net worth
		ps := equity.NewPriceService(database)
		ps2 := equity.NewPortfolioService(database, ps)
		svc.GetEquityValue = ps2.EquityValueForNetWorth
		svc.GetAccountEquityValue = ps2.EquityValueForAccount
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if database != nil {
			database.Close()
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI(cmd, args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", db.DefaultPath(), "database file path")
}
