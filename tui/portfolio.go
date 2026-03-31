package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/model"
)

type PortfolioSubView int

const (
	PortfolioPositions PortfolioSubView = iota
	PortfolioGrants
	PortfolioVestSchedule
)

type PortfolioModel struct {
	prices    *equity.PriceService
	portfolio *equity.PortfolioService
	grants    *equity.GrantService
	svc       interface {
		ListInvestmentAccounts() ([]model.Account, error)
		GetAccountByName(string) (*model.Account, error)
		Get401kStatus(int64, int) (*model.Contribution401k, error)
	}

	width, height   int
	summary         *model.PortfolioSummary
	grantList       []model.EquityGrant
	vestEvents      []model.VestEvent
	vestGrantID     int64
	cursor          int
	subView         PortfolioSubView
	refreshing      bool
	form            FormModel
	statusMsg       string
	// account filter
	investAccounts  []model.Account
	filterIdx       int // 0 = All, 1..N = specific account
	filterAccountID *int64
}

func (p PortfolioModel) InputActive() bool { return p.form.Active() }

func NewPortfolioModel(prices *equity.PriceService, portfolio *equity.PortfolioService, grants *equity.GrantService, svc interface {
	ListInvestmentAccounts() ([]model.Account, error)
	GetAccountByName(string) (*model.Account, error)
	Get401kStatus(int64, int) (*model.Contribution401k, error)
}) PortfolioModel {
	return PortfolioModel{
		prices:    prices,
		portfolio: portfolio,
		grants:    grants,
		svc:       svc,
	}
}

type portfolioDataMsg struct {
	summary *model.PortfolioSummary
	grants  []model.EquityGrant
}

type priceRefreshDoneMsg struct{}

type investAccountsMsg struct {
	accounts []model.Account
}

func (p PortfolioModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			summary, _ := p.portfolio.GetPortfolio(p.filterAccountID)
			grants, _ := p.grants.ListGrants()
			return portfolioDataMsg{summary, grants}
		},
		func() tea.Msg {
			accs, _ := p.svc.ListInvestmentAccounts()
			return investAccountsMsg{accs}
		},
	)
}

func (p PortfolioModel) Update(msg tea.Msg) (PortfolioModel, tea.Cmd) {
	if p.form.Active() {
		var cmd tea.Cmd
		p.form, cmd = p.form.Update(msg)
		return p, cmd
	}

	switch msg := msg.(type) {
	case investAccountsMsg:
		p.investAccounts = msg.accounts

	case portfolioDataMsg:
		if msg.summary != nil {
			p.summary = msg.summary
		}
		if msg.grants != nil {
			p.grantList = msg.grants
		}
		p.cursor = 0
		p.refreshing = false

	case vestScheduleRefreshMsg:
		p.vestEvents = msg.events
		p.cursor = 0
		p.statusMsg = "ISO exercised successfully"

	case priceRefreshDoneMsg:
		p.refreshing = false
		return p, p.Init()

	case tea.KeyMsg:
		p.statusMsg = ""
		switch msg.String() {
		case "j", "down":
			p.cursor++
			maxLen := p.currentListLen()
			if p.cursor >= maxLen {
				p.cursor = maxLen - 1
			}
			if p.cursor < 0 {
				p.cursor = 0
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}
		case "p":
			p.subView = PortfolioPositions
			p.cursor = 0
		case "g":
			if p.subView == PortfolioPositions {
				p.subView = PortfolioGrants
			} else {
				p.cursor = 0
			}
		case "G":
			maxLen := p.currentListLen()
			if maxLen > 0 {
				p.cursor = maxLen - 1
			}
		case "f":
			if p.subView == PortfolioPositions && len(p.investAccounts) > 0 {
				p.filterIdx = (p.filterIdx + 1) % (len(p.investAccounts) + 1)
				if p.filterIdx == 0 {
					p.filterAccountID = nil
				} else {
					p.filterAccountID = &p.investAccounts[p.filterIdx-1].ID
				}
				return p, func() tea.Msg {
					summary, _ := p.portfolio.GetPortfolio(p.filterAccountID)
					return portfolioDataMsg{summary, nil}
				}
			}
		case "a":
			switch p.subView {
			case PortfolioPositions:
				p.form = p.newBuyForm()
			case PortfolioGrants:
				p.form = p.newGrantForm()
			}
		case "d":
			return p.handleDelete()
		case "v":
			if p.subView == PortfolioGrants {
				return p.handleVest()
			}
			if p.subView == PortfolioVestSchedule {
				return p.handleExercise()
			}
		case "s":
			if p.subView == PortfolioGrants && p.cursor < len(p.grantList) {
				grant := p.grantList[p.cursor]
				events, _ := p.grants.GetVestSchedule(grant.ID)
				p.vestEvents = events
				p.vestGrantID = grant.ID
				p.subView = PortfolioVestSchedule
				p.cursor = 0
			}
		case "r":
			p.refreshing = true
			return p, func() tea.Msg {
				tickers, _ := p.portfolio.GetDistinctTickers()
				p.prices.FetchPrices(tickers)
				return priceRefreshDoneMsg{}
			}
		case "esc":
			if p.subView == PortfolioVestSchedule {
				p.subView = PortfolioGrants
				p.cursor = 0
			}
		}
	}
	return p, nil
}

