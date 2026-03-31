package equity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/sam/budget/internal/model"
)

type PriceService struct {
	db     *sql.DB
	client *http.Client
}

func NewPriceService(db *sql.DB) *PriceService {
	return &PriceService{
		db:     db,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *PriceService) FetchPrice(ticker string) (int64, error) {
	today := time.Now().Format("2006-01-02")

	// check cache first
	var cached int64
	err := p.db.QueryRow("SELECT price FROM equity_prices WHERE ticker = ? AND date = ?", ticker, today).Scan(&cached)
	if err == nil {
		return cached, nil
	}

	// fetch from Yahoo Finance
	price, err := p.fetchYahoo(ticker)
	if err != nil {
		// fallback to last cached price
		err2 := p.db.QueryRow("SELECT price FROM equity_prices WHERE ticker = ? ORDER BY date DESC LIMIT 1", ticker).Scan(&cached)
		if err2 == nil {
			return cached, nil
		}
		return 0, fmt.Errorf("fetch price for %s: %w", ticker, err)
	}

	// cache it
	p.db.Exec("INSERT OR REPLACE INTO equity_prices (ticker, date, price) VALUES (?, ?, ?)", ticker, today, price)
	return price, nil
}

func (p *PriceService) FetchPrices(tickers []string) (map[string]int64, error) {
	prices := make(map[string]int64)
	for _, t := range tickers {
		price, err := p.FetchPrice(t)
		if err != nil {
			continue
		}
		prices[t] = price
	}
	return prices, nil
}

func (p *PriceService) GetCachedPrice(ticker string) (int64, error) {
	var price int64
	err := p.db.QueryRow("SELECT price FROM equity_prices WHERE ticker = ? ORDER BY date DESC LIMIT 1", ticker).Scan(&price)
	return price, err
}

func (p *PriceService) GetHistoricalPrices(ticker string, limit int) ([]model.EquityPrice, error) {
	rows, err := p.db.Query(
		"SELECT id, ticker, date, price FROM equity_prices WHERE ticker = ? ORDER BY date DESC LIMIT ?",
		ticker, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prices []model.EquityPrice
	for rows.Next() {
		var ep model.EquityPrice
		rows.Scan(&ep.ID, &ep.Ticker, &ep.Date, &ep.Price)
		prices = append(prices, ep)
	}
	return prices, nil
}

type yahooResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (p *PriceService) fetchYahoo(ticker string) (int64, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", ticker)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("yahoo returned status %d for %s", resp.StatusCode, ticker)
	}

	var yr yahooResponse
	if err := json.NewDecoder(resp.Body).Decode(&yr); err != nil {
		return 0, err
	}

	if yr.Chart.Error != nil {
		return 0, fmt.Errorf("yahoo error: %s", yr.Chart.Error.Description)
	}

	if len(yr.Chart.Result) == 0 {
		return 0, fmt.Errorf("no results for ticker %s", ticker)
	}

	price := yr.Chart.Result[0].Meta.RegularMarketPrice
	if price <= 0 {
		return 0, fmt.Errorf("invalid price for %s: %.2f", ticker, price)
	}

	cents := int64(math.Round(price * 100))
	return cents, nil
}

// SetManualPrice lets users override a price for a given date
func (p *PriceService) SetManualPrice(ticker, date string, priceCents int64) error {
	_, err := p.db.Exec("INSERT OR REPLACE INTO equity_prices (ticker, date, price) VALUES (?, ?, ?)",
		ticker, date, priceCents)
	return err
}
