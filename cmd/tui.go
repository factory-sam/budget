package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sam/budget/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI",
	RunE:  runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	app := tui.NewApp(service)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
