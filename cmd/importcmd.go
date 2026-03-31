package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sam/budget/internal/importer"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import transactions from CSV, OFX, or QFX file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		accountName, _ := cmd.Flags().GetString("account")

		if accountName == "" {
			return fmt.Errorf("--account is required")
		}

		acc, err := service.GetAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account not found: %s", accountName)
		}

		ext := strings.ToLower(filepath.Ext(path))
		var count int

		switch ext {
		case ".csv":
			imp := importer.NewCSV(service)
			count, err = imp.Import(path, acc.ID)
		case ".ofx", ".qfx":
			imp := importer.NewOFX(service)
			count, err = imp.Import(path, acc.ID)
		default:
			return fmt.Errorf("unsupported file format: %s (supported: .csv, .ofx, .qfx)", ext)
		}

		if err != nil {
			return err
		}
		fmt.Printf("Imported %d transactions into %s\n", count, acc.Name)
		return nil
	},
}

func init() {
	importCmd.Flags().String("account", "", "target account name")
	rootCmd.AddCommand(importCmd)
}
