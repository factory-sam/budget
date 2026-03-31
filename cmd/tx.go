package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/sam/budget/internal/model"
	"github.com/spf13/cobra"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Manage transactions",
}

var txAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a transaction",
	RunE: func(cmd *cobra.Command, args []string) error {
		amount, _ := cmd.Flags().GetFloat64("amount")
		payee, _ := cmd.Flags().GetString("payee")
		cat, _ := cmd.Flags().GetString("cat")
		account, _ := cmd.Flags().GetString("account")
		date, _ := cmd.Flags().GetString("date")
		note, _ := cmd.Flags().GetString("note")
		txType, _ := cmd.Flags().GetString("type")

		if amount == 0 {
			return fmt.Errorf("--amount is required")
		}
		if account == "" {
			return fmt.Errorf("--account is required")
		}

		acc, err := service.GetAccountByName(account)
		if err != nil {
			id, err2 := strconv.ParseInt(account, 10, 64)
			if err2 != nil {
				return fmt.Errorf("account not found: %s", account)
			}
			acc, err = service.GetAccount(id)
			if err != nil {
				return fmt.Errorf("account not found: %s", account)
			}
		}

		var categoryID *int64
		if cat != "" {
			c, err := service.FindCategoryByName(cat)
			if err != nil {
				return fmt.Errorf("category not found: %s", cat)
			}
			categoryID = &c.ID
		} else {
			categoryID = service.AutoCategorize(payee)
		}

		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		if txType == "" {
			txType = "expense"
		}

		cents := int64(math.Round(amount * 100))
		tx := model.Transaction{
			AccountID:  acc.ID,
			CategoryID: categoryID,
			Amount:     cents,
			Date:       date,
			Payee:      payee,
			Note:       note,
			Type:       model.TxType(txType),
		}

		created, err := service.CreateTransaction(tx)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(created)
		}
		fmt.Printf("Added transaction #%d: $%.2f to %s (%s)\n", created.ID, amount, payee, date)
		return nil
	},
}

var txListCmd = &cobra.Command{
	Use:   "list",
	Short: "List transactions",
	RunE: func(cmd *cobra.Command, args []string) error {
		month, _ := cmd.Flags().GetString("month")
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		cat, _ := cmd.Flags().GetString("cat")
		payee, _ := cmd.Flags().GetString("payee")
		search, _ := cmd.Flags().GetString("search")
		limit, _ := cmd.Flags().GetInt("limit")

		filter := model.TxFilter{
			Month:  month,
			From:   from,
			To:     to,
			Payee:  payee,
			Search: search,
			Limit:  limit,
		}

		if cat != "" {
			c, err := service.FindCategoryByName(cat)
			if err != nil {
				return fmt.Errorf("category not found: %s", cat)
			}
			filter.CategoryID = &c.ID
		}

		txs, err := service.ListTransactions(filter)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(txs)
		}

		if len(txs) == 0 {
			fmt.Println("No transactions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tDATE\tPAYEE\tAMOUNT\tTYPE\tCATEGORY\tACCOUNT")
		for _, t := range txs {
			sign := "-"
			if t.Type == model.TxIncome {
				sign = "+"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s$%.2f\t%s\t%s\t%s\n",
				t.ID, t.Date, truncate(t.Payee, 30), sign, float64(t.Amount)/100,
				t.Type, t.CategoryName, t.AccountName)
		}
		return w.Flush()
	},
}

var txDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a transaction",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ID: %s", args[0])
		}
		if err := service.DeleteTransaction(id); err != nil {
			return err
		}
		fmt.Printf("Deleted transaction #%d\n", id)
		return nil
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	txAddCmd.Flags().Float64("amount", 0, "transaction amount")
	txAddCmd.Flags().String("payee", "", "payee/merchant name")
	txAddCmd.Flags().String("cat", "", "category name")
	txAddCmd.Flags().String("account", "", "account name or ID")
	txAddCmd.Flags().String("date", "", "date (YYYY-MM-DD, default today)")
	txAddCmd.Flags().String("note", "", "optional note")
	txAddCmd.Flags().String("type", "expense", "transaction type (expense, income, transfer)")
	txAddCmd.Flags().String("format", "table", "output format (table, json)")

	txListCmd.Flags().String("month", "", "filter by month (YYYY-MM)")
	txListCmd.Flags().String("from", "", "filter from date (YYYY-MM-DD)")
	txListCmd.Flags().String("to", "", "filter to date (YYYY-MM-DD)")
	txListCmd.Flags().String("cat", "", "filter by category name")
	txListCmd.Flags().String("payee", "", "filter by payee")
	txListCmd.Flags().String("search", "", "search payee and notes")
	txListCmd.Flags().Int("limit", 50, "max results")
	txListCmd.Flags().String("format", "table", "output format (table, json)")

	txCmd.AddCommand(txAddCmd, txListCmd, txDeleteCmd)
	rootCmd.AddCommand(txCmd)
}
