package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sam/budget/internal/equity"
	"github.com/sam/budget/internal/importer"
	"github.com/sam/budget/internal/model"
	"github.com/spf13/cobra"
)

var (
	pricesSvc    *equity.PriceService
	portfolioSvc *equity.PortfolioService
	grantSvc     *equity.GrantService
)

func initEquityServices() {
	if pricesSvc == nil {
		pricesSvc = equity.NewPriceService(database)
		portfolioSvc = equity.NewPortfolioService(database, pricesSvc)
		grantSvc = equity.NewGrantService(database, pricesSvc, portfolioSvc)
	}
}

var equityCmd = &cobra.Command{
	Use:   "equity",
	Short: "Manage equity portfolio, ISOs, and RSUs",
}

// --- buy ---

var equityBuyCmd = &cobra.Command{
	Use:   "buy",
	Short: "Record a stock purchase",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		ticker, _ := cmd.Flags().GetString("ticker")
		shares, _ := cmd.Flags().GetFloat64("shares")
		price, _ := cmd.Flags().GetFloat64("price")
		date, _ := cmd.Flags().GetString("date")
		note, _ := cmd.Flags().GetString("note")

		accountName, _ := cmd.Flags().GetString("account")

		if ticker == "" || shares == 0 || price == 0 {
			return fmt.Errorf("--ticker, --shares, and --price are required")
		}
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		ticker = strings.ToUpper(ticker)
		priceCents := int64(math.Round(price * 100))

		var accountID *int64
		if accountName != "" {
			acc, err := service.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("account not found: %s", accountName)
			}
			accountID = &acc.ID
		}

		lot, err := portfolioSvc.BuyLot(accountID, ticker, shares, priceCents, date, note)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(lot)
		}
		fmt.Printf("Bought %.4f shares of %s at $%.2f (lot #%d, cost basis: $%.2f)\n",
			shares, ticker, price, lot.ID, float64(lot.CostBasis)/100)
		return nil
	},
}

// --- sell ---

var equitySellCmd = &cobra.Command{
	Use:   "sell",
	Short: "Sell shares from a specific lot",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		lotID, _ := cmd.Flags().GetInt64("lot")
		shares, _ := cmd.Flags().GetFloat64("shares")

		if lotID == 0 || shares == 0 {
			return fmt.Errorf("--lot and --shares are required")
		}
		if err := portfolioSvc.SellLot(lotID, shares); err != nil {
			return err
		}
		fmt.Printf("Sold %.4f shares from lot #%d\n", shares, lotID)
		return nil
	},
}

// --- lots ---

var equityLotsCmd = &cobra.Command{
	Use:   "lots",
	Short: "List all equity lots",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		ticker, _ := cmd.Flags().GetString("ticker")
		ticker = strings.ToUpper(ticker)
		accountName, _ := cmd.Flags().GetString("account")

		var accountID *int64
		if accountName != "" {
			acc, err := service.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("account not found: %s", accountName)
			}
			accountID = &acc.ID
		}

		lots, err := portfolioSvc.ListLots(ticker, accountID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(lots)
		}

		if len(lots) == 0 {
			fmt.Println("No equity lots.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTICKER\tSHARES\tCOST BASIS\tACQUIRED\tSOURCE\tACCOUNT\tVALUE\tGAIN/LOSS\tNW")
		for _, l := range lots {
			nw := " "
			if l.IncludeInNetWorth {
				nw = "*"
			}
			accName := l.AccountName
			if accName == "" {
				accName = emDash
			}
			fmt.Fprintf(w, "%d\t%s\t%.4f\t$%.2f\t%s\t%s\t%s\t$%.2f\t%+.2f (%.1f%%)\t%s\n",
				l.ID, l.Ticker, l.Shares, float64(l.CostBasis)/100,
				l.DateAcquired, l.Source, accName,
				float64(l.MarketValue)/100, float64(l.GainLoss)/100, l.GainPct, nw)
		}
		return w.Flush()
	},
}

// --- portfolio ---

