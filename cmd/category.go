package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var categoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Manage categories",
}

var categoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all categories",
	RunE: func(cmd *cobra.Command, args []string) error {
		cats, err := service.ListCategories()
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			return json.NewEncoder(os.Stdout).Encode(cats)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tGROUP\tNAME")
		curGroup := ""
		for _, c := range cats {
			if c.GroupName != curGroup {
				curGroup = c.GroupName
			}
			fmt.Fprintf(w, "%d\t%s\t%s\n", c.ID, c.GroupName, c.Name)
		}
		return w.Flush()
	},
}

var categoryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a custom category",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		group, _ := cmd.Flags().GetString("group")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if group == "" {
			group = "Personal"
		}
		cat, err := service.CreateCategory(group, name)
		if err != nil {
			return err
		}
		fmt.Printf("Created category: %s (ID: %d, group: %s)\n", cat.Name, cat.ID, cat.GroupName)
		return nil
	},
}

var categoryDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a category",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ID: %s", args[0])
		}
		return service.DeleteCategory(id)
	},
}

func init() {
	categoryListCmd.Flags().String("format", "table", "output format (table, json)")
	categoryAddCmd.Flags().String("name", "", "category name")
	categoryAddCmd.Flags().String("group", "", "category group name")

	categoryCmd.AddCommand(categoryListCmd, categoryAddCmd, categoryDeleteCmd)
	rootCmd.AddCommand(categoryCmd)
}
