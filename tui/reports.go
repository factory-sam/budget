package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type ReportsModel struct {
	svc           *service.Service
	width, height int
	spending      []service.SpendingByCategory
	cashflow      []service.CashFlowReport
	income        []service.IncomePeriod
	nwHistory     []model.NetWorthSnapshot
	incomeGroup   string // monthly, quarterly, yearly
	activeReport  int    // 0=spending, 1=cashflow, 2=income, 3=net worth
}

func NewReportsModel(svc *service.Service) ReportsModel {
	return ReportsModel{svc: svc, incomeGroup: "monthly"}
}

type reportsDataMsg struct {
	spending  []service.SpendingByCategory
	cashflow  []service.CashFlowReport
	income    []service.IncomePeriod
	nwHistory []model.NetWorthSnapshot
}

func (r ReportsModel) Init() tea.Cmd {
	group := r.incomeGroup
	return func() tea.Msg {
		now := time.Now()
		sp, _ := r.svc.SpendingReport(now.Year(), int(now.Month()))
		from := now.AddDate(0, -6, 0).Format("2006-01-02")
		to := now.Format("2006-01-02")
		cf, _ := r.svc.CashFlowReport(from, to)
		incFrom := now.AddDate(-1, 0, 0).Format("2006-01-02")
		inc, _ := r.svc.IncomeReport(incFrom, to, group)
		nwh, _ := r.svc.GetNetWorthHistory(365)
		return reportsDataMsg{sp, cf, inc, nwh}
	}
}

func (r ReportsModel) Update(msg tea.Msg) (ReportsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case reportsDataMsg:
		r.spending = msg.spending
		r.cashflow = msg.cashflow
		r.income = msg.income
		r.nwHistory = msg.nwHistory
	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			if r.activeReport > 0 {
				r.activeReport--
			}
		case "l", "right":
			if r.activeReport < 3 {
				r.activeReport++
			}
		case "g":
			// cycle income grouping when on income tab
			if r.activeReport == 2 {
				switch r.incomeGroup {
				case "monthly":
					r.incomeGroup = "quarterly"
				case "quarterly":
					r.incomeGroup = "yearly"
				default:
					r.incomeGroup = "monthly"
				}
				return r, r.Init()
			}
		}
	}
	return r, nil
}

func (r *ReportsModel) SetSize(w, h int) {
	r.width = w
	r.height = h
}

func (r ReportsModel) View() string {
	var sb strings.Builder

	// Report tabs
	tabs := []string{"Spending", "Cash Flow", "Income", "Net Worth"}
	for i, t := range tabs {
		if i == r.activeReport {
			sb.WriteString(activeTabStyle.Render(t) + " ")
		} else {
			sb.WriteString(inactiveTabStyle.Render(t) + " ")
		}
	}
	sb.WriteString("\n\n")

	switch r.activeReport {
	case 0:
		sb.WriteString(r.viewSpending())
	case 1:
		sb.WriteString(r.viewCashFlow())
	case 2:
		sb.WriteString(r.viewIncome())
	case 3:
		sb.WriteString(r.viewNetWorthHistory())
	}

	help := "h/l:switch reports"
	if r.activeReport == 2 {
		help += "  g:cycle grouping (" + r.incomeGroup + ")"
	}
	sb.WriteString("\n  " + help)
	return sb.String()
}

func (r ReportsModel) viewSpending() string {
	now := time.Now()
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Spending by Category — %s %d", now.Month().String(), now.Year())) + "\n\n")

	if len(r.spending) == 0 {
		sb.WriteString("  No spending data.\n")
		return sb.String()
	}

	barWidth := r.width - 45
	if barWidth < 15 {
		barWidth = 15
	}
	if barWidth > 50 {
		barWidth = 50
	}

	for _, sp := range r.spending {
		filled := int(sp.Percent / 100 * float64(barWidth))
		if filled < 1 && sp.Amount > 0 {
			filled = 1
		}
		empty := barWidth - filled
		bar := lipgloss.NewStyle().Foreground(highlight).Render(strings.Repeat("█", filled)) + strings.Repeat("░", empty)

		sb.WriteString(fmt.Sprintf("  %-18s  %10s  %5.1f%%  %s\n",
			truncStr(sp.CategoryName, 18), fmtMoney(sp.Amount), sp.Percent, bar))
	}

	var total int64
	for _, sp := range r.spending {
		total += sp.Amount
	}
	sb.WriteString(fmt.Sprintf("\n  Total: %s\n", fmtMoney(total)))
	return sb.String()
}

