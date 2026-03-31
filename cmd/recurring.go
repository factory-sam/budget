package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/sam/budget/internal/model"
	"github.com/spf13/cobra"
)

var recurringCmd = &cobra.Command{
	Use:   "recurring",
	Short: "Manage recurring transactions",
}

var recurringAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a recurring transaction rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		payee, _ := cmd.Flags().GetString("payee")
		amount, _ := cmd.Flags().GetFloat64("amount")
		cat, _ := cmd.Flags().GetString("cat")
		account, _ := cmd.Flags().GetString("account")
		freq, _ := cmd.Flags().GetString("freq")
		start, _ := cmd.Flags().GetString("start")
		txType, _ := cmd.Flags().GetString("type")

		if account == "" {
			return fmt.Errorf("--account is required")
		}
		if start == "" {
			return fmt.Errorf("--start is required")
		}

		acc, err := service.GetAccountByName(account)
		if err != nil {
			return fmt.Errorf("account not found: %s", account)
		}

		var catID *int64
		if cat != "" {
			c, err := service.FindCategoryByName(cat)
			if err != nil {
				return fmt.Errorf("category not found: %s", cat)
			}
			catID = &c.ID
		}

		if txType == "" {
			txType = "expense"
		}

		cents := int64(math.Round(amount * 100))
		rule := model.RecurringRule{
			AccountID:  acc.ID,
			CategoryID: catID,
			Amount:     cents,
			Payee:      payee,
			Frequency:  model.Frequency(freq),
			StartDate:  start,
			NextDue:    start,
			Type:       model.TxType(txType),
		}

		created, err := service.CreateRecurringRule(rule)
		if err != nil {
			return err
		}
		fmt.Printf("Created recurring rule #%d: $%.2f %s for %s\n", created.ID, amount, freq, payee)
		return nil
	},
}

var recurringListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recurring rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		rules, err := service.ListRecurringRules()
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(rules)
		}
		if len(rules) == 0 {
			fmt.Println("No recurring rules.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tPAYEE\tAMOUNT\tFREQ\tNEXT DUE\tACCOUNT\tCATEGORY")
		for _, r := range rules {
			fmt.Fprintf(w, "%d\t%s\t$%.2f\t%s\t%s\t%s\t%s\n",
				r.ID, r.Payee, float64(r.Amount)/100, r.Frequency, r.NextDue, r.AccountName, r.CategoryName)
		}
		return w.Flush()
	},
}

var recurringDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a recurring rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ID: %s", args[0])
		}
		return service.DeleteRecurringRule(id)
	},
}

var recurringGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate pending transactions from recurring rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		count, err := service.GenerateRecurring()
		if err != nil {
			return err
		}
		fmt.Printf("Generated %d transactions from recurring rules\n", count)
		return nil
	},
}

func init() {
	recurringAddCmd.Flags().String("payee", "", "payee name")
	recurringAddCmd.Flags().Float64("amount", 0, "amount")
	recurringAddCmd.Flags().String("cat", "", "category name")
	recurringAddCmd.Flags().String("account", "", "account name")
	recurringAddCmd.Flags().String("freq", "monthly", "frequency (weekly, biweekly, monthly, yearly)")
	recurringAddCmd.Flags().String("start", "", "start date (YYYY-MM-DD)")
	recurringAddCmd.Flags().String("type", "expense", "transaction type (expense, income)")

	recurringListCmd.Flags().String("format", "table", "output format (table, json)")

	recurringCmd.AddCommand(recurringAddCmd, recurringListCmd, recurringDeleteCmd, recurringGenerateCmd)
	rootCmd.AddCommand(recurringCmd)
}
