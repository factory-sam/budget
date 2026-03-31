package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type RecurringModel struct {
	svc           *service.Service
	width, height int
	rules         []model.RecurringRule
	cursor        int
}

func NewRecurringModel(svc *service.Service) RecurringModel {
	return RecurringModel{svc: svc}
}

type recurringDataMsg struct {
	rules []model.RecurringRule
}

func (r RecurringModel) Init() tea.Cmd {
	return func() tea.Msg {
		rules, _ := r.svc.ListRecurringRules()
		return recurringDataMsg{rules}
	}
}

func (r RecurringModel) Update(msg tea.Msg) (RecurringModel, tea.Cmd) {
	switch msg := msg.(type) {
	case recurringDataMsg:
		r.rules = msg.rules
		r.cursor = 0
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if r.cursor < len(r.rules)-1 {
				r.cursor++
			}
		case "k", "up":
			if r.cursor > 0 {
				r.cursor--
			}
		case "d":
			if len(r.rules) > 0 && r.cursor < len(r.rules) {
				r.svc.DeleteRecurringRule(r.rules[r.cursor].ID)
				return r, r.Init()
			}
		}
	}
	return r, nil
}

func (r *RecurringModel) SetSize(w, h int) {
	r.width = w
	r.height = h
}

func (r RecurringModel) View() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Recurring Transactions") + "\n\n")

	if len(r.rules) == 0 {
		sb.WriteString("  No recurring rules. Add one with:\n")
		sb.WriteString("  budget recurring add --payee \"Netflix\" --amount 15.99 --freq monthly --account checking --start 2026-01-01\n")
		return sb.String()
	}

	hdr := fmt.Sprintf("  %-20s  %10s  %-10s  %-12s  %-15s  %-15s",
		"PAYEE", "AMOUNT", "FREQ", "NEXT DUE", "ACCOUNT", "CATEGORY")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	for i, rule := range r.rules {
		line := fmt.Sprintf("  %-20s  $%9.2f  %-10s  %-12s  %-15s  %-15s",
			truncStr(rule.Payee, 20),
			float64(rule.Amount)/100,
			rule.Frequency,
			rule.NextDue,
			truncStr(rule.AccountName, 15),
			truncStr(rule.CategoryName, 15))

		if i == r.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString(fmt.Sprintf("\n  %d rules | j/k:navigate  d:delete", len(r.rules)))
	return sb.String()
}