func (p PortfolioModel) handleDelete() (PortfolioModel, tea.Cmd) {
	switch p.subView {
	case PortfolioPositions:
		if p.summary != nil && p.cursor < len(p.summary.Positions) {
			pos := p.summary.Positions[p.cursor]
			// delete all lots for this position
			for _, lot := range pos.Lots {
				p.portfolio.SellLot(lot.ID, lot.Shares)
			}
			p.statusMsg = fmt.Sprintf("Deleted all %s lots", pos.Ticker)
			return p, p.Init()
		}
	case PortfolioGrants:
		if p.cursor < len(p.grantList) {
			grant := p.grantList[p.cursor]
			p.grants.DeleteGrant(grant.ID)
			p.statusMsg = fmt.Sprintf("Deleted %s %s grant", grant.Ticker, strings.ToUpper(grant.GrantType))
			return p, p.Init()
		}
	}
	return p, nil
}

func (p PortfolioModel) handleVest() (PortfolioModel, tea.Cmd) {
	if p.cursor >= len(p.grantList) {
		return p, nil
	}
	grant := p.grantList[p.cursor]
	count, err := p.grants.VestGrant(grant.ID, nil)
	if err != nil {
		p.statusMsg = fmt.Sprintf("Vest error: %v", err)
		return p, nil
	}
	if count == 0 {
		p.statusMsg = "No pending vest events due today"
	} else {
		p.statusMsg = fmt.Sprintf("Vested %d events for %s %s", count, grant.Ticker, strings.ToUpper(grant.GrantType))
	}
	return p, p.Init()
}

func (p PortfolioModel) handleExercise() (PortfolioModel, tea.Cmd) {
	if p.cursor >= len(p.vestEvents) {
		return p, nil
	}
	event := p.vestEvents[p.cursor]
	if event.Status != "vested" {
		p.statusMsg = fmt.Sprintf("Event #%d is %s — only vested events can be exercised", event.ID, event.Status)
		return p, nil
	}
	// open exercise form with FMV input
	p.form = p.newExerciseForm(event)
	return p, nil
}

func (p *PortfolioModel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p PortfolioModel) currentListLen() int {
	switch p.subView {
	case PortfolioPositions:
		if p.summary != nil {
			return len(p.summary.Positions)
		}
	case PortfolioGrants:
		return len(p.grantList)
	case PortfolioVestSchedule:
		return len(p.vestEvents)
	}
	return 0
}

// --- Forms ---

