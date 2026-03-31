package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func fmtMoney(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	dollars := cents / 100
	remainder := cents % 100

	s := fmt.Sprintf("%d", dollars)
	// Insert commas
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ",")
	}

	if negative {
		return fmt.Sprintf("-$%s.%02d", s, remainder)
	}
	return fmt.Sprintf("$%s.%02d", s, remainder)
}

func fmtMoneySign(cents int64) string {
	if cents >= 0 {
		return "+" + fmtMoney(cents)
	}
	return fmtMoney(cents)
}

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	warning   = lipgloss.AdaptiveColor{Light: "#FF6600", Dark: "#FF9933"}
	danger    = lipgloss.AdaptiveColor{Light: "#FF0000", Dark: "#FF4444"}
	muted     = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFF")).
			Background(highlight).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(muted).
				Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			MarginBottom(1)

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#EEE", Dark: "#333"})

	greenStyle  = lipgloss.NewStyle().Foreground(special)
	yellowStyle = lipgloss.NewStyle().Foreground(warning)
	redStyle    = lipgloss.NewStyle().Foreground(danger)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(1, 2)
)
