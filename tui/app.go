package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/service"
)

type Tab int

const (
	TabDashboard Tab = iota
	TabTransactions
	TabBudgets
	TabAccounts
	TabRecurring
	TabReports
	TabPortfolio
)

var tabNames = []string{"Dashboard", "Transactions", "Budgets", "Accounts", "Recurring", "Reports", "Portfolio"}

type App struct {
	svc       *service.Service
	activeTab Tab
	width     int
	height    int

	dashboard    DashboardModel
	transactions TransactionsModel
	budgets      BudgetsModel
	accounts     AccountsModel
	recurring    RecurringModel
	reports      ReportsModel
	portfolio    PortfolioModel

	modal     tea.Model
	showModal bool
	showHelp  bool
	spinner   spinner.Model
}

func NewApp(svc *service.Service) *App {
	db := svc.DB()
	ps := equity.NewPriceService(db)
	portSvc := equity.NewPortfolioService(db, ps)
	grantSvc := equity.NewGrantService(db, ps, portSvc)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	return &App{
		svc:          svc,
		dashboard:    NewDashboardModel(svc),
		transactions: NewTransactionsModel(svc),
		budgets:      NewBudgetsModel(svc),
		accounts:     NewAccountsModel(svc),
		recurring:    NewRecurringModel(svc),
		reports:      NewReportsModel(svc),
		portfolio:    NewPortfolioModel(ps, portSvc, grantSvc, svc),
		spinner:      sp,
	}
}

type RefreshMsg struct{}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.dashboard.Init(),
		a.transactions.Init(),
		a.budgets.Init(),
		a.accounts.Init(),
		a.recurring.Init(),
		a.spinner.Tick,
	)
}

func (a *App) childInputActive() bool {
	switch a.activeTab {
	case TabTransactions:
		return a.transactions.InputActive()
	case TabAccounts:
		return a.accounts.InputActive()
	case TabBudgets:
		return a.budgets.InputActive()
	case TabRecurring:
		return a.recurring.InputActive()
	case TabPortfolio:
		return a.portfolio.InputActive()
	}
	return false
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd

	case tea.KeyMsg:
		if a.showHelp {
			a.showHelp = false
			return a, nil
		}

		if a.showModal {
			var cmd tea.Cmd
			a.modal, cmd = a.modal.Update(msg)
			if msg.String() == keyEsc {
				a.showModal = false
				return a, nil
			}
			return a, cmd
		}

		// When a child view has an active text input, send keys there directly.
		if a.childInputActive() {
			if msg.String() == "ctrl+c" {
				return a, tea.Quit
			}
			var cmd tea.Cmd
			switch a.activeTab {
			case TabTransactions:
				a.transactions, cmd = a.transactions.Update(msg)
			case TabAccounts:
				a.accounts, cmd = a.accounts.Update(msg)
			case TabBudgets:
				a.budgets, cmd = a.budgets.Update(msg)
			case TabRecurring:
				a.recurring, cmd = a.recurring.Update(msg)
			case TabPortfolio:
				a.portfolio, cmd = a.portfolio.Update(msg)
			}
			return a, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "?":
			a.showHelp = !a.showHelp
			return a, nil
		case "1":
			a.activeTab = TabDashboard
			return a, a.dashboard.Init()
		case "2":
			a.activeTab = TabTransactions
			return a, a.transactions.Init()
		case "3":
			a.activeTab = TabBudgets
			return a, a.budgets.Init()
		case "4":
			a.activeTab = TabAccounts
			return a, a.accounts.Init()
		case "5":
			a.activeTab = TabRecurring
			return a, a.recurring.Init()
		case "6":
			a.activeTab = TabReports
			return a, a.reports.Init()
		case "7":
			a.activeTab = TabPortfolio
			return a, a.portfolio.Init()
		case keyTab:
			a.activeTab = (a.activeTab + 1) % Tab(len(tabNames))
			return a, a.initActiveTab()
		case keyShiftTab:
			a.activeTab = (a.activeTab - 1 + Tab(len(tabNames))) % Tab(len(tabNames))
			return a, a.initActiveTab()
		}

	case tea.MouseMsg:
		// Mouse support for tab bar clicks
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			if msg.Y <= 1 {
				x := 0
				for i, name := range tabNames {
					label := fmt.Sprintf("%d:%s", i+1, name)
					tabW := len(label) + 4 // padding
					if msg.X >= x && msg.X < x+tabW {
						a.activeTab = Tab(i)
						return a, a.initActiveTab()
					}
					x += tabW
				}
			}
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.dashboard.SetSize(msg.Width, msg.Height-4)
		a.transactions.SetSize(msg.Width, msg.Height-4)
		a.budgets.SetSize(msg.Width, msg.Height-4)
		a.accounts.SetSize(msg.Width, msg.Height-4)
		a.recurring.SetSize(msg.Width, msg.Height-4)
		a.reports.SetSize(msg.Width, msg.Height-4)
		a.portfolio.SetSize(msg.Width, msg.Height-4)

	case RefreshMsg:
		return a, a.initActiveTab()
	}

	var cmd tea.Cmd
	switch a.activeTab {
	case TabDashboard:
		a.dashboard, cmd = a.dashboard.Update(msg)
	case TabTransactions:
		a.transactions, cmd = a.transactions.Update(msg)
	case TabBudgets:
		a.budgets, cmd = a.budgets.Update(msg)
	case TabAccounts:
		a.accounts, cmd = a.accounts.Update(msg)
	case TabRecurring:
		a.recurring, cmd = a.recurring.Update(msg)
	case TabReports:
		a.reports, cmd = a.reports.Update(msg)
	case TabPortfolio:
		a.portfolio, cmd = a.portfolio.Update(msg)
	}
	return a, cmd
}

