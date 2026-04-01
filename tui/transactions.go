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
	statusIsGood  bool // true for success messages
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

func (t *TransactionsModel) accountSuggestions(input string) []string {
	accs, _ := t.svc.ListAccounts()
	if input == "" {
		names := make([]string, len(accs))
		for i, a := range accs {
			names[i] = a.Name
		}
		return names
	}
	lower := strings.ToLower(input)
	var matches []string
	for _, a := range accs {
		if strings.Contains(strings.ToLower(a.Name), lower) {
			matches = append(matches, a.Name)
		}
	}
	return matches
}

func (t *TransactionsModel) categorySuggestions(input string) []string {
	if len(t.catList) == 0 {
		t.catList, _ = t.svc.ListCategories()
	}
	if input == "" {
		names := make([]string, len(t.catList))
		for i, c := range t.catList {
			names[i] = c.Name
		}
		return names
	}
	lower := strings.ToLower(input)
	var matches []string
	for _, c := range t.catList {
		if strings.Contains(strings.ToLower(c.Name), lower) {
			matches = append(matches, c.Name)
		}
	}
	return matches
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

	case tea.KeyMsg:
		// Clear status on any keypress
		if t.statusMsg != "" && !t.catPicking && !t.confirmDelete && !t.searching {
			t.statusMsg = ""
			t.statusIsGood = false
		}

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
					t.statusMsg = fmt.Sprintf("Categorized as %s", cat.Name)
					t.statusIsGood = true
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
					payee := t.txs[t.cursor].Payee
					if err := t.svc.DeleteTransaction(t.txs[t.cursor].ID); err != nil {
						t.statusMsg = fmt.Sprintf("Error deleting transaction: %v", err)
						return t, nil
					}
					t.statusMsg = fmt.Sprintf("Deleted %q", payee)
					t.statusIsGood = true
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
		case "e":
			if len(t.txs) > 0 && t.cursor < len(t.txs) {
				t.form = t.newEditForm(t.txs[t.cursor])
			}
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
				var newType model.TxType
				switch tx.Type {
				case model.TxExpense:
					newType = model.TxTransferOut
				case model.TxTransfer, model.TxTransferOut:
					newType = model.TxTransferIn
				case model.TxTransferIn:
					newType = model.TxIncome
				case model.TxIncome:
					newType = model.TxExpense
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
		{Label: "Category", Placeholder: "e.g. Groceries", SuggestFunc: t.categorySuggestions},
		{Label: "Account", Placeholder: "e.g. Checking", SuggestFunc: t.accountSuggestions},
		{Label: "Type", Value: "expense", Options: []string{"expense", "income", "transfer_out", "transfer_in"}},
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
			return nil, errAmountRequired
		}
		if accName == "" {
			return nil, errAccountRequired
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

func (t *TransactionsModel) newEditForm(tx model.Transaction) FormModel {
	svc := t.svc
	filter := t.filter
	txID := tx.ID
	oldAccountID := tx.AccountID
	oldAmount := tx.Amount
	oldType := tx.Type

	return NewForm(fmt.Sprintf("Edit Transaction — %s", truncStr(tx.Payee, 30)), []FormField{
		{Label: "Amount", Value: fmt.Sprintf("%.2f", float64(tx.Amount)/100)},
		{Label: "Payee", Value: tx.Payee},
		{Label: "Category", Value: tx.CategoryName, SuggestFunc: t.categorySuggestions},
		{Label: "Account", Value: tx.AccountName, SuggestFunc: t.accountSuggestions},
		{Label: "Type", Value: string(tx.Type), Options: []string{"expense", "income", "transfer_out", "transfer_in"}},
		{Label: "Date", Value: tx.Date},
		{Label: "Note", Value: tx.Note},
	}, func(fields []FormField) (tea.Cmd, string) {
		amtStr := strings.TrimSpace(fields[0].Value)
		payee := strings.TrimSpace(fields[1].Value)
		catName := strings.TrimSpace(fields[2].Value)
		accName := strings.TrimSpace(fields[3].Value)
		txType := fields[4].Value
		date := strings.TrimSpace(fields[5].Value)
		note := strings.TrimSpace(fields[6].Value)

		if amtStr == "" {
			return nil, errAmountRequired
		}
		if accName == "" {
			return nil, errAccountRequired
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
				return nil, fmt.Sprintf("Category %q not found", catName)
			}
			catID = &c.ID
		}

		// Reverse old balance effect
		oldDelta := oldAmount
		if !oldType.IsCredit() {
			oldDelta = -oldDelta
		}
		if err := svc.UpdateAccountBalance(oldAccountID, -oldDelta); err != nil {
			return nil, fmt.Sprintf("Error reverting balance: %v", err)
		}

		// Update the transaction
		if err := svc.UpdateTransaction(txID, acc.ID, catID, cents, date, payee, note, model.TxType(txType)); err != nil {
			// re-apply old balance on failure
			_ = svc.UpdateAccountBalance(oldAccountID, oldDelta)
			return nil, fmt.Sprintf("Error updating transaction: %v", err)
		}

		// Apply new balance effect
		newDelta := cents
		if !model.TxType(txType).IsCredit() {
			newDelta = -newDelta
		}
		if err := svc.UpdateAccountBalance(acc.ID, newDelta); err != nil {
			return nil, fmt.Sprintf("Error updating balance: %v", err)
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
		b.WriteString("  No transactions found.\n")
		b.WriteString("  Press " + lipgloss.NewStyle().Bold(true).Render("a") + " to add one")
		b.WriteString(" or import with: " + lipgloss.NewStyle().Bold(true).Render("budget import <file>") + "\n")
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
		typeLabel := string(tx.Type)
		if tx.Type.IsCredit() {
			sign = "+"
			style = greenStyle
		}
		if tx.Type.IsTransfer() {
			sign = "~"
			style = lipgloss.NewStyle().Foreground(muted)
			if tx.Type == model.TxTransferIn {
				typeLabel = "xfer in"
			} else {
				typeLabel = "xfer out"
			}
		}
		rawAmount := fmt.Sprintf("%s%s", sign, fmtMoney(tx.Amount))
		paddedAmount := fmt.Sprintf("%10s", rawAmount)
		amountStr := style.Render(paddedAmount)

		line := fmt.Sprintf("  %-10s  %-*s  %s  %-8s  %-*s  %-*s",
			tx.Date, payeeW, truncStr(tx.Payee, payeeW), amountStr, typeLabel,
			catW, truncStr(tx.CategoryName, catW), acctW, truncStr(tx.AccountName, acctW))

		if i == t.cursor {
			line = selectedRowStyle.Render(line)
		} else if i%2 == 1 {
			line = altRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Scroll indicator
	if len(t.txs) > visible {
		b.WriteString(renderScrollIndicator(start, end, len(t.txs), t.width))
	}

	if t.confirmDelete && t.cursor < len(t.txs) {
		tx := t.txs[t.cursor]
		b.WriteString(fmt.Sprintf("\n  %s",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).
				Render(fmt.Sprintf("Delete \"%s\" (%s on %s)? y/N", tx.Payee, fmtMoney(tx.Amount), tx.Date))))
	} else if t.statusMsg != "" {
		style := lipgloss.NewStyle().Foreground(danger)
		if t.statusIsGood {
			style = lipgloss.NewStyle().Foreground(special)
		}
		b.WriteString("\n  " + style.Render(t.statusMsg))
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
