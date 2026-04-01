package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
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
	nwHistory     []model.NetWorthSnapshot
	acctHistory   map[int64][]int64 // accountID -> balance history
	cashflow      []service.CashFlowReport
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
	nwHistory    []model.NetWorthSnapshot
	acctHistory  map[int64][]int64
	cashflow     []service.CashFlowReport
}

func (d DashboardModel) Init() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		nw, _ := d.svc.SnapshotNetWorth()
		bs, _ := d.svc.GetBudgetStatus(now.Year(), int(now.Month()))
		sp, _ := d.svc.SpendingReport(now.Year(), int(now.Month()))
		txs, _ := d.svc.ListTransactions(model.TxFilter{Limit: 10})
		accs, _ := d.svc.ListAccounts()
		nwh, _ := d.svc.GetNetWorthHistory(365)
		ah := make(map[int64][]int64)
		for _, a := range accs {
			hist, _ := d.svc.GetAccountBalanceHistory(a.ID, 30)
			if len(hist) > 0 {
				ah[a.ID] = hist
			}
		}
		from := now.AddDate(0, -6, 0).Format("2006-01-02")
		to := now.Format("2006-01-02")
		cf, _ := d.svc.CashFlowReport(from, to)
		return dashDataMsg{nw, bs, sp, txs, accs, nwh, ah, cf}
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
		d.nwHistory = msg.nwHistory
		d.acctHistory = msg.acctHistory
		d.cashflow = msg.cashflow
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

	// Single-column for narrow terminals
	if d.width < 80 {
		nwBox := d.renderNetWorth(d.width - 4)
		nwChart := d.renderNetWorthChart(d.width-4, 8)
		accBox := d.renderAccounts(d.width - 4)
		budgetBox := d.renderBudgetSummary(d.width - 4)
		spendBox := d.renderTopSpending(d.width - 4)
		txBox := d.renderRecentTxs()
		return lipgloss.JoinVertical(lipgloss.Left,
			nwBox, nwChart, accBox, "", budgetBox, "", spendBox, "", txBox)
	}

	// Left column: Net Worth + Chart + Accounts
	nwBox := d.renderNetWorth(colWidth)
	nwChart := d.renderNetWorthChart(colWidth, 8)
	accBox := d.renderAccounts(colWidth)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, nwBox, nwChart, accBox)

	// Right column: Budget Progress + Top Spending + Cash Flow
	budgetBox := d.renderBudgetSummary(colWidth)
	spendBox := d.renderTopSpending(colWidth)
	cfBox := d.renderCashFlowMini(colWidth)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, budgetBox, "", spendBox, "", cfBox)

	top := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)

	// Bottom: Recent Transactions
	txBox := d.renderRecentTxs()

	return lipgloss.JoinVertical(lipgloss.Left, top, "", txBox)
}

func (d DashboardModel) renderNetWorth(w int) string {
	s := headerStyle.Render("Net Worth")
	s += "\n"
	nw := d.netWorth
	style := greenStyle
	if nw.NetWorth < 0 {
		style = redStyle
	}
	s += style.Bold(true).Render(fmt.Sprintf("  %s", fmtMoney(nw.NetWorth)))
	s += "\n"
	s += fmt.Sprintf("  Assets: %s  |  Liabilities: %s",
		fmtMoney(nw.TotalAssets), fmtMoney(nw.TotalLiabilities))
	if nw.EquityValue > 0 {
		cashAssets := nw.TotalAssets - nw.EquityValue
		s += fmt.Sprintf("\n  Cash/Accounts: %s  |  Investments: %s",
			fmtMoney(cashAssets), fmtMoney(nw.EquityValue))
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderNetWorthChart(w, h int) string {
	if len(d.nwHistory) < 2 {
		return ""
	}

	chartW := w - 2
	if chartW < 20 {
		chartW = 20
	}

	tslc := timeserieslinechart.New(chartW, h,
		timeserieslinechart.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("10"))),
	)

	for _, snap := range d.nwHistory {
		t, err := time.Parse("2006-01-02", snap.Date)
		if err != nil {
			continue
		}
		tslc.Push(timeserieslinechart.TimePoint{Time: t, Value: float64(snap.NetWorth) / 100})
	}
	tslc.DrawBraille()
	return "  " + tslc.View()
}

