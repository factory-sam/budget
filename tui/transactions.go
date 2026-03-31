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
	search        string
	searching     bool
	filter        model.TxFilter
	form          FormModel
	// category picker
	catPicking  bool
	catList     []model.Category
	catFiltered []model.Category
	catCursor   int
	catSearch   string
	// delete confirmation
	confirmDelete bool
	statusMsg     string
}

func (t TransactionsModel) InputActive() bool {
	return t.searching || t.form.Active() || t.catPicking || t.confirmDelete
}

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

func (t *TransactionsModel) openCategoryPicker() {
	if len(t.catList) == 0 {
		cats, _ := t.svc.ListCategories()
		t.catList = cats
	}
	t.catPicking = true
	t.catCursor = 0
	t.catSearch = ""
	t.catFiltered = t.catList
}

func (t *TransactionsModel) filterCategories() {
	if t.catSearch == "" {
		t.catFiltered = t.catList
		return
	}
	s := strings.ToLower(t.catSearch)
	t.catFiltered = nil
	for _, c := range t.catList {
		if strings.Contains(strings.ToLower(c.Name), s) || strings.Contains(strings.ToLower(c.GroupName), s) {
			t.catFiltered = append(t.catFiltered, c)
		}
	}
	if t.catCursor >= len(t.catFiltered) {
		t.catCursor = 0
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
		if t.cursor >= len(t.txs) && len(t.txs) > 0 {
			t.cursor = len(t.txs) - 1
		}
		// don't reset cursor on reload (preserve position for categorize/type toggle)

	case tea.KeyMsg:
		t.statusMsg = ""
		if t.catPicking {
			switch msg.String() {
			case keyEsc:
				t.catPicking = false
			case keyEnter:
				if len(t.catFiltered) > 0 && t.cursor < len(t.txs) {
					cat := t.catFiltered[t.catCursor]
					tx := t.txs[t.cursor]
					if err := t.svc.UpdateTransactionCategory(tx.ID, &cat.ID); err != nil {
						t.statusMsg = fmt.Sprintf("Error categorizing: %v", err)
						t.catPicking = false
						return t, nil
					}
					t.catPicking = false
					return t, t.Init()
				}
			case "j", keyDown:
				if t.catCursor < len(t.catFiltered)-1 {
					t.catCursor++
				}
			case "k", "up":
				if t.catCursor > 0 {
					t.catCursor--
				}
			case keyBackspace:
				if len(t.catSearch) > 0 {
					t.catSearch = t.catSearch[:len(t.catSearch)-1]
					t.filterCategories()
				}
			default:
				if len(msg.String()) == 1 {
					t.catSearch += msg.String()
					t.filterCategories()
				}
			}
			return t, nil
		}

		if t.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				t.confirmDelete = false
				if t.cursor < len(t.txs) {
					if err := t.svc.DeleteTransaction(t.txs[t.cursor].ID); err != nil {
						t.statusMsg = fmt.Sprintf("Error deleting transaction: %v", err)
						return t, nil
					}
					return t, t.Init()
				}
			default:
				t.confirmDelete = false
			}
			return t, nil
		}

		if t.searching {
			switch msg.String() {
			case keyEnter:
				t.searching = false
				t.filter.Search = t.search
				return t, t.Init()
			case keyEsc:
				t.searching = false
				t.search = ""
				t.filter.Search = ""
				return t, t.Init()
			case keyBackspace:
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
		case "j", keyDown:
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
		case "u":
			t.filter.Uncategorized = !t.filter.Uncategorized
			t.cursor = 0
			return t, t.Init()
		case "c":
			if len(t.txs) > 0 && t.cursor < len(t.txs) {
				t.openCategoryPicker()
			}
		case "t":
			if len(t.txs) > 0 && t.cursor < len(t.txs) {
				tx := t.txs[t.cursor]
				// Only toggle between expense <-> transfer (both are outflows)
				// Income stays as income — use 'c' to categorize instead
				var newType model.TxType
				switch tx.Type {
				case model.TxExpense:
					newType = model.TxTransfer
				case model.TxTransfer:
					newType = model.TxExpense
				default:
					break
				}
				if newType != "" {
					if err := t.svc.UpdateTransactionType(tx.ID, newType); err != nil {
						t.statusMsg = fmt.Sprintf("Error updating type: %v", err)
						return t, nil
					}
					return t, t.Init()
				}
			}
		case "d":
			if len(t.txs) > 0 && t.cursor < len(t.txs) {
				t.confirmDelete = true
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
		if _, err := svc.CreateTransaction(tx); err != nil {
			return nil, fmt.Sprintf("Error creating transaction: %v", err)
		}

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

	if t.catPicking {
		return t.viewCategoryPicker()
	}

	var b strings.Builder

	if t.searching {
		b.WriteString(headerStyle.Render("Search: ") + t.search + "█\n\n")
	} else {
		title := "Transactions"
		if t.filter.Uncategorized {
			title += " — Uncategorized"
		}
		if t.filter.Search != "" {
			title += fmt.Sprintf(" (search: %q)", t.filter.Search)
		}
		b.WriteString(headerStyle.Render(title) + "\n")
	}

	if len(t.txs) == 0 {
		b.WriteString("  No transactions found. Press 'a' to add one or import with: budget import <file>\n")
		return b.String()
	}

	// Responsive column widths
	payeeW := 25
	catW := 15
	acctW := 15
	if t.width >= 140 {
		payeeW = 35
		catW = 20
		acctW = 20
	} else if t.width < 100 {
		payeeW = 18
		catW = 12
		acctW = 12
	}

	// Header
	hdr := fmt.Sprintf("  %-10s  %-*s  %10s  %-8s  %-*s  %-*s",
		"DATE", payeeW, "PAYEE", "AMOUNT", "TYPE", catW, "CATEGORY", acctW, "ACCOUNT")
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
		} else if tx.Type == model.TxTransfer {
			sign = "~"
			style = lipgloss.NewStyle().Foreground(muted)
		}
		rawAmount := fmt.Sprintf("%s%s", sign, fmtMoney(tx.Amount))
		paddedAmount := fmt.Sprintf("%10s", rawAmount)
		amountStr := style.Render(paddedAmount)

		line := fmt.Sprintf("  %-10s  %-*s  %s  %-8s  %-*s  %-*s",
			tx.Date, payeeW, truncStr(tx.Payee, payeeW), amountStr, string(tx.Type),
			catW, truncStr(tx.CategoryName, catW), acctW, truncStr(tx.AccountName, acctW))

		if i == t.cursor {
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	if t.confirmDelete && t.cursor < len(t.txs) {
		tx := t.txs[t.cursor]
		b.WriteString(fmt.Sprintf("\n  %s",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).
				Render(fmt.Sprintf("Delete \"%s\" (%s on %s)? y/N", tx.Payee, fmtMoney(tx.Amount), tx.Date))))
	} else if t.statusMsg != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(danger).Render(t.statusMsg))
	} else {
		b.WriteString(fmt.Sprintf("\n  %d transactions [%d/%d]", len(t.txs), t.cursor+1, len(t.txs)))
	}
	return b.String()
}

func (t TransactionsModel) viewCategoryPicker() string {
	var b strings.Builder

	tx := t.txs[t.cursor]
	b.WriteString(headerStyle.Render("Categorize Transaction") + "\n")
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n\n",
		tx.Date, truncStr(tx.Payee, 40), fmtMoney(tx.Amount)))
	b.WriteString("  Search: " + t.catSearch + "█\n\n")

	visible := t.height - 8
	if visible < 5 {
		visible = 5
	}

	start := 0
	if t.catCursor >= visible {
		start = t.catCursor - visible + 1
	}
	end := start + visible
	if end > len(t.catFiltered) {
		end = len(t.catFiltered)
	}

	lastGroup := ""
	for i := start; i < end; i++ {
		cat := t.catFiltered[i]
		if cat.GroupName != lastGroup {
			lastGroup = cat.GroupName
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(
				fmt.Sprintf("  %s", cat.GroupName)) + "\n")
		}
		line := fmt.Sprintf("    %-25s", cat.Name)
		if i == t.catCursor {
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n  j/k:navigate  enter:select  esc:cancel  type to filter")
	return b.String()
}
