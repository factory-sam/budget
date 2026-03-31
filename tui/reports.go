package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/service"
)

type ReportsModel struct {
	svc           *service.Service
	width, height int
	spending      []service.SpendingByCategory
	cashflow      []service.CashFlowReport
	income        []service.IncomePeriod
	incomeGroup   string // monthly, quarterly, yearly
	activeReport  int    // 0=spending, 1=cashflow, 2=income
}

func NewReportsModel(svc *service.Service) ReportsModel {
	return ReportsModel{svc: svc, incomeGroup: "monthly"}
}

type reportsDataMsg struct {
	spending []service.SpendingByCategory
	cashflow []service.CashFlowReport
	income   []service.IncomePeriod
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
		return reportsDataMsg{sp, cf, inc}
	}
}

func (r ReportsModel) Update(msg tea.Msg) (ReportsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case reportsDataMsg:
		r.spending = msg.spending
		r.cashflow = msg.cashflow
		r.income = msg.income
	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			if r.activeReport > 0 {
				r.activeReport--
			}
		case "l", "right":
			if r.activeReport < 2 {
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
	tabs := []string{"Spending", "Cash Flow", "Income"}
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

		sb.WriteString(fmt.Sprintf("  %-18s  $%8.2f  %5.1f%%  %s\n",
			truncStr(sp.CategoryName, 18), float64(sp.Amount)/100, sp.Percent, bar))
	}

	var total int64
	for _, sp := range r.spending {
		total += sp.Amount
	}
	sb.WriteString(fmt.Sprintf("\n  Total: $%.2f\n", float64(total)/100))
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
		greenStyle.Render(fmt.Sprintf("$%.2f", float64(grandTotal)/100))))

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
			greenStyle.Render(fmt.Sprintf("$%.2f", float64(p.Total)/100))))

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
				greenStyle.Render(fmt.Sprintf("$%.2f", float64(s.Amount)/100)),
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
		greenStyle.Render(fmt.Sprintf("$%.2f", float64(totalInc)/100)),
		redStyle.Render(fmt.Sprintf("$%.2f", float64(totalExp)/100)),
		totNetStyle.Render(fmt.Sprintf("$%.2f", float64(totalNet)/100))))

	for _, cf := range r.cashflow {
		// Month header with net
		netStyle := greenStyle
		netSign := "+"
		if cf.Net < 0 {
			netStyle = redStyle
			netSign = ""
		}
		sb.WriteString(fmt.Sprintf("  %s  net %s\n",
			lipgloss.NewStyle().Bold(true).Render(cf.Month),
			netStyle.Render(fmt.Sprintf("%s$%.2f", netSign, float64(cf.Net)/100))))

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
			greenStyle.Render(fmt.Sprintf("$%.2f", float64(cf.Income)/100))))

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
			redStyle.Render(fmt.Sprintf("$%.2f", float64(cf.Expenses)/100))))

		sb.WriteString("\n")
	}
	return sb.String()
}