func (d DashboardModel) renderAccounts(w int) string {
	s := headerStyle.Render("Accounts")
	if len(d.accounts) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No accounts yet")
	}
	for _, a := range d.accounts {
		line := fmt.Sprintf("\n  %-18s %10s", truncStr(a.Name, 18), fmtMoney(a.Balance))
		// Add sparkline if we have history
		if hist, ok := d.acctHistory[a.ID]; ok && len(hist) > 1 {
			sparkW := w - 40
			if sparkW < 8 {
				sparkW = 8
			}
			if sparkW > 20 {
				sparkW = 20
			}
			sl := sparkline.New(sparkW, 1)
			for _, b := range hist {
				sl.Push(float64(b) / 100)
			}
			sl.Draw()
			line += "  " + sl.View()
		}
		s += line
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderBudgetSummary(w int) string {
	now := time.Now()
	s := headerStyle.Render(fmt.Sprintf("Budgets — %s %d", now.Month().String(), now.Year()))

	// Filter: only show categories with a budget set or actual spending
	var active []model.BudgetStatus
	for _, b := range d.budgetStatus {
		if b.AmountLimit > 0 || b.Spent > 0 {
			active = append(active, b)
		}
	}
	if len(active) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No activity this month")
	}
	for _, b := range active {
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
		case b.AmountLimit == 0:
			barStyle = lipgloss.NewStyle().Foreground(muted)
		case b.Percent > 100:
			barStyle = redStyle
		case b.Percent > 80:
			barStyle = yellowStyle
		default:
			barStyle = greenStyle
		}
		bar := barStyle.Render(strings.Repeat("█", filled)) + strings.Repeat("░", empty)

		label := truncStr(b.CategoryName, 15)
		if b.AmountLimit > 0 {
			s += fmt.Sprintf("\n  %-15s %s %.0f%%", label, bar, b.Percent)
		} else {
			s += fmt.Sprintf("\n  %-15s %s (no limit)", label, fmtMoney(b.Spent))
		}
	}
	return boxStyle.Width(w).Render(s)
}

func (d DashboardModel) renderCashFlowMini(w int) string {
	s := headerStyle.Render("Cash Flow — Last 6 Months")
	if len(d.cashflow) == 0 {
		return boxStyle.Width(w).Render(s + "\n  No data")
	}

	var maxVal int64
	for _, cf := range d.cashflow {
		if cf.Income > maxVal {
			maxVal = cf.Income
		}
		if cf.Expenses > maxVal {
			maxVal = cf.Expenses
		}
	}

	barW := (w - 30) / 2
	if barW < 5 {
		barW = 5
	}

	for _, cf := range d.cashflow {
		incBar := 0
		expBar := 0
		if maxVal > 0 {
			incBar = int(float64(cf.Income) / float64(maxVal) * float64(barW))
			expBar = int(float64(cf.Expenses) / float64(maxVal) * float64(barW))
		}
		if incBar < 1 && cf.Income > 0 {
			incBar = 1
		}
		if expBar < 1 && cf.Expenses > 0 {
			expBar = 1
		}
		net := cf.Income - cf.Expenses
		netStyle := greenStyle
		if net < 0 {
			netStyle = redStyle
		}
		s += fmt.Sprintf("\n  %s %s%s %s",
			cf.Month[:7],
			greenStyle.Render(strings.Repeat("█", incBar))+redStyle.Render(strings.Repeat("▒", expBar)),
			strings.Repeat("░", barW*2-incBar-expBar),
			netStyle.Render(fmtMoneySign(net)),
		)
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
		s += fmt.Sprintf("\n  %-20s %s (%.0f%%)", sp.CategoryName, fmtMoney(sp.Amount), sp.Percent)
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
		if t.Type.IsCredit() {
			sign = "+"
			style = greenStyle
		}
		if t.Type.IsTransfer() {
			sign = "~"
			style = lipgloss.NewStyle().Foreground(muted)
		}
		s += fmt.Sprintf("  %s  %-25s  %s  %s\n",
			t.Date,
			truncStr(t.Payee, 25),
			style.Render(fmt.Sprintf("%s%s", sign, fmtMoney(t.Amount))),
			lipgloss.NewStyle().Foreground(muted).Render(t.CategoryName))
	}
	return s
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
