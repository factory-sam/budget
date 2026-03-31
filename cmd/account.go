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

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage accounts",
}

var accountAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new account",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		typ, _ := cmd.Flags().GetString("type")
		balance, _ := cmd.Flags().GetFloat64("balance")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		cents := int64(math.Round(balance * 100))
		acc, err := service.CreateAccount(name, model.AccountType(typ), cents)
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(acc)
		}
		fmt.Printf("Created account: %s (ID: %d, balance: $%.2f)\n", acc.Name, acc.ID, float64(acc.Balance)/100)
		return nil
	},
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all accounts",
	RunE: func(cmd *cobra.Command, args []string) error {
		accs, err := service.ListAccounts()
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(accs)
		}
		if len(accs) == 0 {
			fmt.Println("No accounts. Add one with: budget account add --name \"My Checking\" --type checking --balance 1000")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tTYPE\tBALANCE")
		for _, a := range accs {
			fmt.Fprintf(w, "%d\t%s\t%s\t$%.2f\n", a.ID, a.Name, a.Type, float64(a.Balance)/100)
		}
		return w.Flush()
	},
}

var accountDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete an account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ID: %s", args[0])
		}
		return service.DeleteAccount(id)
	},
}

func init() {
	accountAddCmd.Flags().String("name", "", "account name")
	accountAddCmd.Flags().String("type", "checking", "account type (checking, savings, credit_card, investment, loan, cash)")
	accountAddCmd.Flags().Float64("balance", 0, "starting balance")
	accountAddCmd.Flags().String("format", "table", "output format (table, json)")

	accountListCmd.Flags().String("format", "table", "output format (table, json)")

	accountCmd.AddCommand(accountAddCmd, accountListCmd, accountDeleteCmd)
	rootCmd.AddCommand(accountCmd)
}