func (a *App) initActiveTab() tea.Cmd {
	switch a.activeTab {
	case TabDashboard:
		return a.dashboard.Init()
	case TabTransactions:
		return a.transactions.Init()
	case TabBudgets:
		return a.budgets.Init()
	case TabAccounts:
		return a.accounts.Init()
	case TabRecurring:
		return a.recurring.Init()
	case TabReports:
		return a.reports.Init()
	case TabPortfolio:
		return a.portfolio.Init()
	}
	return nil
}

func (a *App) View() string {
	if a.width == 0 {
		return a.spinner.View() + " Loading..."
	}

	// Help overlay
	if a.showHelp {
		return a.viewHelp()
	}

	// Tab bar
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf("%d:%s", i+1, name)
		if Tab(i) == a.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	// Content
	var content string
	switch a.activeTab {
	case TabDashboard:
		content = a.dashboard.View()
	case TabTransactions:
		content = a.transactions.View()
	case TabBudgets:
		content = a.budgets.View()
	case TabAccounts:
		content = a.accounts.View()
	case TabRecurring:
		content = a.recurring.View()
	case TabReports:
		content = a.reports.View()
	case TabPortfolio:
		content = a.portfolio.View()
	}

	// Status bar
	help := statusBarStyle.Render(a.helpText())

	// Compose
	header := titleStyle.Render("budget") + "  " + tabBar
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", content)

	contentHeight := a.height - 3
	lines := strings.Count(view, "\n") + 1
	if lines < contentHeight {
		view += strings.Repeat("\n", contentHeight-lines)
	}
	view += "\n" + help

	return view
}