func (p *PortfolioModel) newBuyForm() FormModel {
	ps := p.portfolio
	priceSvc := p.prices
	svc := p.svc
	filterAccID := p.filterAccountID
	return NewForm("Buy Stock", []FormField{
		{Label: "Ticker", Placeholder: "e.g. AAPL"},
		{Label: "Shares", Placeholder: "e.g. 10"},
		{Label: "Price/Share", Placeholder: "e.g. 185.50"},
		{Label: "Account", Placeholder: "e.g. Schwab Brokerage (optional)"},
		{Label: "Date", Value: time.Now().Format("2006-01-02")},
		{Label: "Note", Placeholder: "optional"},
	}, func(fields []FormField) (tea.Cmd, string) {
		ticker := strings.ToUpper(strings.TrimSpace(fields[0].Value))
		sharesStr := strings.TrimSpace(fields[1].Value)
		priceStr := strings.TrimSpace(fields[2].Value)
		accName := strings.TrimSpace(fields[3].Value)
		date := strings.TrimSpace(fields[4].Value)
		note := strings.TrimSpace(fields[5].Value)

		if ticker == "" {
			return nil, "Ticker is required"
		}
		if sharesStr == "" {
			return nil, "Shares is required"
		}
		if priceStr == "" {
			return nil, "Price per share is required"
		}
		shares, err := strconv.ParseFloat(sharesStr, 64)
		if err != nil || shares <= 0 {
			return nil, "Invalid shares — enter a number like 10"
		}
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil || price <= 0 {
			return nil, "Invalid price — enter a number like 185.50"
		}
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		var accountID *int64
		if accName != "" {
			acc, err := svc.GetAccountByName(accName)
			if err != nil {
				return nil, fmt.Sprintf("Account %q not found", accName)
			}
			accountID = &acc.ID
		}

		priceCents := int64(math.Round(price * 100))
		ps.BuyLot(accountID, ticker, shares, priceCents, date, note)
		priceSvc.FetchPrice(ticker)

		return func() tea.Msg {
			summary, _ := ps.GetPortfolio(filterAccID)
			return portfolioDataMsg{summary, nil}
		}, ""
	})
}

func (p *PortfolioModel) newGrantForm() FormModel {
	gs := p.grants
	return NewForm("Add Grant", []FormField{
		{Label: "Ticker", Placeholder: "e.g. ACME"},
		{Label: "Type", Value: "rsu", Options: []string{"iso", "rsu"}},
		{Label: "Total Shares", Placeholder: "e.g. 10000"},
		{Label: "Grant Date", Placeholder: "YYYY-MM-DD"},
		{Label: "Vest Start", Placeholder: "YYYY-MM-DD (blank = grant date)"},
		{Label: "FMV at Grant", Placeholder: "e.g. 25.00"},
		{Label: "Strike (ISO)", Placeholder: "e.g. 12.50 (ISOs only)"},
		{Label: "Cliff Months", Value: "12"},
		{Label: "Vest Months", Value: "48"},
		{Label: "Interval", Value: "monthly", Options: []string{"monthly", "quarterly"}},
	}, func(fields []FormField) (tea.Cmd, string) {
		ticker := strings.ToUpper(strings.TrimSpace(fields[0].Value))
		grantType := fields[1].Value
		sharesStr := strings.TrimSpace(fields[2].Value)
		grantDate := strings.TrimSpace(fields[3].Value)
		vestStart := strings.TrimSpace(fields[4].Value)
		fmvStr := strings.TrimSpace(fields[5].Value)
		strikeStr := strings.TrimSpace(fields[6].Value)
		cliffStr := strings.TrimSpace(fields[7].Value)
		vestStr := strings.TrimSpace(fields[8].Value)
		interval := fields[9].Value

		if ticker == "" {
			return nil, "Ticker is required"
		}
		if sharesStr == "" {
			return nil, "Total shares is required"
		}
		if grantDate == "" {
			return nil, "Grant date is required"
		}
		shares, err := strconv.ParseFloat(sharesStr, 64)
		if err != nil || shares <= 0 {
			return nil, "Invalid shares"
		}
		cliff, _ := strconv.Atoi(cliffStr)
		vest, _ := strconv.Atoi(vestStr)
		if cliff <= 0 {
			cliff = 12
		}
		if vest <= 0 {
			vest = 48
		}

		grant := model.EquityGrant{
			Ticker:           ticker,
			GrantType:        grantType,
			TotalShares:      shares,
			GrantDate:        grantDate,
			VestingStartDate: vestStart,
			CliffMonths:      cliff,
			VestingMonths:    vest,
			VestingInterval:  interval,
		}

		if fmvStr != "" {
			fmv, err := strconv.ParseFloat(fmvStr, 64)
			if err != nil || fmv <= 0 {
				return nil, "Invalid FMV — enter a number like 25.00"
			}
			fmvCents := int64(math.Round(fmv * 100))
			grant.FMVAtGrant = &fmvCents
		}

		if grantType == "iso" {
			if strikeStr == "" {
				return nil, "Strike price is required for ISOs"
			}
			strike, err := strconv.ParseFloat(strikeStr, 64)
			if err != nil || strike <= 0 {
				return nil, "Invalid strike price"
			}
			strikeCents := int64(math.Round(strike * 100))
			grant.StrikePrice = &strikeCents
		}

		_, err = gs.CreateGrant(grant)
		if err != nil {
			return nil, fmt.Sprintf("Error: %v", err)
		}

		return func() tea.Msg {
			summary, _ := gs.ListGrants()
			return portfolioDataMsg{nil, summary}
		}, ""
	})
}

