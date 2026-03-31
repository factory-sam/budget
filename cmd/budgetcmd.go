package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set [category] [amount]",
	Short: "Set a monthly budget for a category",
	Long:  "Set a monthly budget limit. Example: budget set Groceries 600",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		catName := args[0]
		amount, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("invalid amount: %s", args[1])
		}
		month, _ := cmd.Flags().GetString("month")

		cat, err := service.FindCategoryByName(catName)
		if err != nil {
			return fmt.Errorf("category not found: %s", catName)
		}

		y, m := parseMonth(month)
		cents := int64(math.Round(amount * 100))
		if err := service.SetBudget(cat.ID, y, m, cents); err != nil {
			return err
		}
		fmt.Printf("Set budget for %s: $%.2f/month (%d-%02d)\n", cat.Name, amount, y, m)
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show budget status for current month",
	RunE: func(cmd *cobra.Command, args []string) error {
		month, _ := cmd.Flags().GetString("month")
		y, m := parseMonth(month)

		statuses, err := service.GetBudgetStatus(y, m)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(statuses)
		}

		if len(statuses) == 0 {
			fmt.Printf("No budgets set for %d-%02d. Use: budget set <category> <amount>\n", y, m)
			return nil
		}

		fmt.Printf("Budget Status — %d-%02d\n\n", y, m)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CATEGORY\tBUDGET\tSPENT\tREMAINING\tUSED")
		for _, s := range statuses {
			indicator := "✓"
			if s.Percent > 90 {
				indicator = "!"
			}
			if s.Percent > 100 {
				indicator = "X"
			}
			fmt.Fprintf(w, "%s\t$%.2f\t$%.2f\t$%.2f\t%.0f%% %s\n",
				s.CategoryName,
				float64(s.AmountLimit)/100,
				float64(s.Spent)/100,
				float64(s.Remaining)/100,
				s.Percent,
				indicator)
		}
		return w.Flush()
	},
}

func parseMonth(s string) (int, int) {
	if s != "" {
		t, err := time.Parse("2006-01", s)
		if err == nil {
			return t.Year(), int(t.Month())
		}
	}
	now := time.Now()
	return now.Year(), int(now.Month())
}

func init() {
	setCmd.Flags().String("month", "", "target month (YYYY-MM, default current)")
	statusCmd.Flags().String("month", "", "target month (YYYY-MM, default current)")
	statusCmd.Flags().String("format", "table", "output format (table, json)")

	rootCmd.AddCommand(setCmd, statusCmd)
}
