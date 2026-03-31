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

type TransactionsModel struct {
	svc           *service.Service
	width, height int
	txs           []model.Transaction
	cursor        int
	offset        int
	search        string
	searching     bool
	filter        model.TxFilter
	form          FormModel
}

func (t TransactionsModel) InputActive() bool { return t.searching || t.form.Active() }

func NewTransactionsModel(svc *service.Service) TransactionsModel {
	return TransactionsModel{svc: svc, filter: model.TxFilter{Limit: 100}}
}

type txDataMsg struct {
	txs []model.Transaction
}

func (t TransactionsModel) Init() tea.Cmd {
	return func() tea.Msg {
		txs, _ := t.svc.ListTransactions(t.filter)
		return txDataMsg{txs}
	}
}

func (t TransactionsModel) Update(msg tea.Msg) (TransactionsModel, tea.Cmd) {
	if t.form.Active() {
		var cmd tea.Cmd
		t.form, cmd = t.form.Update(msg)
		return t, cmd
	}

	switch msg := msg.(type) {
	case txDataMsg:
		t.txs = msg.txs
		t.cursor = 0
		t.offset = 0

	case tea.KeyMsg:
		if t.searching {
			switch msg.String() {
			case "enter":
				t.searching = false
				t.filter.Search = t.search
				return t, t.Init()
			case "esc":
				t.searching = false
				t.search = ""
				t.filter.Search = ""
				return t, t.Init()
			case "backspace":
				if len(t.search) > 0 {
					t.search = t.search[:len(t.search)-1]
				}
			default:
				if len(msg.String()) == 1 {
					t.search += msg.String()
				}
			}
			return t, nil
		}

		switch msg.String() {
		case "j", "down":
			if t.cursor < len(t.txs)-1 {
				t.cursor++
			}
		case "k", "up":
			if t.cursor > 0 {
				t.cursor--
			}
		case "g":
			t.cursor = 0
		case "G":
			if len(t.txs) > 0 {
				t.cursor = len(t.txs) - 1
			}
		case "/":
			t.searching = true
			t.search = ""
		case "a":
			t.form = t.newAddForm()
		case "d":
			if len(t.txs) > 0 && t.cursor < len(t.txs) {
				t.svc.DeleteTransaction(t.txs[t.cursor].ID)
				return t, t.Init()
			}
		}
	}
	return t, nil
}

func (t *TransactionsModel) newAddForm() FormModel {
	svc := t.svc
	filter := t.filter
	return NewForm("Add Transaction", []FormField{
		{Label: "Amount", Placeholder: "0.00"},
		{Label: "Payee", Placeholder: "e.g. Whole Foods"},
		{Label: "Category", Placeholder: "e.g. Groceries"},
		{Label: "Account", Placeholder: "e.g. Checking"},
		{Label: "Type", Value: "expense", Options: []string{"expense", "income", "transfer"}},
		{Label: "Date", Value: time.Now().Format("2006-01-02")},
		{Label: "Note", Placeholder: "optional"},
	}, func(fields []FormField) (tea.Cmd, string) {
		amtStr := strings.TrimSpace(fields[0].Value)
		payee := strings.TrimSpace(fields[1].Value)
		catName := strings.TrimSpace(fields[2].Value)
		accName := strings.TrimSpace(fields[3].Value)
		txType := fields[4].Value
		date := strings.TrimSpace(fields[5].Value)
		note := strings.TrimSpace(fields[6].Value)

		if amtStr == "" {
			return nil, "Amount is required"
		}
		if accName == "" {
			return nil, "Account is required"
		}
		amount, err := strconv.ParseFloat(amtStr, 64)
		if err != nil || amount <= 0 {
			return nil, "Invalid amount — enter a number like 45.50"
		}
		cents := int64(math.Round(amount * 100))

		acc, err := svc.GetAccountByName(accName)
		if err != nil {
			return nil, fmt.Sprintf("Account %q not found", accName)
		}

		var catID *int64
		if catName != "" {
			c, err := svc.FindCategoryByName(catName)
			if err != nil {
				return nil, fmt.Sprintf("Category %q not found — try: Groceries, Rent/Mortgage, Streaming, etc.", catName)
			}
			catID = &c.ID
		}
		if catID == nil {
			catID = svc.AutoCategorize(payee)
		}

		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		tx := model.Transaction{
			AccountID:  acc.ID,
			CategoryID: catID,
			Amount:     cents,
			Date:       date,
			Payee:      payee,
			Note:       note,
			Type:       model.TxType(txType),
		}
		svc.CreateTransaction(tx)

		return func() tea.Msg {
			txs, _ := svc.ListTransactions(filter)
			return txDataMsg{txs}
		}, ""
	})
}

func (t *TransactionsModel) SetSize(w, h int) {
	t.width = w
	t.height = h
}

func (t TransactionsModel) View() string {
	if t.form.Active() {
		return t.form.View()
	}

	var b strings.Builder

	if t.searching {
		b.WriteString(headerStyle.Render("Search: ") + t.search + "█\n\n")
	} else {
		title := "Transactions"
		if t.filter.Search != "" {
			title += fmt.Sprintf(" (search: %q)", t.filter.Search)
		}
		b.WriteString(headerStyle.Render(title) + "\n")
	}

	if len(t.txs) == 0 {
		b.WriteString("  No transactions found. Press 'a' to add one or import with: budget import <file>\n")
		return b.String()
	}

	// Header
	hdr := fmt.Sprintf("  %-10s  %-25s  %10s  %-8s  %-15s  %-15s",
		"DATE", "PAYEE", "AMOUNT", "TYPE", "CATEGORY", "ACCOUNT")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	visible := t.height - 6
	if visible < 5 {
		visible = 5
	}

	// Scroll window
	start := 0
	if t.cursor >= visible {
		start = t.cursor - visible + 1
	}
	end := start + visible
	if end > len(t.txs) {
		end = len(t.txs)
	}

	for i := start; i < end; i++ {
		tx := t.txs[i]
		sign := "-"
		style := redStyle
		if tx.Type == model.TxIncome {
			sign = "+"
			style = greenStyle
		}
		amountStr := style.Render(fmt.Sprintf("%s$%.2f", sign, float64(tx.Amount)/100))

		line := fmt.Sprintf("  %-10s  %-25s  %10s  %-8s  %-15s  %-15s",
			tx.Date, truncStr(tx.Payee, 25), amountStr, tx.Type,
			truncStr(tx.CategoryName, 15), truncStr(tx.AccountName, 15))

		if i == t.cursor {
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(fmt.Sprintf("\n  %d transactions | j/k:navigate  /:search  d:delete  g/G:top/bottom", len(t.txs)))
	return b.String()
}