var equityPortfolioCmd = &cobra.Command{
	Use:   "portfolio",
	Short: "Show portfolio summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		accountName, _ := cmd.Flags().GetString("account")

		var accountID *int64
		if accountName != "" {
			acc, err := service.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("account not found: %s", accountName)
			}
			accountID = &acc.ID
		}

		// refresh prices first
		tickers, _ := portfolioSvc.GetDistinctTickers()
		if _, err := pricesSvc.FetchPrices(tickers); err != nil {
			return fmt.Errorf("fetching prices: %w", err)
		}

		portfolio, err := portfolioSvc.GetPortfolio(accountID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(portfolio)
		}

		gainStyle := "+"
		if portfolio.TotalGainLoss < 0 {
			gainStyle = ""
		}
		var pct float64
		if portfolio.TotalCostBasis > 0 {
			pct = float64(portfolio.TotalGainLoss) / float64(portfolio.TotalCostBasis) * 100
		}
		fmt.Printf("Portfolio — $%.2f (%s$%.2f / %s%.1f%%)\n\n",
			float64(portfolio.TotalValue)/100,
			gainStyle, float64(portfolio.TotalGainLoss)/100,
			gainStyle, pct)

		if len(portfolio.Positions) == 0 {
			fmt.Println("No positions.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TICKER\tSHARES\tAVG COST\tPRICE\tVALUE\tGAIN/LOSS")
		for _, p := range portfolio.Positions {
			fmt.Fprintf(w, "%s\t%.4f\t$%.2f\t$%.2f\t$%.2f\t%+.2f (%.1f%%)\n",
				p.Ticker, p.TotalShares,
				float64(p.AvgCostBasis)/100, float64(p.CurrentPrice)/100,
				float64(p.MarketValue)/100, float64(p.GainLoss)/100, p.GainPct)
		}
		return w.Flush()
	},
}

// --- price ---

var equityPriceCmd = &cobra.Command{
	Use:   "price [tickers...]",
	Short: "Fetch and display current stock prices",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		format, _ := cmd.Flags().GetString("format")

		type priceResult struct {
			Ticker string  `json:"ticker"`
			Price  float64 `json:"price"`
		}
		var results []priceResult

		for _, t := range args {
			t = strings.ToUpper(t)
			price, err := pricesSvc.FetchPrice(t)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: error — %v\n", t, err)
				continue
			}
			results = append(results, priceResult{t, float64(price) / 100})
		}

		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		for _, r := range results {
			fmt.Printf("  %s: $%.2f\n", r.Ticker, r.Price)
		}
		return nil
	},
}

// --- grant add ---

var equityGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Manage ISO/RSU grants",
}

var equityGrantAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an ISO or RSU grant",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		ticker, _ := cmd.Flags().GetString("ticker")
		grantType, _ := cmd.Flags().GetString("type")
		shares, _ := cmd.Flags().GetFloat64("shares")
		strike, _ := cmd.Flags().GetFloat64("strike")
		fmv, _ := cmd.Flags().GetFloat64("fmv")
		date, _ := cmd.Flags().GetString("date")
		vestStart, _ := cmd.Flags().GetString("vest-start")
		expDate, _ := cmd.Flags().GetString("expiration")
		cliff, _ := cmd.Flags().GetInt("cliff")
		vest, _ := cmd.Flags().GetInt("vest")
		interval, _ := cmd.Flags().GetString("interval")
		note, _ := cmd.Flags().GetString("note")

		if ticker == "" || shares == 0 || date == "" {
			return fmt.Errorf("--ticker, --shares, and --date are required")
		}
		if grantType != "iso" && grantType != "rsu" {
			return fmt.Errorf("--type must be 'iso' or 'rsu'")
		}
		ticker = strings.ToUpper(ticker)

		grant := model.EquityGrant{
			Ticker:           ticker,
			GrantType:        grantType,
			TotalShares:      shares,
			GrantDate:        date,
			VestingStartDate: vestStart,
			CliffMonths:      cliff,
			VestingMonths:    vest,
			VestingInterval:  interval,
			Note:             note,
		}

		if fmv > 0 {
			fmvCents := int64(math.Round(fmv * 100))
			grant.FMVAtGrant = &fmvCents
		}

		if grantType == "iso" {
			if strike == 0 {
				return fmt.Errorf("--strike is required for ISOs")
			}
			strikeCents := int64(math.Round(strike * 100))
			grant.StrikePrice = &strikeCents
			if expDate != "" {
				grant.ExpirationDate = &expDate
			}
		}

		created, err := grantSvc.CreateGrant(grant)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(created)
		}
		fmt.Printf("Created %s grant #%d: %.0f shares of %s (cliff: %dm, vest: %dm, interval: %s)\n",
			strings.ToUpper(grantType), created.ID, shares, ticker, cliff, vest, interval)
		return nil
	},
}

var equityGrantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all grants",
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		grants, err := grantSvc.ListGrants()
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(grants)
		}

		if len(grants) == 0 {
			fmt.Println("No grants.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTICKER\tTYPE\tTOTAL\tVESTED\tUNVESTED\tSTRIKE\tFMV\tVEST START\tNEXT VEST")
		for _, g := range grants {
			strikeStr := emDash
			if g.StrikePrice != nil {
				strikeStr = fmt.Sprintf("$%.2f", float64(*g.StrikePrice)/100)
			}
			fmvStr := emDash
			if g.FMVAtGrant != nil {
				fmvStr = fmt.Sprintf("$%.2f", float64(*g.FMVAtGrant)/100)
			}
			vestStart := g.VestingStartDate
			if vestStart == "" || vestStart == g.GrantDate {
				vestStart = emDash
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%.0f\t%.0f\t%.0f\t%s\t%s\t%s\t%s\n",
				g.ID, g.Ticker, strings.ToUpper(g.GrantType), g.TotalShares,
				g.VestedShares, g.UnvestedShares, strikeStr, fmvStr, vestStart, g.NextVestDate)
		}
		return w.Flush()
	},
}

var equityGrantDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a grant and its vest events",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ID: %s", args[0])
		}
		if err := grantSvc.DeleteGrant(id); err != nil {
			return err
		}
		fmt.Printf("Deleted grant #%d\n", id)
		return nil
	},
}

// --- vest ---

var equityVestCmd = &cobra.Command{
	Use:   "vest [grant-id]",
	Short: "Process pending vest events for a grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		grantID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid grant ID: %s", args[0])
		}

		var fmvPtr *int64
		fmv, _ := cmd.Flags().GetFloat64("fmv")
		if fmv > 0 {
			cents := int64(math.Round(fmv * 100))
			fmvPtr = &cents
		}

		count, err := grantSvc.VestGrant(grantID, fmvPtr)
		if err != nil {
			return err
		}
		fmt.Printf("Vested %d events for grant #%d\n", count, grantID)
		return nil
	},
}

// --- vest-schedule ---

var equityVestScheduleCmd = &cobra.Command{
	Use:   "vest-schedule [grant-id]",
	Short: "Show vesting schedule for a grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		grantID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid grant ID: %s", args[0])
		}

		grant, err := grantSvc.GetGrant(grantID)
		if err != nil {
			return err
		}

		events, err := grantSvc.GetVestSchedule(grantID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(events)
		}

		fmt.Printf("Vesting Schedule — %s %s #%d (%.0f shares)\n\n",
			grant.Ticker, strings.ToUpper(grant.GrantType), grant.ID, grant.TotalShares)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DATE\tSHARES\tSTATUS\tFMV")
		var cumulative float64
		for _, e := range events {
			cumulative += e.Shares
			fmvStr := emDash
			if e.FMVPerShare != nil {
				fmvStr = fmt.Sprintf("$%.2f", float64(*e.FMVPerShare)/100)
			}
			fmt.Fprintf(w, "%s\t%.4f (cum: %.0f)\t%s\t%s\n",
				e.Date, e.Shares, cumulative, e.Status, fmvStr)
		}
		return w.Flush()
	},
}

// --- exercise ---

var equityExerciseCmd = &cobra.Command{
	Use:   "exercise [vest-event-id]",
	Short: "Exercise a vested ISO",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		eventID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid vest event ID: %s", args[0])
		}

		fmv, _ := cmd.Flags().GetFloat64("fmv")
		if fmv == 0 {
			return fmt.Errorf("--fmv is required (fair market value at exercise)")
		}
		fmvCents := int64(math.Round(fmv * 100))

		lot, err := grantSvc.ExerciseISO(eventID, fmvCents)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(lot)
		}
		fmt.Printf("Exercised ISO vest event #%d → lot #%d (%.4f shares of %s, cost basis: $%.2f)\n",
			eventID, lot.ID, lot.Shares, lot.Ticker, float64(lot.CostBasis)/100)
		return nil
	},
}

// --- include/exclude ---

var equityIncludeCmd = &cobra.Command{
	Use:   "include [lot-id]",
	Short: "Include a lot in net worth calculation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		id, _ := strconv.ParseInt(args[0], 10, 64)
		return portfolioSvc.SetNetWorthInclusion(id, true)
	},
}

