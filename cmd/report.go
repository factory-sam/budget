package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate financial reports",
}

var reportSpendingCmd = &cobra.Command{
	Use:   "spending",
	Short: "Spending by category",
	RunE: func(cmd *cobra.Command, args []string) error {
		month, _ := cmd.Flags().GetString("month")
		y, m := parseMonth(month)

		results, err := service.SpendingReport(y, m)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		fmt.Printf("Spending Report — %d-%02d\n\n", y, m)
		if len(results) == 0 {
			fmt.Println("No expenses this month.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CATEGORY\tAMOUNT\t%\tBAR")
		for _, r := range results {
			barLen := int(r.Percent / 2)
			if barLen > 40 {
				barLen = 40
			}
			bar := strings.Repeat("█", barLen)
			fmt.Fprintf(w, "%s\t$%.2f\t%.1f%%\t%s\n",
				r.CategoryName, float64(r.Amount)/100, r.Percent, bar)
		}
		return w.Flush()
	},
}

var reportNetworthCmd = &cobra.Command{
	Use:   "networth",
	Short: "Net worth summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		snap, err := service.SnapshotNetWorth()
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(snap)
		}

		fmt.Printf("Net Worth — %s\n\n", snap.Date)
		fmt.Printf("  Assets:      $%.2f\n", float64(snap.TotalAssets)/100)
		if snap.EquityValue > 0 {
			cashAssets := snap.TotalAssets - snap.EquityValue
			fmt.Printf("    Cash/Accounts:  $%.2f\n", float64(cashAssets)/100)
			fmt.Printf("    Investments:    $%.2f\n", float64(snap.EquityValue)/100)
		}
		fmt.Printf("  Liabilities: $%.2f\n", float64(snap.TotalLiabilities)/100)
		fmt.Printf("  Net Worth:   $%.2f\n", float64(snap.NetWorth)/100)
		return nil
	},
}

var reportCashflowCmd = &cobra.Command{
	Use:   "cashflow",
	Short: "Cash flow over time",
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")

		if from == "" {
			from = time.Now().AddDate(0, -6, 0).Format("2006-01-02")
		}
		if to == "" {
			to = time.Now().Format("2006-01-02")
		}

		results, err := service.CashFlowReport(from, to)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("No data for the period.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MONTH\tINCOME\tEXPENSES\tNET")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t$%.2f\t$%.2f\t$%.2f\n",
				r.Month, float64(r.Income)/100, float64(r.Expenses)/100, float64(r.Net)/100)
		}
		return w.Flush()
	},
}

func init() {
	reportSpendingCmd.Flags().String("month", "", "target month (YYYY-MM)")
	reportSpendingCmd.Flags().String("format", "table", "output format")
	reportNetworthCmd.Flags().String("format", "table", "output format")
	reportCashflowCmd.Flags().String("from", "", "start date (YYYY-MM-DD)")
	reportCashflowCmd.Flags().String("to", "", "end date (YYYY-MM-DD)")
	reportCashflowCmd.Flags().String("format", "table", "output format")

	reportCmd.AddCommand(reportSpendingCmd, reportNetworthCmd, reportCashflowCmd)
	rootCmd.AddCommand(reportCmd)
}
