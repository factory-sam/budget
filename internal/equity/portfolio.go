package equity

import (
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/sam/budget/internal/model"
)

type PortfolioService struct {
	db     *sql.DB
	prices *PriceService
}

func NewPortfolioService(db *sql.DB, prices *PriceService) *PortfolioService {
	return &PortfolioService{db: db, prices: prices}
}

// --- Lot CRUD ---

func (p *PortfolioService) BuyLot(ticker string, shares float64, pricePerShareCents int64, date, note string) (*model.EquityLot, error) {
	costBasis := int64(math.Round(float64(pricePerShareCents) * shares))
	res, err := p.db.Exec(
		"INSERT INTO equity_lots (ticker, shares, cost_basis, date_acquired, source, note) VALUES (?, ?, ?, ?, 'buy', ?)",
		ticker, shares, costBasis, date, note)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.EquityLot{
		ID: id, Ticker: ticker, Shares: shares, CostBasis: costBasis,
		DateAcquired: date, Source: "buy", Note: note, IncludeInNetWorth: true,
	}, nil
}

func (p *PortfolioService) CreateLot(ticker string, shares float64, costBasis int64, date, source string, grantID *int64, note string) (*model.EquityLot, error) {
	res, err := p.db.Exec(
		"INSERT INTO equity_lots (ticker, shares, cost_basis, date_acquired, source, grant_id, note) VALUES (?, ?, ?, ?, ?, ?, ?)",
		ticker, shares, costBasis, date, source, grantID, note)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.EquityLot{
		ID: id, Ticker: ticker, Shares: shares, CostBasis: costBasis,
		DateAcquired: date, Source: source, GrantID: grantID, Note: note, IncludeInNetWorth: true,
	}, nil
}

func (p *PortfolioService) SellLot(lotID int64, sharesToSell float64) error {
	var current float64
	err := p.db.QueryRow("SELECT shares FROM equity_lots WHERE id = ?", lotID).Scan(&current)
	if err != nil {
		return fmt.Errorf("lot %d not found", lotID)
	}
	if sharesToSell >= current {
		_, err = p.db.Exec("DELETE FROM equity_lots WHERE id = ?", lotID)
	} else {
		// reduce shares and proportionally reduce cost basis
		ratio := (current - sharesToSell) / current
		_, err = p.db.Exec("UPDATE equity_lots SET shares = ?, cost_basis = CAST(cost_basis * ? AS INTEGER) WHERE id = ?",
			current-sharesToSell, ratio, lotID)
	}
	return err
}

func (p *PortfolioService) ListLots(ticker string) ([]model.EquityLot, error) {
	q := "SELECT id, ticker, shares, cost_basis, date_acquired, source, grant_id, include_in_networth, note FROM equity_lots"
	var args []interface{}
	if ticker != "" {
		q += " WHERE UPPER(ticker) = UPPER(?)"
		args = append(args, ticker)
	}
	q += " ORDER BY ticker, date_acquired"

	rows, err := p.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lots []model.EquityLot
	for rows.Next() {
		var l model.EquityLot
		var inclNW int
		rows.Scan(&l.ID, &l.Ticker, &l.Shares, &l.CostBasis, &l.DateAcquired, &l.Source, &l.GrantID, &inclNW, &l.Note)
		l.IncludeInNetWorth = inclNW == 1

		// enrich with current price
		price, err := p.prices.GetCachedPrice(l.Ticker)
		if err == nil && price > 0 {
			l.CurrentPrice = price
			l.MarketValue = int64(math.Round(float64(price) * l.Shares))
			l.GainLoss = l.MarketValue - l.CostBasis
			if l.CostBasis > 0 {
				l.GainPct = float64(l.GainLoss) / float64(l.CostBasis) * 100
			}
		}
		lots = append(lots, l)
	}
	return lots, nil
}

func (p *PortfolioService) SetNetWorthInclusion(lotID int64, include bool) error {
	v := 0
	if include {
		v = 1
	}
	_, err := p.db.Exec("UPDATE equity_lots SET include_in_networth = ? WHERE id = ?", v, lotID)
	return err
}

// --- Portfolio Aggregation ---

func (p *PortfolioService) GetPortfolio() (*model.PortfolioSummary, error) {
	lots, err := p.ListLots("")
	if err != nil {
		return nil, err
	}

	// group by ticker
	posMap := make(map[string]*model.PositionSummary)
	for _, l := range lots {
		pos, ok := posMap[l.Ticker]
		if !ok {
			pos = &model.PositionSummary{Ticker: l.Ticker}
			posMap[l.Ticker] = pos
		}
		pos.TotalShares += l.Shares
		pos.MarketValue += l.MarketValue
		pos.GainLoss += l.GainLoss
		pos.Lots = append(pos.Lots, l)
		pos.CurrentPrice = l.CurrentPrice // same for all lots of same ticker
	}

	summary := &model.PortfolioSummary{}
	for _, pos := range posMap {
		// compute avg cost basis
		var totalCost int64
		for _, l := range pos.Lots {
			totalCost += l.CostBasis
		}
		if pos.TotalShares > 0 {
			pos.AvgCostBasis = int64(math.Round(float64(totalCost) / pos.TotalShares))
		}
		if totalCost > 0 {
			pos.GainPct = float64(pos.GainLoss) / float64(totalCost) * 100
		}
		summary.TotalValue += pos.MarketValue
		summary.TotalCostBasis += totalCost
		summary.Positions = append(summary.Positions, *pos)
	}
	summary.TotalGainLoss = summary.TotalValue - summary.TotalCostBasis

	sort.Slice(summary.Positions, func(i, j int) bool {
		return summary.Positions[i].MarketValue > summary.Positions[j].MarketValue
	})
	return summary, nil
}

// EquityValueForNetWorth returns the total market value of lots opted into net worth
func (p *PortfolioService) EquityValueForNetWorth() (int64, error) {
	rows, err := p.db.Query("SELECT ticker, shares FROM equity_lots WHERE include_in_networth = 1")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total int64
	for rows.Next() {
		var ticker string
		var shares float64
		rows.Scan(&ticker, &shares)
		price, err := p.prices.GetCachedPrice(ticker)
		if err != nil {
			continue
		}
		total += int64(math.Round(float64(price) * shares))
	}
	return total, nil
}

// GetDistinctTickers returns all unique tickers in the portfolio
func (p *PortfolioService) GetDistinctTickers() ([]string, error) {
	rows, err := p.db.Query("SELECT DISTINCT ticker FROM equity_lots ORDER BY ticker")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tickers []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		tickers = append(tickers, t)
	}
	return tickers, nil
}
