package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type DashboardModel struct {
	svc           *service.Service
	width, height int
	netWorth      *model.NetWorthSnapshot
	budgetStatus  []model.BudgetStatus
	spending      []service.SpendingByCategory
	recentTxs     []model.Transaction
	accounts      []model.Account
}

func NewDashboardModel(svc *service.Service) DashboardModel {
	return DashboardModel{svc: svc}
}

type dashDataMsg struct {
	netWorth     *model.NetWorthSnapshot
	budgetStatus []model.BudgetStatus
	spending     []service.SpendingByCategory
	recentTxs    []model.Transaction
	accounts     []model.Account
}

func (d DashboardModel) Init() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		nw, _ := d.svc.SnapshotNetWorth()
		bs, _ := d.svc.GetBudgetStatus(now.Year(), int(now.Month()))
		sp, _ := d.svc.SpendingReport(now.Year(), int(now.Month()))
		txs, _ := d.svc.ListTransactions(model.TxFilter{Limit: 10})
		accs, _ := d.svc.ListAccounts()
		return dashDataMsg{nw, bs, sp, txs, accs}
	}
}

func (d DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case dashDataMsg:
		d.netWorth = msg.netWorth
		d.budgetStatus = msg.budgetStatus
		d.spending = msg.spending
		d.recentTxs = msg.recentTxs
		d.accounts = msg.accounts
	}
	return d, nil
}

func (d *DashboardModel) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d DashboardModel) View() string {
	if d.netWorth == nil {
		return "Loading dashboard..."
	}

	colWidth := d.width/2 - 4
	if colWidth < 30 {
		colWidth = 30
	}

	// Left column: Net Worth + Accounts
	nwBox := d.renderNetWorth(colWidth)
	accBox := d.renderAccounts(colWidth)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, nwBox, "", accBox)

	// Right column: Budget Progress + Top Spending
	budgetBox := d.renderBudgetSummary(colWidth)
	spendBox := d.renderTopSpending(colWidth)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, budgetBox, "", spendBox)

	top := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)

	// Bottom: Recent Transactions
	txBox := d.renderRecentTxs()

	return lipgloss.JoinVertical(lipgloss.Left, top, "", txBox)
}

func (d DashboardModel) renderNetWorth(w int) string {
	s := headerStyle.Render("Net Worth")
	s += "\n"
	nw := d.netWorth
	sign := ""
	style := greenStyle
	if nw.NetWorth < 0 {
		style = redStyle
		sign = "-"
	}
	s += style.Bold(true).Render(fmt.Sprintf("  %s$%.2f", sign, float64(abs(nw.NetWorth))/100))
	s += "\n"
	s += fmt.Sprintf("  Assets: $%.2f  |  Liabilities: $%.2f",
		float64(nw.TotalAssets)/100, float64(nw.TotalLiabilities)/100)
	if nw.EquityValue > 0 {
		s += fmt.Sprintf("\n  Equity: $%.2f", float64(nw.EquityValue)/100)
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderAccounts(w int) string {
	s := headerStyle.Render("Accounts")
	if len(d.accounts) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No accounts yet")
	}
	for _, a := range d.accounts {
		s += fmt.Sprintf("\n  %-20s $%.2f", a.Name, float64(a.Balance)/100)
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderBudgetSummary(w int) string {
	now := time.Now()
	s := headerStyle.Render(fmt.Sprintf("Budgets — %s %d", now.Month().String(), now.Year()))
	if len(d.budgetStatus) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No budgets set")
	}
	for _, b := range d.budgetStatus {
		barW := w - 30
		if barW < 10 {
			barW = 10
		}
		pct := b.Percent
		if pct > 100 {
			pct = 100
		}
		filled := int(pct / 100 * float64(barW))
		empty := barW - filled

		var barStyle lipgloss.Style
		switch {
		case b.Percent > 100:
			barStyle = redStyle
		case b.Percent > 80:
			barStyle = yellowStyle
		default:
			barStyle = greenStyle
		}
		bar := barStyle.Render(strings.Repeat("█", filled)) + strings.Repeat("░", empty)
		s += fmt.Sprintf("\n  %-15s %s %.0f%%", truncStr(b.CategoryName, 15), bar, b.Percent)
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderTopSpending(w int) string {
	now := time.Now()
	s := headerStyle.Render(fmt.Sprintf("Top Spending — %s", now.Month().String()))
	if len(d.spending) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No spending yet")
	}
	limit := 5
	if len(d.spending) < limit {
		limit = len(d.spending)
	}
	for _, sp := range d.spending[:limit] {
		s += fmt.Sprintf("\n  %-20s $%.2f (%.0f%%)", sp.CategoryName, float64(sp.Amount)/100, sp.Percent)
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderRecentTxs() string {
	s := headerStyle.Render("Recent Transactions")
	if len(d.recentTxs) == 0 {
		return s + "\n  No transactions yet"
	}
	s += "\n"
	for _, t := range d.recentTxs {
		sign := "-"
		style := redStyle
		if t.Type == model.TxIncome {
			sign = "+"
			style = greenStyle
		}
		s += fmt.Sprintf("  %s  %-25s  %s  %s\n",
			t.Date,
			truncStr(t.Payee, 25),
			style.Render(fmt.Sprintf("%s$%.2f", sign, float64(t.Amount)/100)),
			lipgloss.NewStyle().Foreground(muted).Render(t.CategoryName))
	}
	return s
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
