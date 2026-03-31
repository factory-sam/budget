package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sam/budget/internal/model"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export transactions to CSV or JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		format, _ := cmd.Flags().GetString("format")

		filter := model.TxFilter{From: from, To: to}
		txs, err := service.ListTransactions(filter)
		if err != nil {
			return err
		}

		switch format {
		case "json":
			return json.NewEncoder(os.Stdout).Encode(txs)
		case "csv":
			w := csv.NewWriter(os.Stdout)
			w.Write([]string{"ID", "Date", "Payee", "Amount", "Type", "Category", "Account", "Note"})
			for _, t := range txs {
				w.Write([]string{
					fmt.Sprintf("%d", t.ID),
					t.Date,
					t.Payee,
					fmt.Sprintf("%.2f", float64(t.Amount)/100),
					string(t.Type),
					t.CategoryName,
					t.AccountName,
					t.Note,
				})
			}
			w.Flush()
			return w.Error()
		default:
			return fmt.Errorf("unsupported format: %s (use csv or json)", format)
		}
	},
}

func init() {
	exportCmd.Flags().String("from", "", "start date (YYYY-MM-DD)")
	exportCmd.Flags().String("to", "", "end date (YYYY-MM-DD)")
	exportCmd.Flags().String("format", "csv", "output format (csv, json)")
	rootCmd.AddCommand(exportCmd)
}