var equityExcludeCmd = &cobra.Command{
	Use:   "exclude [lot-id]",
	Short: "Exclude a lot from net worth calculation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		id, _ := strconv.ParseInt(args[0], 10, 64)
		return portfolioSvc.SetNetWorthInclusion(id, false)
	},
}

// --- import ---

var equityImportCmd = &cobra.Command{
	Use:   "import [csv-file]",
	Short: "Import portfolio transactions from a brokerage CSV",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		initEquityServices()
		accountName, _ := cmd.Flags().GetString("account")

		var accountID *int64
		if accountName != "" {
			acc, err := service.GetAccountByName(accountName)
			if err != nil {
				return fmt.Errorf("account not found: %s", accountName)
			}
			accountID = &acc.ID
		}

		imp := importer.NewEquityCSV(service, portfolioSvc)
		result, err := imp.Import(args[0], accountID)
		if err != nil {
			return err
		}

		format, _ := cmd.Flags().GetString("format")
		if format == formatJSON {
			return json.NewEncoder(os.Stdout).Encode(result)
		}

		fmt.Println(result)
		if len(result.Errors) > 0 {
			fmt.Printf("\n%d errors:\n", len(result.Errors))
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}
		return nil
	},
}

func init() {
	equityImportCmd.Flags().String("account", "", "investment account to link lots to")
	equityImportCmd.Flags().String("format", "table", "output format")

	equityBuyCmd.Flags().String("ticker", "", "stock ticker symbol")
	equityBuyCmd.Flags().Float64("shares", 0, "number of shares")
	equityBuyCmd.Flags().Float64("price", 0, "price per share")
	equityBuyCmd.Flags().String("date", "", "purchase date (YYYY-MM-DD)")
	equityBuyCmd.Flags().String("note", "", "optional note")
	equityBuyCmd.Flags().String("account", "", "investment account name")
	equityBuyCmd.Flags().String("format", "table", "output format")

	equitySellCmd.Flags().Int64("lot", 0, "lot ID to sell from")
	equitySellCmd.Flags().Float64("shares", 0, "shares to sell")

	equityLotsCmd.Flags().String("ticker", "", "filter by ticker")
	equityLotsCmd.Flags().String("account", "", "filter by account name")
	equityLotsCmd.Flags().String("format", "table", "output format")

	equityPortfolioCmd.Flags().String("account", "", "filter by account name")
	equityPortfolioCmd.Flags().String("format", "table", "output format")

	equityPriceCmd.Flags().String("format", "table", "output format")

	equityGrantAddCmd.Flags().String("ticker", "", "stock ticker")
	equityGrantAddCmd.Flags().String("type", "", "grant type (iso, rsu)")
	equityGrantAddCmd.Flags().Float64("shares", 0, "total shares granted")
	equityGrantAddCmd.Flags().Float64("strike", 0, "strike price (ISOs only)")
	equityGrantAddCmd.Flags().Float64("fmv", 0, "FMV per share at grant date")
	equityGrantAddCmd.Flags().String("date", "", "grant date (YYYY-MM-DD)")
	equityGrantAddCmd.Flags().String("vest-start", "", "vesting start date (defaults to grant date)")
	equityGrantAddCmd.Flags().String("expiration", "", "expiration date (ISOs only)")
	equityGrantAddCmd.Flags().Int("cliff", 12, "cliff months")
	equityGrantAddCmd.Flags().Int("vest", 48, "total vesting months")
	equityGrantAddCmd.Flags().String("interval", "monthly", "vesting interval (monthly, quarterly)")
	equityGrantAddCmd.Flags().String("note", "", "optional note")
	equityGrantAddCmd.Flags().String("format", "table", "output format")

	equityGrantListCmd.Flags().String("format", "table", "output format")

	equityVestCmd.Flags().Float64("fmv", 0, "FMV per share override")

	equityVestScheduleCmd.Flags().String("format", "table", "output format")

	equityExerciseCmd.Flags().Float64("fmv", 0, "FMV per share at exercise")
	equityExerciseCmd.Flags().String("format", "table", "output format")

	equityGrantCmd.AddCommand(equityGrantAddCmd, equityGrantListCmd, equityGrantDeleteCmd)
	equityCmd.AddCommand(equityBuyCmd, equitySellCmd, equityLotsCmd, equityPortfolioCmd,
		equityPriceCmd, equityGrantCmd, equityVestCmd, equityVestScheduleCmd,
		equityExerciseCmd, equityIncludeCmd, equityExcludeCmd, equityImportCmd)
	rootCmd.AddCommand(equityCmd)
}