func (r ReportsModel) viewIncome() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Income by Source — Last 12 Months (%s)", r.incomeGroup)) + "\n\n")

	if len(r.income) == 0 {
		sb.WriteString("  No income data.\n")
		return sb.String()
	}

	// Grand total
	var grandTotal int64
	for _, p := range r.income {
		grandTotal += p.Total
	}
	sb.WriteString(fmt.Sprintf("  Total: %s\n\n",
		greenStyle.Render(fmtMoney(grandTotal))))

	barWidth := r.width - 55
	if barWidth < 15 {
		barWidth = 15
	}
	if barWidth > 35 {
		barWidth = 35
	}

	// Find max source for scaling
	var maxAmt int64
	for _, p := range r.income {
		for _, s := range p.Sources {
			if s.Amount > maxAmt {
				maxAmt = s.Amount
			}
		}
	}

	for _, p := range r.income {
		sb.WriteString(fmt.Sprintf("  %s  %s\n",
			lipgloss.NewStyle().Bold(true).Render(p.Period),
			greenStyle.Render(fmtMoney(p.Total))))

		for _, s := range p.Sources {
			bar := 0
			if maxAmt > 0 {
				bar = int(float64(s.Amount) / float64(maxAmt) * float64(barWidth))
			}
			if bar < 1 && s.Amount > 0 {
				bar = 1
			}
			pad := strings.Repeat("░", barWidth-bar)
			sb.WriteString(fmt.Sprintf("    %-18s %s%s  %s (%4.1f%%)\n",
				truncStr(s.CategoryName, 18),
				greenStyle.Render(strings.Repeat("█", bar)),
				pad,
				greenStyle.Render(fmtMoney(s.Amount)),
				s.Percent))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r ReportsModel) viewCashFlow() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Cash Flow — Last 6 Months") + "\n\n")

	if len(r.cashflow) == 0 {
		sb.WriteString("  No data yet.\n")
		return sb.String()
	}

	// Find max for scaling bars
	var maxVal int64
	for _, cf := range r.cashflow {
		if cf.Income > maxVal {
			maxVal = cf.Income
		}
		if cf.Expenses > maxVal {
			maxVal = cf.Expenses
		}
	}

	barWidth := r.width - 55
	if barWidth < 15 {
		barWidth = 15
	}
	if barWidth > 40 {
		barWidth = 40
	}

	// Totals
	var totalInc, totalExp int64
	for _, cf := range r.cashflow {
		totalInc += cf.Income
		totalExp += cf.Expenses
	}
	totalNet := totalInc - totalExp
	totNetStyle := greenStyle
	if totalNet < 0 {
		totNetStyle = redStyle
	}
	sb.WriteString(fmt.Sprintf("  Total Income: %s  Expenses: %s  Net: %s\n\n",
		greenStyle.Render(fmtMoney(totalInc)),
		redStyle.Render(fmtMoney(totalExp)),
		totNetStyle.Render(fmtMoney(totalNet))))

	for _, cf := range r.cashflow {
		// Month header with net
		netStyle := greenStyle
		if cf.Net < 0 {
			netStyle = redStyle
		}
		sb.WriteString(fmt.Sprintf("  %s  net %s\n",
			lipgloss.NewStyle().Bold(true).Render(cf.Month),
			netStyle.Render(fmtMoneySign(cf.Net))))

		// Income bar
		incBar := 0
		if maxVal > 0 {
			incBar = int(float64(cf.Income) / float64(maxVal) * float64(barWidth))
		}
		if incBar < 1 && cf.Income > 0 {
			incBar = 1
		}
		sb.WriteString(fmt.Sprintf("    %s %s  %s\n",
			lipgloss.NewStyle().Foreground(muted).Render("IN "),
			greenStyle.Render(strings.Repeat("█", incBar)+strings.Repeat("░", barWidth-incBar)),
			greenStyle.Render(fmtMoney(cf.Income))))

		// Expense bar
		expBar := 0
		if maxVal > 0 {
			expBar = int(float64(cf.Expenses) / float64(maxVal) * float64(barWidth))
		}
		if expBar < 1 && cf.Expenses > 0 {
			expBar = 1
		}
		sb.WriteString(fmt.Sprintf("    %s %s  %s\n",
			lipgloss.NewStyle().Foreground(muted).Render("OUT"),
			redStyle.Render(strings.Repeat("█", expBar)+strings.Repeat("░", barWidth-expBar)),
			redStyle.Render(fmtMoney(cf.Expenses))))

		sb.WriteString("\n")
	}
	return sb.String()
}

func (r ReportsModel) viewNetWorthHistory() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Net Worth Over Time") + "\n\n")

	if len(r.nwHistory) < 2 {
		sb.WriteString("  Not enough data yet. Net worth history builds as you use the app.\n")
		return sb.String()
	}

	chartW := r.width - 8
	if chartW < 30 {
		chartW = 30
	}
	if chartW > 100 {
		chartW = 100
	}
	chartH := r.height - 12
	if chartH < 8 {
		chartH = 8
	}
	if chartH > 20 {
		chartH = 20
	}

	tslc := timeserieslinechart.New(chartW, chartH,
		timeserieslinechart.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("10"))),
	)

	var latest model.NetWorthSnapshot
	for _, snap := range r.nwHistory {
		t, err := time.Parse("2006-01-02", snap.Date)
		if err != nil {
			continue
		}
		tslc.Push(timeserieslinechart.TimePoint{Time: t, Value: float64(snap.NetWorth) / 100})
		latest = snap
	}
	tslc.DrawBraille()
	sb.WriteString("  " + tslc.View() + "\n\n")

	// nwHistory is in desc order (newest first), so first entry is latest
	first := r.nwHistory[len(r.nwHistory)-1] // oldest
	change := latest.NetWorth - first.NetWorth
	changeStyle := greenStyle
	if change < 0 {
		changeStyle = redStyle
	}
	sb.WriteString(fmt.Sprintf("  Current: %s  |  Change: %s  |  Period: %s to %s\n",
		greenStyle.Bold(true).Render(fmtMoney(latest.NetWorth)),
		changeStyle.Render(fmtMoneySign(change)),
		first.Date, latest.Date))
	sb.WriteString(fmt.Sprintf("  Assets: %s  |  Investments: %s  |  Liabilities: %s\n",
		fmtMoney(latest.TotalAssets), fmtMoney(latest.EquityValue), fmtMoney(latest.TotalLiabilities)))

	return sb.String()
}
