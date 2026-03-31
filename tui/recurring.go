package tui

import (
	"fmt"
	"math"
	"strconv"
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
	form          FormModel
	confirmDelete bool
	statusMsg     string
}

func (r RecurringModel) InputActive() bool { return r.form.Active() || r.confirmDelete }

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
	if r.form.Active() {
		var cmd tea.Cmd
		r.form, cmd = r.form.Update(msg)
		return r, cmd
	}

	switch msg := msg.(type) {
	case recurringDataMsg:
		r.rules = msg.rules
		r.cursor = 0
	case tea.KeyMsg:
		r.statusMsg = ""
		if r.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				r.confirmDelete = false
				if r.cursor < len(r.rules) {
					if err := r.svc.DeleteRecurringRule(r.rules[r.cursor].ID); err != nil {
						r.statusMsg = fmt.Sprintf("Error deleting rule: %v", err)
						return r, nil
					}
					return r, r.Init()
				}
			default:
				r.confirmDelete = false
			}
			return r, nil
		}

		switch msg.String() {
		case "j", keyDown:
			if r.cursor < len(r.rules)-1 {
				r.cursor++
			}
		case "k", "up":
			if r.cursor > 0 {
				r.cursor--
			}
		case "g":
			r.cursor = 0
		case "G":
			if len(r.rules) > 0 {
				r.cursor = len(r.rules) - 1
			}
		case "a":
			r.form = r.newAddForm()
		case "d":
			if len(r.rules) > 0 && r.cursor < len(r.rules) {
				r.confirmDelete = true
			}
		}
	}
	return r, nil
}

func (r *RecurringModel) newAddForm() FormModel {
	svc := r.svc
	return NewForm("Add Recurring Rule", []FormField{
		{Label: "Payee", Placeholder: "e.g. Netflix"},
		{Label: "Amount", Placeholder: "0.00"},
		{Label: "Category", Placeholder: "e.g. Streaming"},
		{Label: "Account", Placeholder: "e.g. Checking"},
		{Label: "Frequency", Value: "monthly", Options: []string{"weekly", "biweekly", "monthly", "yearly"}},
		{Label: "Type", Value: "expense", Options: []string{"expense", "income"}},
		{Label: "Start Date", Placeholder: "YYYY-MM-DD"},
	}, func(fields []FormField) (tea.Cmd, string) {
		payee := strings.TrimSpace(fields[0].Value)
		amtStr := strings.TrimSpace(fields[1].Value)
		catName := strings.TrimSpace(fields[2].Value)
		accName := strings.TrimSpace(fields[3].Value)
		freq := fields[4].Value
		txType := fields[5].Value
		startDate := strings.TrimSpace(fields[6].Value)

		if payee == "" {
			return nil, "Payee is required"
		}
		if amtStr == "" {
			return nil, "Amount is required"
		}
		if accName == "" {
			return nil, "Account is required"
		}
		if startDate == "" {
			return nil, "Start date is required"
		}

		acc, err := svc.GetAccountByName(accName)
		if err != nil {
			return nil, fmt.Sprintf("Account %q not found", accName)
		}

		amount, err := strconv.ParseFloat(amtStr, 64)
		if err != nil || amount <= 0 {
			return nil, "Invalid amount — enter a number like 15.99"
		}
		cents := int64(math.Round(amount * 100))

		var catID *int64
		if catName != "" {
			c, err := svc.FindCategoryByName(catName)
			if err != nil {
				return nil, fmt.Sprintf("Category %q not found", catName)
			}
			catID = &c.ID
		}

		rule := model.RecurringRule{
			AccountID:  acc.ID,
			CategoryID: catID,
			Amount:     cents,
			Payee:      payee,
			Frequency:  model.Frequency(freq),
			StartDate:  startDate,
			NextDue:    startDate,
			Type:       model.TxType(txType),
		}
		if _, err := svc.CreateRecurringRule(rule); err != nil {
			return nil, fmt.Sprintf("Error creating rule: %v", err)
		}

		return func() tea.Msg {
			rules, _ := svc.ListRecurringRules()
			return recurringDataMsg{rules}
		}, ""
	})
}

func (r *RecurringModel) SetSize(w, h int) {
	r.width = w
	r.height = h
}

func (r RecurringModel) View() string {
	if r.form.Active() {
		return r.form.View()
	}

	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Recurring Transactions") + "\n\n")

	if len(r.rules) == 0 {
		sb.WriteString("  No recurring rules. Press 'a' to add one.\n")
		return sb.String()
	}

	hdr := fmt.Sprintf("  %-20s  %10s  %-10s  %-12s  %-15s  %-15s",
		"PAYEE", "AMOUNT", "FREQ", "NEXT DUE", "ACCOUNT", "CATEGORY")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	for i, rule := range r.rules {
		line := fmt.Sprintf("  %-20s  %10s  %-10s  %-12s  %-15s  %-15s",
			truncStr(rule.Payee, 20),
			fmtMoney(rule.Amount),
			rule.Frequency,
			rule.NextDue,
			truncStr(rule.AccountName, 15),
			truncStr(rule.CategoryName, 15))

		if i == r.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	if r.confirmDelete && r.cursor < len(r.rules) {
		rule := r.rules[r.cursor]
		sb.WriteString(fmt.Sprintf("\n  %s",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).
				Render(fmt.Sprintf("Delete recurring rule %q (%s)? y/N", rule.Payee, fmtMoney(rule.Amount)))))
	} else if r.statusMsg != "" {
		sb.WriteString("\n  " + lipgloss.NewStyle().Foreground(danger).Render(r.statusMsg))
	} else {
		sb.WriteString(fmt.Sprintf("\n  %d rules [%d/%d]", len(r.rules), r.cursor+1, len(r.rules)))
	}
	return sb.String()
}