type vestScheduleRefreshMsg struct {
	events []model.VestEvent
}

func (p *PortfolioModel) newExerciseForm(event model.VestEvent) FormModel {
	gs := p.grants
	grantID := p.vestGrantID
	return NewForm(fmt.Sprintf("Exercise ISO — Event #%d (%.2f shares)", event.ID, event.Shares), []FormField{
		{Label: "FMV/Share", Placeholder: "e.g. 45.00 (current fair market value)"},
	}, func(fields []FormField) (tea.Cmd, string) {
		fmvStr := strings.TrimSpace(fields[0].Value)
		if fmvStr == "" {
			return nil, "FMV per share is required"
		}
		fmv, err := strconv.ParseFloat(fmvStr, 64)
		if err != nil || fmv <= 0 {
			return nil, "Invalid FMV — enter a number like 45.00"
		}
		fmvCents := int64(math.Round(fmv * 100))
		_, err = gs.ExerciseISO(event.ID, fmvCents)
		if err != nil {
			return nil, fmt.Sprintf("Error: %v", err)
		}
		return func() tea.Msg {
			events, _ := gs.GetVestSchedule(grantID)
			return vestScheduleRefreshMsg{events}
		}, ""
	})
}

// --- Views ---

func (p PortfolioModel) View() string {
	if p.form.Active() {
		return p.form.View()
	}

	var sb strings.Builder

	// Sub-view tabs
	tabs := []struct {
		name string
		view PortfolioSubView
	}{
		{"Positions", PortfolioPositions},
		{"Grants", PortfolioGrants},
	}
	if p.subView == PortfolioVestSchedule {
		tabs = append(tabs, struct {
			name string
			view PortfolioSubView
		}{"Vest Schedule", PortfolioVestSchedule})
	}
	for _, t := range tabs {
		if t.view == p.subView {
			sb.WriteString(activeTabStyle.Render(t.name) + " ")
		} else {
			sb.WriteString(inactiveTabStyle.Render(t.name) + " ")
		}
	}

	if p.refreshing {
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(warning).Render("refreshing prices..."))
	}
	sb.WriteString("\n\n")

	switch p.subView {
	case PortfolioPositions:
		sb.WriteString(p.viewPositions())
	case PortfolioGrants:
		sb.WriteString(p.viewGrants())
	case PortfolioVestSchedule:
		sb.WriteString(p.viewVestSchedule())
	}

	if p.statusMsg != "" {
		sb.WriteString("\n  " + lipgloss.NewStyle().Foreground(special).Render(p.statusMsg))
	}

	return sb.String()
}

func (p PortfolioModel) viewPositions() string {
	var sb strings.Builder

	// Show active filter
	if p.filterAccountID != nil && p.filterIdx > 0 && p.filterIdx <= len(p.investAccounts) {
		sb.WriteString(lipgloss.NewStyle().Foreground(highlight).Render(
			fmt.Sprintf("  Filter: %s", p.investAccounts[p.filterIdx-1].Name)) + "\n")
	}

	if p.summary == nil || len(p.summary.Positions) == 0 {
		sb.WriteString("  No positions. Press 'a' to buy stock.\n")
		return sb.String()
	}

	// Header summary
	gainSign := "+"
	gainStyle := greenStyle
	if p.summary.TotalGainLoss < 0 {
		gainSign = ""
		gainStyle = redStyle
	}
	var pct float64
	if p.summary.TotalCostBasis > 0 {
		pct = float64(p.summary.TotalGainLoss) / float64(p.summary.TotalCostBasis) * 100
	}
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Portfolio — $%.2f", float64(p.summary.TotalValue)/100)))
	sb.WriteString("  " + gainStyle.Render(fmt.Sprintf("(%s$%.2f / %s%.1f%%)",
		gainSign, float64(p.summary.TotalGainLoss)/100, gainSign, pct)))
	sb.WriteString("\n\n")

	// Positions table
	hdr := fmt.Sprintf("  %-8s  %10s  %10s  %10s  %12s  %14s",
		"TICKER", "SHARES", "AVG COST", "PRICE", "VALUE", "GAIN/LOSS")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	for i, pos := range p.summary.Positions {
		glSign := "+"
		glStyle := greenStyle
		if pos.GainLoss < 0 {
			glSign = ""
			glStyle = redStyle
		}
		line := fmt.Sprintf("  %-8s  %10.4f  $%9.2f  $%9.2f  $%11.2f  %s",
			pos.Ticker, pos.TotalShares,
			float64(pos.AvgCostBasis)/100, float64(pos.CurrentPrice)/100,
			float64(pos.MarketValue)/100,
			glStyle.Render(fmt.Sprintf("%s$%.2f (%.1f%%)", glSign, float64(pos.GainLoss)/100, pos.GainPct)))

		if i == p.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")

		// show lots if selected
		if i == p.cursor && len(pos.Lots) > 1 {
			for _, lot := range pos.Lots {
				sb.WriteString(fmt.Sprintf("    └ lot#%d  %.4f sh  $%.2f basis  %s  %s\n",
					lot.ID, lot.Shares, float64(lot.CostBasis)/100, lot.DateAcquired, lot.Source))
			}
		}
	}

	return sb.String()
}

