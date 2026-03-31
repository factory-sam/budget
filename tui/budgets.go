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
}

func (b BudgetsModel) InputActive() bool { return b.form.Active() }

func NewBudgetsModel(svc *service.Service) BudgetsModel {
	return BudgetsModel{svc: svc}
}

type budgetDataMsg struct {
	statuses []model.BudgetStatus
}

func (b BudgetsModel) Init() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		statuses, _ := b.svc.GetBudgetStatus(now.Year(), int(now.Month()))
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
		case "j", "down":
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
		case "a":
			b.form = b.newSetForm()
		}
	}
	return b, nil
}

func (b *BudgetsModel) newSetForm() FormModel {
	svc := b.svc
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
		now := time.Now()
		svc.SetBudget(cat.ID, now.Year(), int(now.Month()), cents)
		return func() tea.Msg {
			statuses, _ := svc.GetBudgetStatus(now.Year(), int(now.Month()))
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

	now := time.Now()
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Budgets — %s %d", now.Month().String(), now.Year())) + "\n\n")

	if len(b.statuses) == 0 {
		sb.WriteString("  No budgets set. Use the CLI to set budgets:\n")
		sb.WriteString("  budget set Groceries 600\n")
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

		remaining := fmt.Sprintf("$%.2f remaining", float64(s.Remaining)/100)
		if s.Remaining < 0 {
			remaining = redStyle.Render(fmt.Sprintf("$%.2f over!", float64(-s.Remaining)/100))
		}

		line := fmt.Sprintf("  %-18s %s  %5.0f%%  $%.2f / $%.2f  %s",
			truncStr(s.CategoryName, 18),
			bar,
			s.Percent,
			float64(s.Spent)/100,
			float64(s.AmountLimit)/100,
			remaining)

		if i == b.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString(fmt.Sprintf("\n  j/k:navigate"))
	return sb.String()
}
