package tui

import (
	"fmt"
	"strings"

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
}

func NewApp(svc *service.Service) *App {
	db := svc.DB()
	ps := equity.NewPriceService(db)
	portSvc := equity.NewPortfolioService(db, ps)
	grantSvc := equity.NewGrantService(db, ps, portSvc)

	return &App{
		svc:          svc,
		dashboard:    NewDashboardModel(svc),
		transactions: NewTransactionsModel(svc),
		budgets:      NewBudgetsModel(svc),
		accounts:     NewAccountsModel(svc),
		recurring:    NewRecurringModel(svc),
		reports:      NewReportsModel(svc),
		portfolio:    NewPortfolioModel(ps, portSvc, grantSvc),
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
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if a.showModal {
			var cmd tea.Cmd
			a.modal, cmd = a.modal.Update(msg)
			if msg.String() == "esc" {
				a.showModal = false
				return a, nil
			}
			return a, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
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
		case "tab":
			a.activeTab = (a.activeTab + 1) % Tab(len(tabNames))
			return a, a.initActiveTab()
		case "shift+tab":
			a.activeTab = (a.activeTab - 1 + Tab(len(tabNames))) % Tab(len(tabNames))
			return a, a.initActiveTab()
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
		return "Loading..."
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
	help := statusBarStyle.Render("?:help  a:add  e:edit  d:delete  /:search  tab:switch  q:quit")

	// Compose
	header := titleStyle.Render("budget") + "  " + tabBar
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", content)

	// Pad content to fill screen
	contentHeight := a.height - 3 // header + help + gap
	lines := strings.Count(view, "\n") + 1
	if lines < contentHeight {
		view += strings.Repeat("\n", contentHeight-lines)
	}
	view += "\n" + help

	return view
}