func (a *App) viewHelp() string {
	helpBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(highlight).
		Padding(1, 3).
		Width(60)

	title := lipgloss.NewStyle().Bold(true).Foreground(highlight).Render("Keyboard Shortcuts")

	sections := []struct {
		header string
		keys   [][2]string
	}{
		{"Global", [][2]string{
			{"1-7", "Switch to tab"},
			{"tab / shift+tab", "Next / previous tab"},
			{"?", "Toggle this help"},
			{"q / ctrl+c", "Quit"},
		}},
		{"Lists", [][2]string{
			{"j / k", "Navigate down / up"},
			{"g / G", "Jump to top / bottom"},
		}},
		{"Transactions", [][2]string{
			{"a", "Add transaction"},
			{"e", "Edit transaction"},
			{"c", "Categorize"},
			{"t", "Cycle type"},
			{"d", "Delete"},
			{"/", "Search"},
			{"u", "Toggle uncategorized"},
		}},
		{"Budgets", [][2]string{
			{"a", "Set new budget"},
			{"e / enter", "Edit budget"},
			{"[ / ]", "Previous / next month"},
		}},
		{"Reports", [][2]string{
			{"h / l", "Switch reports"},
			{"[ / ]", "Previous / next period"},
			{"g", "Cycle income grouping"},
		}},
		{"Portfolio", [][2]string{
			{"a", "Buy stock / add grant"},
			{"d", "Delete"},
			{"l", "Toggle lots"},
			{"f", "Filter by account"},
			{"r", "Refresh prices"},
			{"p / g / s", "Positions / grants / schedule"},
		}},
		{"Forms", [][2]string{
			{"tab / shift+tab", "Next / previous field"},
			{"enter", "Submit (on last field)"},
			{"ctrl+n / ctrl+p", "Navigate suggestions"},
			{"esc", "Cancel"},
		}},
	}

	var sb strings.Builder
	sb.WriteString(title + "\n\n")
	for _, sec := range sections {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(special).Render(sec.header) + "\n")
		for _, kv := range sec.keys {
			key := lipgloss.NewStyle().Width(22).Foreground(highlight).Render(kv[0])
			sb.WriteString("  " + key + kv[1] + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("Press any key to close"))

	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
		helpBox.Render(sb.String()))
}

func (a *App) helpText() string {
	common := "1-7:tabs  tab/shift+tab:switch  ?:help  q:quit"
	switch a.activeTab {
	case TabDashboard:
		return common
	case TabTransactions:
		if a.transactions.catPicking {
			return "j/k:navigate  enter:select  esc:cancel  type to filter"
		}
		if a.transactions.InputActive() {
			return helpFormNav
		}
		return "j/k:navigate  a:add  e:edit  c:categorize  t:type  u:uncategorized  /:search  d:delete  " + common
	case TabBudgets:
		if a.budgets.InputActive() {
			return helpFormNav
		}
		return "j/k:navigate  e/enter:edit  a:set budget  [/]:month  g/G:top/bottom  " + common
	case TabAccounts:
		if a.accounts.confirmDelete {
			return helpConfirmDelete
		}
		if a.accounts.InputActive() {
			return helpFormNav
		}
		return "j/k:navigate  a:add  g/G:top/bottom  d:delete  " + common
	case TabRecurring:
		if a.recurring.confirmDelete {
			return helpConfirmDelete
		}
		if a.recurring.InputActive() {
			return helpFormNav
		}
		return "j/k:navigate  a:add  g/G:top/bottom  d:delete  " + common
	case TabReports:
		return "h/l:switch reports  [/]:period  " + common
	case TabPortfolio:
		if a.portfolio.confirmDelete {
			return helpConfirmDelete
		}
		if a.portfolio.InputActive() {
			return helpFormNav
		}
		switch a.portfolio.subView {
		case PortfolioPositions:
			return "j/k:navigate  a:buy  d:delete  l:lots  f:filter account  g:grants  r:refresh  " + common
		case PortfolioGrants:
			return "j/k:navigate  a:add grant  d:delete  v:vest  s:schedule  p:positions  " + common
		case PortfolioVestSchedule:
			return "j/k:navigate  v:exercise(ISO)  esc:back  p:positions  g:grants  " + common
		}
		return "j/k:navigate  " + common
	}
	return common
}
