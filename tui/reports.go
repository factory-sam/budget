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
	activeReport  int // 0=spending, 1=cashflow
}

func NewReportsModel(svc *service.Service) ReportsModel {
	return ReportsModel{svc: svc}
}

type reportsDataMsg struct {
	spending []service.SpendingByCategory
	cashflow []service.CashFlowReport
}

func (r ReportsModel) Init() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		sp, _ := r.svc.SpendingReport(now.Year(), int(now.Month()))
		from := now.AddDate(0, -6, 0).Format("2006-01-02")
		to := now.Format("2006-01-02")
		cf, _ := r.svc.CashFlowReport(from, to)
		return reportsDataMsg{sp, cf}
	}
}

func (r ReportsModel) Update(msg tea.Msg) (ReportsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case reportsDataMsg:
		r.spending = msg.spending
		r.cashflow = msg.cashflow
	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			if r.activeReport > 0 {
				r.activeReport--
			}
		case "l", "right":
			if r.activeReport < 1 {
				r.activeReport++
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
	tabs := []string{"Spending", "Cash Flow"}
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
	}

	sb.WriteString("\n  h/l:switch reports")
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

func (r ReportsModel) viewCashFlow() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Cash Flow — Last 6 Months") + "\n\n")

	if len(r.cashflow) == 0 {
		sb.WriteString("  No data yet.\n")
		return sb.String()
	}

	hdr := fmt.Sprintf("  %-10s  %12s  %12s  %12s", "MONTH", "INCOME", "EXPENSES", "NET")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	for _, cf := range r.cashflow {
		netStyle := greenStyle
		if cf.Net < 0 {
			netStyle = redStyle
		}
		sb.WriteString(fmt.Sprintf("  %-10s  %s  %s  %s\n",
			cf.Month,
			greenStyle.Render(fmt.Sprintf("$%10.2f", float64(cf.Income)/100)),
			redStyle.Render(fmt.Sprintf("$%10.2f", float64(cf.Expenses)/100)),
			netStyle.Render(fmt.Sprintf("$%10.2f", float64(cf.Net)/100))))
	}
	return sb.String()
}
