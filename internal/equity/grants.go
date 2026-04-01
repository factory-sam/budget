package equity

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/sam/budget/internal/model"
)

const grantTypeISO = "iso"

type GrantService struct {
	db        *sql.DB
	prices    *PriceService
	portfolio *PortfolioService
}

func NewGrantService(db *sql.DB, prices *PriceService, portfolio *PortfolioService) *GrantService {
	return &GrantService{db: db, prices: prices, portfolio: portfolio}
}

func (g *GrantService) CreateGrant(grant model.EquityGrant) (*model.EquityGrant, error) {
	// Default vesting start date to grant date if not specified
	if grant.VestingStartDate == "" {
		grant.VestingStartDate = grant.GrantDate
	}
	res, err := g.db.Exec(`
		INSERT INTO equity_grants (ticker, grant_type, total_shares, grant_date, vesting_start_date, fmv_at_grant,
			strike_price, expiration_date, cliff_months, vesting_months, vesting_interval, note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		grant.Ticker, grant.GrantType, grant.TotalShares, grant.GrantDate,
		grant.VestingStartDate, grant.FMVAtGrant,
		grant.StrikePrice, grant.ExpirationDate,
		grant.CliffMonths, grant.VestingMonths, grant.VestingInterval, grant.Note)
	if err != nil {
		return nil, err
	}
	grant.ID, _ = res.LastInsertId()

	// generate vest events
	if err := g.generateVestEvents(&grant); err != nil {
		return nil, fmt.Errorf("generate vest events: %w", err)
	}

	return &grant, nil
}

func (g *GrantService) generateVestEvents(grant *model.EquityGrant) error {
	vestStart := grant.VestingStartDate
	if vestStart == "" {
		vestStart = grant.GrantDate
	}
	grantDate, _ := time.Parse("2006-01-02", vestStart)
	totalMonths := grant.VestingMonths
	cliffMonths := grant.CliffMonths

	intervalMonths := 1
	if grant.VestingInterval == "quarterly" {
		intervalMonths = 3
	}

	// calculate number of vesting periods after cliff
	postCliffMonths := totalMonths - cliffMonths
	if postCliffMonths < 0 {
		postCliffMonths = 0
	}
	postCliffPeriods := postCliffMonths / intervalMonths
	if postCliffPeriods < 0 {
		postCliffPeriods = 0
	}

	totalPeriods := postCliffPeriods + 1 // +1 for the cliff vest
	sharesPerPeriod := grant.TotalShares / float64(totalPeriods)

	tx, err := g.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// cliff vest
	cliffDate := grantDate.AddDate(0, cliffMonths, 0)
	cliffShares := sharesPerPeriod
	// if only cliff, give all shares
	if postCliffPeriods == 0 {
		cliffShares = grant.TotalShares
	}

	if _, err := tx.Exec("INSERT INTO equity_vest_events (grant_id, date, shares, status) VALUES (?, ?, ?, 'pending')",
		grant.ID, cliffDate.Format("2006-01-02"), cliffShares); err != nil {
		return err
	}

	// post-cliff vests
	remaining := grant.TotalShares - cliffShares
	if postCliffPeriods > 0 && remaining > 0 {
		perPeriod := remaining / float64(postCliffPeriods)
		for i := 1; i <= postCliffPeriods; i++ {
			vestDate := cliffDate.AddDate(0, i*intervalMonths, 0)
			shares := perPeriod
			// last period gets remainder to avoid rounding issues
			if i == postCliffPeriods {
				sumSoFar := cliffShares + perPeriod*float64(i-1)
				shares = grant.TotalShares - sumSoFar
			}
			if _, err := tx.Exec("INSERT INTO equity_vest_events (grant_id, date, shares, status) VALUES (?, ?, ?, 'pending')",
				grant.ID, vestDate.Format("2006-01-02"), shares); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

const grantCols = `id, ticker, grant_type, total_shares, grant_date, vesting_start_date, fmv_at_grant,
	strike_price, expiration_date, cliff_months, vesting_months, vesting_interval, note`

func scanGrant(scanner interface{ Scan(...interface{}) error }) (*model.EquityGrant, error) {
	var gr model.EquityGrant
	err := scanner.Scan(&gr.ID, &gr.Ticker, &gr.GrantType, &gr.TotalShares, &gr.GrantDate,
		&gr.VestingStartDate, &gr.FMVAtGrant,
		&gr.StrikePrice, &gr.ExpirationDate,
		&gr.CliffMonths, &gr.VestingMonths, &gr.VestingInterval, &gr.Note)
	if err != nil {
		return nil, err
	}
	// backfill vesting start if empty
	if gr.VestingStartDate == "" {
		gr.VestingStartDate = gr.GrantDate
	}
	return &gr, nil
}

func (g *GrantService) ListGrants() ([]model.EquityGrant, error) {
	rows, err := g.db.Query(`SELECT ` + grantCols + ` FROM equity_grants ORDER BY grant_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []model.EquityGrant
	for rows.Next() {
		gr, err := scanGrant(rows)
		if err != nil {
			continue
		}

		gr.VestedShares, gr.UnvestedShares, gr.NextVestDate = g.computeVestingStatus(gr.ID, gr.TotalShares)
		grants = append(grants, *gr)
	}
	return grants, nil
}

func (g *GrantService) GetGrant(id int64) (*model.EquityGrant, error) {
	gr, err := scanGrant(g.db.QueryRow(`SELECT `+grantCols+` FROM equity_grants WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	gr.VestedShares, gr.UnvestedShares, gr.NextVestDate = g.computeVestingStatus(gr.ID, gr.TotalShares)
	return gr, nil
}

func (g *GrantService) computeVestingStatus(grantID int64, totalShares float64) (vested, unvested float64, nextVest string) {
	today := time.Now().Format("2006-01-02")

	// sum vested/exercised shares
	_ = g.db.QueryRow("SELECT COALESCE(SUM(shares), 0) FROM equity_vest_events WHERE grant_id = ? AND status IN ('vested', 'exercised') ",
		grantID).Scan(&vested)
	unvested = totalShares - vested

	// next pending vest date
	_ = g.db.QueryRow("SELECT date FROM equity_vest_events WHERE grant_id = ? AND status = 'pending' AND date >= ? ORDER BY date LIMIT 1",
		grantID, today).Scan(&nextVest)
	return
}

func (g *GrantService) GetVestSchedule(grantID int64) ([]model.VestEvent, error) {
	rows, err := g.db.Query(
		"SELECT id, grant_id, date, shares, fmv_per_share, status, lot_id FROM equity_vest_events WHERE grant_id = ? ORDER BY date",
		grantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.VestEvent
	for rows.Next() {
		var e model.VestEvent
		if err := rows.Scan(&e.ID, &e.GrantID, &e.Date, &e.Shares, &e.FMVPerShare, &e.Status, &e.LotID); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}

// VestGrant processes all pending vest events for a grant up to today
func (g *GrantService) VestGrant(grantID int64, fmvOverride *int64) (int, error) {
	grant, err := g.GetGrant(grantID)
	if err != nil {
		return 0, fmt.Errorf("grant %d not found: %w", grantID, err)
	}

	today := time.Now().Format("2006-01-02")
	rows, err := g.db.Query(
		"SELECT id, date, shares FROM equity_vest_events WHERE grant_id = ? AND status = 'pending' AND date <= ? ORDER BY date",
		grantID, today)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var events []struct {
		id     int64
		date   string
		shares float64
	}
	for rows.Next() {
		var e struct {
			id     int64
			date   string
			shares float64
		}
		if err := rows.Scan(&e.id, &e.date, &e.shares); err != nil {
			continue
		}
		events = append(events, e)
	}
	rows.Close()

	count := 0
	for _, e := range events {
		var fmv int64
		if fmvOverride != nil {
			fmv = *fmvOverride
		} else {
			fmv, _ = g.prices.FetchPrice(grant.Ticker)
		}

		if grant.GrantType == "rsu" {
			// RSU: auto-create lot at FMV
			costBasis := int64(math.Round(float64(fmv) * e.shares))
			lot, err := g.portfolio.CreateLot(nil, grant.Ticker, e.shares, costBasis, e.date, "rsu_vest", &grantID, "")
			if err != nil {
				return count, err
			}
			if _, err := g.db.Exec("UPDATE equity_vest_events SET status = 'vested', fmv_per_share = ?, lot_id = ? WHERE id = ?",
				fmv, lot.ID, e.id); err != nil {
				return count, err
			}
		} else {
			// ISO: mark as vested (exercisable) but don't create lot yet
			if _, err := g.db.Exec("UPDATE equity_vest_events SET status = 'vested', fmv_per_share = ? WHERE id = ?",
				fmv, e.id); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

// ExerciseISO exercises a vested ISO vest event
func (g *GrantService) ExerciseISO(vestEventID int64, fmvAtExerciseCents int64) (*model.EquityLot, error) {
	var e model.VestEvent
	var grantID int64
	err := g.db.QueryRow(
		"SELECT id, grant_id, date, shares, status FROM equity_vest_events WHERE id = ?", vestEventID).
		Scan(&e.ID, &grantID, &e.Date, &e.Shares, &e.Status)
	if err != nil {
		return nil, fmt.Errorf("vest event %d not found", vestEventID)
	}
	if e.Status != "vested" {
		return nil, fmt.Errorf("vest event %d is %s, not vested (exercisable)", vestEventID, e.Status)
	}

	grant, err := g.GetGrant(grantID)
	if err != nil {
		return nil, err
	}
	if grant.GrantType != grantTypeISO {
		return nil, fmt.Errorf("grant %d is %s, not ISO", grantID, grant.GrantType)
	}
	if grant.StrikePrice == nil {
		return nil, fmt.Errorf("grant %d has no strike price", grantID)
	}

	// cost basis = strike price * shares
	costBasis := int64(math.Round(float64(*grant.StrikePrice) * e.Shares))
	today := time.Now().Format("2006-01-02")
	note := fmt.Sprintf("ISO exercise, FMV at exercise: $%.2f, AMT spread: $%.2f",
		float64(fmvAtExerciseCents)/100,
		float64(fmvAtExerciseCents-*grant.StrikePrice)*e.Shares/100)

	lot, err := g.portfolio.CreateLot(nil, grant.Ticker, e.Shares, costBasis, today, "iso_exercise", &grantID, note)
	if err != nil {
		return nil, err
	}

	if _, err := g.db.Exec("UPDATE equity_vest_events SET status = 'exercised', fmv_per_share = ?, lot_id = ? WHERE id = ?",
		fmvAtExerciseCents, lot.ID, vestEventID); err != nil {
		return nil, err
	}

	return lot, nil
}

func (g *GrantService) DeleteGrant(id int64) error {
	if _, err := g.db.Exec("DELETE FROM equity_vest_events WHERE grant_id = ?", id); err != nil {
		return err
	}
	_, err := g.db.Exec("DELETE FROM equity_grants WHERE id = ?", id)
	return err
}
