package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var fourOneKCmd = &cobra.Command{
	Use:   "401k",
	Short: "Manage 401k contributions and limits",
}

var fourOneKContributeCmd = &cobra.Command{
	Use:   "contribute",
	Short: "Record a 401k contribution",
	RunE: func(cmd *cobra.Command, args []string) error {
		accountName, _ := cmd.Flags().GetString("account")
		amount, _ := cmd.Flags().GetFloat64("amount")
		match, _ := cmd.Flags().GetFloat64("match")

		if accountName == "" {
			return fmt.Errorf("--account is required")
		}
		if amount == 0 {
			return fmt.Errorf("--amount is required")
		}

		acc, err := service.GetAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account not found: %s", accountName)
		}

		year := time.Now().Year()
		empCents := int64(math.Round(amount * 100))
		matchCents := int64(math.Round(match * 100))

		status, err := service.Contribute401k(acc.ID, year, empCents, matchCents)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(status)
		}

		fmt.Printf("Contributed $%.2f (+ $%.2f match) to %s\n", amount, match, acc.Name)
		fmt.Printf("  YTD Employee: $%.2f / $%.2f (%.1f%%)\n",
			float64(status.EmployeeContrib)/100, float64(status.AnnualLimit)/100, status.Percent)
		if status.Percent >= 100 {
			fmt.Println("  ⚠ Annual contribution limit reached!")
		}
		return nil
	},
}

var fourOneKStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show 401k contribution status",
	RunE: func(cmd *cobra.Command, args []string) error {
		accountName, _ := cmd.Flags().GetString("account")
		if accountName == "" {
			return fmt.Errorf("--account is required")
		}
		acc, err := service.GetAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account not found: %s", accountName)
		}

		year := time.Now().Year()
		status, err := service.Get401kStatus(acc.ID, year)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(status)
		}

		fmt.Printf("401k Status — %s (%d)\n\n", acc.Name, year)
		fmt.Printf("  Employee Contributions: $%.2f\n", float64(status.EmployeeContrib)/100)
		fmt.Printf("  Employer Match:         $%.2f\n", float64(status.EmployerMatch)/100)
		fmt.Printf("  Match Rate:             %.1f%%\n", status.MatchPercent)
		fmt.Printf("  Total:                  $%.2f\n", float64(status.TotalContrib)/100)
		fmt.Printf("  Annual Limit:           $%.2f\n", float64(status.AnnualLimit)/100)

		pct := status.Percent
		barW := 40
		filled := int(pct / 100 * float64(barW))
		if filled > barW {
			filled = barW
		}
		empty := barW - filled
		bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
		fmt.Printf("  Progress:               %s %.1f%%\n", bar, pct)
		if status.Remaining > 0 {
			fmt.Printf("  Remaining:              $%.2f\n", float64(status.Remaining)/100)
		} else {
			fmt.Println("  ⚠ Annual limit reached!")
		}
		return nil
	},
}

var fourOneKSetMatchCmd = &cobra.Command{
	Use:   "set-match",
	Short: "Set employer match percentage",
	RunE: func(cmd *cobra.Command, args []string) error {
		accountName, _ := cmd.Flags().GetString("account")
		pct, _ := cmd.Flags().GetFloat64("percent")
		if accountName == "" {
			return fmt.Errorf("--account is required")
		}
		acc, err := service.GetAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account not found: %s", accountName)
		}
		year := time.Now().Year()
		if err := service.Set401kMatchPercent(acc.ID, year, pct); err != nil {
			return err
		}
		fmt.Printf("Set employer match to %.1f%% for %s\n", pct, acc.Name)
		return nil
	},
}

var fourOneKSetLimitCmd = &cobra.Command{
	Use:   "set-limit",
	Short: "Override annual contribution limit",
	RunE: func(cmd *cobra.Command, args []string) error {
		accountName, _ := cmd.Flags().GetString("account")
		limit, _ := cmd.Flags().GetFloat64("limit")
		if accountName == "" {
			return fmt.Errorf("--account is required")
		}
		acc, err := service.GetAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account not found: %s", accountName)
		}
		year := time.Now().Year()
		cents := int64(math.Round(limit * 100))
		if err := service.Set401kAnnualLimit(acc.ID, year, cents); err != nil {
			return err
		}
		fmt.Printf("Set annual limit to $%.2f for %s\n", limit, acc.Name)
		return nil
	},
}

func init() {
	fourOneKContributeCmd.Flags().String("account", "", "401k account name")
	fourOneKContributeCmd.Flags().Float64("amount", 0, "employee contribution amount")
	fourOneKContributeCmd.Flags().Float64("match", 0, "employer match amount")
	fourOneKContributeCmd.Flags().String("format", "table", "output format")

	fourOneKStatusCmd.Flags().String("account", "", "401k account name")
	fourOneKStatusCmd.Flags().String("format", "table", "output format")

	fourOneKSetMatchCmd.Flags().String("account", "", "401k account name")
	fourOneKSetMatchCmd.Flags().Float64("percent", 0, "match percentage")

	fourOneKSetLimitCmd.Flags().String("account", "", "401k account name")
	fourOneKSetLimitCmd.Flags().Float64("limit", 0, "annual limit in dollars")

	fourOneKCmd.AddCommand(fourOneKContributeCmd, fourOneKStatusCmd, fourOneKSetMatchCmd, fourOneKSetLimitCmd)
	rootCmd.AddCommand(fourOneKCmd)
}
