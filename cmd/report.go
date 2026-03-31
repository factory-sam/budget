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

		// Find max for scaling bars
		var maxVal int64
		for _, r := range results {
			if r.Income > maxVal {
				maxVal = r.Income
			}
			if r.Expenses > maxVal {
				maxVal = r.Expenses
			}
		}

		barWidth := 30
		fmt.Printf("Cash Flow — %s to %s\n\n", from[:7], to[:7])
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MONTH\tINCOME\tEXPENSES\tNET")
		for _, r := range results {
			netSign := "+"
			if r.Net < 0 {
				netSign = ""
			}
			fmt.Fprintf(w, "%s\t$%10.2f\t$%10.2f\t%s$%.2f\n",
				r.Month, float64(r.Income)/100, float64(r.Expenses)/100,
				netSign, float64(r.Net)/100)
		}
		w.Flush()

		// Bar chart
		fmt.Println()
		for _, r := range results {
			incBar := 0
			expBar := 0
			if maxVal > 0 {
				incBar = int(float64(r.Income) / float64(maxVal) * float64(barWidth))
				expBar = int(float64(r.Expenses) / float64(maxVal) * float64(barWidth))
			}
			incPad := strings.Repeat("░", barWidth-incBar)
			expPad := strings.Repeat("░", barWidth-expBar)
			fmt.Printf("  %s  IN  %s%s  $%.0f\n", r.Month, strings.Repeat("█", incBar), incPad, float64(r.Income)/100)
			fmt.Printf("            OUT %s%s  $%.0f\n", strings.Repeat("█", expBar), expPad, float64(r.Expenses)/100)
		}
		return nil
	},
}

var reportIncomeCmd = &cobra.Command{
	Use:   "income",
	Short: "Income by source over time (monthly, quarterly, yearly)",
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		groupBy, _ := cmd.Flags().GetString("group")

		if from == "" {
			from = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
		}
		if to == "" {
			to = time.Now().Format("2006-01-02")
		}

		results, err := service.IncomeReport(from, to, groupBy)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("No income for the period.")
			return nil
		}

		var grandTotal int64
		for _, p := range results {
			grandTotal += p.Total
		}

		fmt.Printf("Income Report — %s to %s (by %s)\n", from[:7], to[:7], groupBy)
		fmt.Printf("Total: $%.2f\n\n", float64(grandTotal)/100)

		barWidth := 30
		// Find max source amount for bar scaling
		var maxAmt int64
		for _, p := range results {
			for _, s := range p.Sources {
				if s.Amount > maxAmt {
					maxAmt = s.Amount
				}
			}
		}

		for _, p := range results {
			fmt.Printf("  %s  $%.2f\n", p.Period, float64(p.Total)/100)
			for _, s := range p.Sources {
				bar := 0
				if maxAmt > 0 {
					bar = int(float64(s.Amount) / float64(maxAmt) * float64(barWidth))
				}
				if bar < 1 && s.Amount > 0 {
					bar = 1
				}
				pad := strings.Repeat("░", barWidth-bar)
				fmt.Printf("    %-20s %s%s  $%.2f (%4.1f%%)\n",
					s.CategoryName, strings.Repeat("█", bar), pad,
					float64(s.Amount)/100, s.Percent)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	reportSpendingCmd.Flags().String("month", "", "target month (YYYY-MM)")
	reportSpendingCmd.Flags().String("format", "table", "output format")
	reportNetworthCmd.Flags().String("format", "table", "output format")
	reportCashflowCmd.Flags().String("from", "", "start date (YYYY-MM-DD)")
	reportCashflowCmd.Flags().String("to", "", "end date (YYYY-MM-DD)")
	reportCashflowCmd.Flags().String("format", "table", "output format")

	reportIncomeCmd.Flags().String("from", "", "start date (YYYY-MM-DD)")
	reportIncomeCmd.Flags().String("to", "", "end date (YYYY-MM-DD)")
	reportIncomeCmd.Flags().String("group", "monthly", "grouping: monthly, quarterly, yearly")
	reportIncomeCmd.Flags().String("format", "table", "output format")

	reportCmd.AddCommand(reportSpendingCmd, reportNetworthCmd, reportCashflowCmd, reportIncomeCmd)
	rootCmd.AddCommand(reportCmd)
}
