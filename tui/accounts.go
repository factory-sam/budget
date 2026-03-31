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

type AccountsModel struct {
	svc           *service.Service
	width, height int
	accounts      []model.Account
	cursor        int
	form          FormModel
	confirmDelete bool
	statusMsg     string
}

func (a AccountsModel) InputActive() bool { return a.form.Active() || a.confirmDelete }

func NewAccountsModel(svc *service.Service) AccountsModel {
	return AccountsModel{svc: svc}
}

type accountDataMsg struct {
	accounts []model.Account
}

func (a AccountsModel) Init() tea.Cmd {
	return func() tea.Msg {
		accs, _ := a.svc.ListAccounts()
		return accountDataMsg{accs}
	}
}

func (a AccountsModel) Update(msg tea.Msg) (AccountsModel, tea.Cmd) {
	if a.form.Active() {
		var cmd tea.Cmd
		a.form, cmd = a.form.Update(msg)
		return a, cmd
	}

	switch msg := msg.(type) {
	case accountDataMsg:
		a.accounts = msg.accounts
		a.cursor = 0
	case tea.KeyMsg:
		a.statusMsg = ""
		if a.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				a.confirmDelete = false
				if a.cursor < len(a.accounts) {
					if err := a.svc.DeleteAccount(a.accounts[a.cursor].ID); err != nil {
						a.statusMsg = fmt.Sprintf("Error deleting account: %v", err)
						return a, nil
					}
					return a, a.Init()
				}
			default:
				a.confirmDelete = false
			}
			return a, nil
		}

		switch msg.String() {
		case "j", keyDown:
			if a.cursor < len(a.accounts)-1 {
				a.cursor++
			}
		case "k", "up":
			if a.cursor > 0 {
				a.cursor--
			}
		case "g":
			a.cursor = 0
		case "G":
			if len(a.accounts) > 0 {
				a.cursor = len(a.accounts) - 1
			}
		case "a":
			a.form = a.newAddForm()
		case "d":
			if len(a.accounts) > 0 && a.cursor < len(a.accounts) {
				a.confirmDelete = true
			}
		}
	}
	return a, nil
}

func (a *AccountsModel) newAddForm() FormModel {
	svc := a.svc
	return NewForm("Add Account", []FormField{
		{Label: "Name", Placeholder: "e.g. Chase Checking"},
		{Label: "Type", Value: "checking", Options: []string{
			"checking", "savings", "credit_card", "investment", "loan", "cash",
			"brokerage", "401k", "money_market", "managed",
		}},
		{Label: "Balance", Placeholder: "0.00"},
		{Label: "Tracking", Value: "holdings", Options: []string{"holdings", "balance"}},
	}, func(fields []FormField) (tea.Cmd, string) {
		name := strings.TrimSpace(fields[0].Value)
		typ := fields[1].Value
		balStr := strings.TrimSpace(fields[2].Value)
		trackingMode := fields[3].Value
		if name == "" {
			return nil, "Name is required"
		}
		bal := 0.0
		if balStr != "" {
			var err error
			bal, err = strconv.ParseFloat(balStr, 64)
			if err != nil {
				return nil, "Invalid balance — enter a number like 5000"
			}
		}
		cents := int64(math.Round(bal * 100))
		_, err := svc.CreateAccountFull(name, model.AccountType(typ), cents, trackingMode)
		if err != nil {
			return nil, fmt.Sprintf("Error: %v", err)
		}
		return func() tea.Msg {
			accs, _ := svc.ListAccounts()
			return accountDataMsg{accs}
		}, ""
	})
}

func (a *AccountsModel) SetSize(w, h int) {
	a.width = w
	a.height = h
}

func (a AccountsModel) View() string {
	if a.form.Active() {
		return a.form.View()
	}

	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Accounts") + "\n\n")

	if len(a.accounts) == 0 {
		sb.WriteString("  No accounts yet. Press 'a' to add one.\n")
		return sb.String()
	}

	var assets, liabilities []model.Account
	for _, acc := range a.accounts {
		if model.IsLiability(acc.Type) {
			liabilities = append(liabilities, acc)
		} else {
			assets = append(assets, acc)
		}
	}

	var totalAssets, totalLiabilities int64
	idx := 0

	if len(assets) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(special).Render("  Assets") + "\n")
		for _, acc := range assets {
			totalAssets += acc.Balance
			line := fmt.Sprintf("    %-25s  %-12s  %10s", acc.Name, acc.Type, fmtMoney(acc.Balance))
			if idx == a.cursor {
				line = selectedRowStyle.Render(line)
			}
			sb.WriteString(line + "\n")
			idx++
		}
		sb.WriteString(fmt.Sprintf("    %-25s  %-12s  %10s\n", "", "Total", fmtMoney(totalAssets)))
	}

	if len(liabilities) > 0 {
		sb.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(danger).Render("  Liabilities") + "\n")
		for _, acc := range liabilities {
			totalLiabilities += acc.Balance
			line := fmt.Sprintf("    %-25s  %-12s  %10s", acc.Name, acc.Type, fmtMoney(acc.Balance))
			if idx == a.cursor {
				line = selectedRowStyle.Render(line)
			}
			sb.WriteString(line + "\n")
			idx++
		}
		sb.WriteString(fmt.Sprintf("    %-25s  %-12s  %10s\n", "", "Total", fmtMoney(totalLiabilities)))
	}

	nw := totalAssets - totalLiabilities
	sb.WriteString(fmt.Sprintf("\n  Net Worth: %s", fmtMoney(nw)))

	if a.confirmDelete && a.cursor < len(a.accounts) {
		acc := a.accounts[a.cursor]
		sb.WriteString(fmt.Sprintf("\n\n  %s",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).
				Render(fmt.Sprintf("Delete account %q (%s)? y/N", acc.Name, fmtMoney(acc.Balance)))))
	} else if a.statusMsg != "" {
		sb.WriteString("\n\n  " + lipgloss.NewStyle().Foreground(danger).Render(a.statusMsg))
	} else {
		sb.WriteString(fmt.Sprintf("\n\n  %d accounts [%d/%d]", len(a.accounts), a.cursor+1, len(a.accounts)))
	}
	return sb.String()
}
