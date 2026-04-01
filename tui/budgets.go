package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/model"
	"github.com/sam/budget/internal/service"
)

type BudgetsModel struct {
	svc           *service.Service
	width, height int
	statuses      []model.BudgetStatus
	cursor        int
	form          FormModel
	viewYear      int
	viewMonth     int
}

func (b BudgetsModel) InputActive() bool { return b.form.Active() }

func NewBudgetsModel(svc *service.Service) BudgetsModel {
	now := time.Now()
	return BudgetsModel{svc: svc, viewYear: now.Year(), viewMonth: int(now.Month())}
}

type budgetDataMsg struct {
	statuses []model.BudgetStatus
}

func (b BudgetsModel) Init() tea.Cmd {
	year, month := b.viewYear, b.viewMonth
	return func() tea.Msg {
		statuses, _ := b.svc.GetBudgetStatus(year, month)
		return budgetDataMsg{statuses}
	}
}

func (b BudgetsModel) Update(msg tea.Msg) (BudgetsModel, tea.Cmd) {
	if b.form.Active() {
		var cmd tea.Cmd
		b.form, cmd = b.form.Update(msg)
		return b, cmd
	}

	switch msg := msg.(type) {
	case budgetDataMsg:
		b.statuses = msg.statuses
		b.cursor = 0
	case tea.KeyMsg:
		switch msg.String() {
		case "j", keyDown:
			if b.cursor < len(b.statuses)-1 {
				b.cursor++
			}
		case "k", "up":
			if b.cursor > 0 {
				b.cursor--
			}
		case "g":
			b.cursor = 0
		case "G":
			if len(b.statuses) > 0 {
				b.cursor = len(b.statuses) - 1
			}
		case "[":
			b.viewMonth--
			if b.viewMonth < 1 {
				b.viewMonth = 12
				b.viewYear--
			}
			b.cursor = 0
			return b, b.Init()
		case "]":
			b.viewMonth++
			if b.viewMonth > 12 {
				b.viewMonth = 1
				b.viewYear++
			}
			b.cursor = 0
			return b, b.Init()
		case "a":
			b.form = b.newSetForm()
		case "e", "enter":
			if len(b.statuses) > 0 && b.cursor < len(b.statuses) {
				b.form = b.newEditForm(b.statuses[b.cursor])
			}
		}
	}
	return b, nil
}

func (b *BudgetsModel) newSetForm() FormModel {
	svc := b.svc
	year, month := b.viewYear, b.viewMonth
	return NewForm("Set Budget", []FormField{
		{Label: "Category", Placeholder: "e.g. Groceries, Rent, Streaming"},
		{Label: "Monthly Limit", Placeholder: "e.g. 600"},
	}, func(fields []FormField) (tea.Cmd, string) {
		catName := strings.TrimSpace(fields[0].Value)
		amtStr := strings.TrimSpace(fields[1].Value)
		if catName == "" {
			return nil, "Category is required"
		}
		if amtStr == "" {
			return nil, "Monthly limit is required"
		}
		cat, err := svc.FindCategoryByName(catName)
		if err != nil {
			return nil, fmt.Sprintf("Category %q not found — try: Groceries, Rent/Mortgage, Streaming, etc.", catName)
		}
		amount, err := strconv.ParseFloat(amtStr, 64)
		if err != nil || amount <= 0 {
			return nil, "Invalid amount — enter a number like 600"
		}
		cents := int64(math.Round(amount * 100))
		if err := svc.SetBudget(cat.ID, year, month, cents); err != nil {
			return nil, fmt.Sprintf("Error setting budget: %v", err)
		}
		return func() tea.Msg {
			statuses, _ := svc.GetBudgetStatus(year, month)
			return budgetDataMsg{statuses}
		}, ""
	})
}

func (b *BudgetsModel) newEditForm(status model.BudgetStatus) FormModel {
	svc := b.svc
	catID := status.CategoryID
	year, month := b.viewYear, b.viewMonth
	currentLimit := ""
	if status.AmountLimit > 0 {
		currentLimit = fmt.Sprintf("%.2f", float64(status.AmountLimit)/100)
	}
	return NewForm(fmt.Sprintf("Edit Budget — %s", status.CategoryName), []FormField{
		{Label: "Monthly Limit", Value: currentLimit, Placeholder: "e.g. 600 (0 to remove)"},
	}, func(fields []FormField) (tea.Cmd, string) {
		amtStr := strings.TrimSpace(fields[0].Value)
		if amtStr == "" {
			return nil, "Monthly limit is required (enter 0 to remove)"
		}
		amount, err := strconv.ParseFloat(amtStr, 64)
		if err != nil || amount < 0 {
			return nil, "Invalid amount — enter a number like 600"
		}
		cents := int64(math.Round(amount * 100))
		if err := svc.SetBudget(catID, year, month, cents); err != nil {
			return nil, fmt.Sprintf("Failed to set budget: %v", err)
		}
		return func() tea.Msg {
			statuses, _ := svc.GetBudgetStatus(year, month)
			return budgetDataMsg{statuses}
		}, ""
	})
}

func (b *BudgetsModel) SetSize(w, h int) {
	b.width = w
	b.height = h
}

func (b BudgetsModel) View() string {
	if b.form.Active() {
		return b.form.View()
	}

	monthName := time.Month(b.viewMonth).String()
	now := time.Now()
	isCurrent := b.viewYear == now.Year() && b.viewMonth == int(now.Month())

	var sb strings.Builder

	titleStr := fmt.Sprintf("Budgets — %s %d", monthName, b.viewYear)
	if !isCurrent {
		titleStr += "  " + lipgloss.NewStyle().Foreground(muted).Render("[/]:navigate months")
	}
	sb.WriteString(headerStyle.Render(titleStr) + "\n\n")

	if len(b.statuses) == 0 {
		sb.WriteString("  No budgets set for this month.\n")
		sb.WriteString("  Press " + lipgloss.NewStyle().Bold(true).Render("a") + " to add one,")
		sb.WriteString(" or " + lipgloss.NewStyle().Bold(true).Render("[/]") + " to navigate months.\n")
		return sb.String()
	}

	barWidth := b.width - 50
	if barWidth < 20 {
		barWidth = 20
	}
	if barWidth > 60 {
		barWidth = 60
	}

	for i, s := range b.statuses {
		pct := s.Percent
		if pct > 100 {
			pct = 100
		}
		filled := int(pct / 100 * float64(barWidth))
		empty := barWidth - filled

		var barStyle lipgloss.Style
		switch {
		case s.Percent > 100:
			barStyle = redStyle
		case s.Percent > 80:
			barStyle = yellowStyle
		default:
			barStyle = greenStyle
		}

		bar := barStyle.Render(strings.Repeat("█", filled)) + strings.Repeat("░", empty)

		remaining := fmt.Sprintf("%s remaining", fmtMoney(s.Remaining))
		if s.Remaining < 0 {
			remaining = redStyle.Render(fmt.Sprintf("%s over!", fmtMoney(-s.Remaining)))
		}

		line := fmt.Sprintf("  %-18s %s  %5.0f%%  %s / %s  %s",
			truncStr(s.CategoryName, 18),
			bar,
			s.Percent,
			fmtMoney(s.Spent),
			fmtMoney(s.AmountLimit),
			remaining)

		if i == b.cursor {
			line = selectedRowStyle.Render(line)
		} else if i%2 == 1 {
			line = altRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString(fmt.Sprintf("\n  %d categories [%d/%d]", len(b.statuses), b.cursor+1, len(b.statuses)))
	return sb.String()
}