func (p PortfolioModel) viewGrants() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render("ISO & RSU Grants") + "\n\n")

	if len(p.grantList) == 0 {
		sb.WriteString("  No grants. Press 'a' to add an ISO or RSU grant.\n")
		return sb.String()
	}

	barWidth := p.width - 60
	if barWidth < 15 {
		barWidth = 15
	}
	if barWidth > 50 {
		barWidth = 50
	}

	for i, g := range p.grantList {
		typeStr := strings.ToUpper(g.GrantType)
		details := ""
		if g.StrikePrice != nil {
			details += fmt.Sprintf("  strike $%.2f", float64(*g.StrikePrice)/100)
		}
		if g.FMVAtGrant != nil {
			details += fmt.Sprintf("  FMV $%.2f", float64(*g.FMVAtGrant)/100)
		}
		if g.VestingStartDate != "" && g.VestingStartDate != g.GrantDate {
			details += fmt.Sprintf("  vest from %s", g.VestingStartDate)
		}

		line := fmt.Sprintf("  %s %s  %.0f shares%s  (granted %s)",
			g.Ticker, typeStr, g.TotalShares, details, g.GrantDate)

		if i == p.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")

		// vesting progress bar
		pct := 0.0
		if g.TotalShares > 0 {
			pct = g.VestedShares / g.TotalShares * 100
		}
		filled := int(pct / 100 * float64(barWidth))
		empty := barWidth - filled

		var barStyle lipgloss.Style
		if pct >= 100 {
			barStyle = greenStyle
		} else {
			barStyle = lipgloss.NewStyle().Foreground(highlight)
		}
		bar := barStyle.Render(strings.Repeat("█", filled)) + strings.Repeat("░", empty)

		vestInfo := fmt.Sprintf("  vested: %.0f/%.0f", g.VestedShares, g.TotalShares)
		if g.NextVestDate != "" {
			vestInfo += fmt.Sprintf("  next: %s", g.NextVestDate)
		}

		sb.WriteString(fmt.Sprintf("    %s  %.1f%%  %s\n", bar, pct, vestInfo))
	}

	return sb.String()
}

func (p PortfolioModel) viewVestSchedule() string {
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Vest Schedule — Grant #%d", p.vestGrantID)) + "\n\n")

	if len(p.vestEvents) == 0 {
		sb.WriteString("  No vest events.\n")
		return sb.String()
	}

	hdr := fmt.Sprintf("  %-4s  %-12s  %12s  %12s  %-10s",
		"#", "DATE", "SHARES", "CUM", "STATUS")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(muted).Render(hdr) + "\n")

	var cumulative float64
	for i, e := range p.vestEvents {
		cumulative += e.Shares

		statusStyle := lipgloss.NewStyle().Foreground(muted)
		switch e.Status {
		case "vested":
			statusStyle = greenStyle
		case "exercised":
			statusStyle = lipgloss.NewStyle().Foreground(highlight)
		case "pending":
			statusStyle = yellowStyle
		}

		fmvStr := ""
		if e.FMVPerShare != nil {
			fmvStr = fmt.Sprintf("  FMV $%.2f", float64(*e.FMVPerShare)/100)
		}

		line := fmt.Sprintf("  %-4d  %-12s  %12.2f  %12.0f  %s%s",
			e.ID, e.Date, e.Shares, cumulative,
			statusStyle.Render(e.Status), fmvStr)

		if i == p.cursor {
			line = selectedRowStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}

	return sb.String()
}
