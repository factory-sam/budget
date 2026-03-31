package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/model"
)

type PortfolioSubView int

const (
	PortfolioPositions PortfolioSubView = iota
	PortfolioGrants
)

type PortfolioModel struct {
	prices    *equity.PriceService
	portfolio *equity.PortfolioService
	grants    *equity.GrantService

	width, height int
	summary       *model.PortfolioSummary
	grantList     []model.EquityGrant
	cursor        int
	subView       PortfolioSubView
	refreshing    bool
}

func NewPortfolioModel(prices *equity.PriceService, portfolio *equity.PortfolioService, grants *equity.GrantService) PortfolioModel {
	return PortfolioModel{
		prices:    prices,
		portfolio: portfolio,
		grants:    grants,
	}
}

type portfolioDataMsg struct {
	summary *model.PortfolioSummary
	grants  []model.EquityGrant
}

type priceRefreshDoneMsg struct{}

func (p PortfolioModel) Init() tea.Cmd {
	return func() tea.Msg {
		summary, _ := p.portfolio.GetPortfolio()
		grants, _ := p.grants.ListGrants()
		return portfolioDataMsg{summary, grants}
	}
}

func (p PortfolioModel) Update(msg tea.Msg) (PortfolioModel, tea.Cmd) {
	switch msg := msg.(type) {
	case portfolioDataMsg:
		p.summary = msg.summary
		p.grantList = msg.grants
		p.cursor = 0
		p.refreshing = false

	case priceRefreshDoneMsg:
		p.refreshing = false
		return p, p.Init()

	case tea.KeyMsg:
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
				// go to top
				p.cursor = 0
			}
		case "G":
			maxLen := p.currentListLen()
			if maxLen > 0 {
				p.cursor = maxLen - 1
			}
		case "r":
			p.refreshing = true
			return p, func() tea.Msg {
				tickers, _ := p.portfolio.GetDistinctTickers()
				p.prices.FetchPrices(tickers)
				return priceRefreshDoneMsg{}
			}
		}
	}
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
	}
	return 0
}

func (p PortfolioModel) View() string {
	var sb strings.Builder

	// Sub-view tabs
	posLabel := "Positions"
	grantLabel := "Grants"
	if p.subView == PortfolioPositions {
		posLabel = activeTabStyle.Render(posLabel)
		grantLabel = inactiveTabStyle.Render(grantLabel)
	} else {
		posLabel = inactiveTabStyle.Render(posLabel)
		grantLabel = activeTabStyle.Render(grantLabel)
	}
	sb.WriteString(posLabel + " " + grantLabel)

	if p.refreshing {
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(warning).Render("refreshing prices..."))
	}
	sb.WriteString("\n\n")

	switch p.subView {
	case PortfolioPositions:
		sb.WriteString(p.viewPositions())
	case PortfolioGrants:
		sb.WriteString(p.viewGrants())
	}

	sb.WriteString("\n  j/k:navigate  g:grants  p:positions  r:refresh prices  G:bottom")
	return sb.String()
}

func (p PortfolioModel) viewPositions() string {
	var sb strings.Builder

	if p.summary == nil || len(p.summary.Positions) == 0 {
		sb.WriteString("  No positions. Buy stocks with: budget equity buy --ticker AAPL --shares 10 --price 185.50\n")
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
		sb.WriteString("  No grants. Add one with:\n")
		sb.WriteString("  budget equity grant add --ticker ACME --type iso --shares 10000 --strike 12.50 --date 2024-06-01\n")
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
		strikeStr := ""
		if g.StrikePrice != nil {
			strikeStr = fmt.Sprintf("  strike $%.2f", float64(*g.StrikePrice)/100)
		}

		line := fmt.Sprintf("  %s %s  %.0f shares%s",
			g.Ticker, typeStr, g.TotalShares, strikeStr)

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
