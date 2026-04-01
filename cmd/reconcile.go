package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sam/budget/internal/importer"
	"github.com/sam/budget/internal/model"
	"github.com/spf13/cobra"
)

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Reconciliation and verification tools",
}

var matchTransfersCmd = &cobra.Command{
	Use:   "transfers",
	Short: "Auto-match transfer_out with corresponding transfer_in transactions",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		maxDays, _ := cmd.Flags().GetInt("max-days")

		matches, err := service.AutoMatchTransfers(maxDays)
		if err != nil {
			return err
		}

		if format == formatJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(matches)
		}

		if len(matches) == 0 {
			fmt.Println("No transfer matches found.")
			return nil
		}

		fmt.Printf("Found %d transfer matches:\n\n", len(matches))
		for _, m := range matches {
			conf := m.Confidence
			switch conf {
			case "exact":
				conf = "EXACT"
			case "likely":
				conf = fmt.Sprintf("LIKELY (%dd apart)", m.DaysDiff)
			case "ambiguous":
				conf = "AMBIGUOUS (needs review)"
			}
			fmt.Printf("  [%s] %s\n", conf, m.OutTx.Date)
			fmt.Printf("    OUT: #%-5d %-30s %10s  %s\n",
				m.OutTx.ID, truncate(m.OutTx.Payee, 30),
				fmtDollars(m.OutTx.Amount), m.OutTx.AccountName)
			fmt.Printf("    IN:  #%-5d %-30s %10s  %s\n\n",
				m.InTx.ID, truncate(m.InTx.Payee, 30),
				fmtDollars(m.InTx.Amount), m.InTx.AccountName)
		}

		if dryRun {
			fmt.Println("Dry run — no changes applied.")
			return nil
		}

		count, err := service.ApplyTransferMatches(matches)
		if err != nil {
			return err
		}
		fmt.Printf("Linked %d transfer pairs (skipped ambiguous).\n", count)

		unmatched, _ := service.GetUnmatchedTransfers()
		if len(unmatched) > 0 {
			fmt.Printf("\n%d transfers still unmatched:\n", len(unmatched))
			for _, u := range unmatched {
				dir := "OUT"
				if u.Direction == "in" {
					dir = "IN "
				}
				fmt.Printf("  %s #%-5d %s  %-30s  %10s  %s\n",
					dir, u.Tx.ID, u.Tx.Date, truncate(u.Tx.Payee, 30),
					fmtDollars(u.Tx.Amount), u.Tx.AccountName)
			}
		}
		return nil
	},
}

var findDuplicatesCmd = &cobra.Command{
	Use:   "duplicates",
	Short: "Find potential duplicate transactions",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")

		groups, err := service.FindDuplicates()
		if err != nil {
			return err
		}

		if format == formatJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(groups)
		}

		if len(groups) == 0 {
			fmt.Println("No duplicates found.")
			return nil
		}

		total := 0
		for _, g := range groups {
			total += len(g.Transactions) - 1
		}
		fmt.Printf("Found %d duplicate groups (%d extra transactions):\n\n", len(groups), total)

		for i, g := range groups {
			fmt.Printf("  Group %d (%s):\n", i+1, g.Reason)
			for _, tx := range g.Transactions {
				fmt.Printf("    #%-5d %s  %-30s  %10s  %-8s  %s\n",
					tx.ID, tx.Date, truncate(tx.Payee, 30),
					fmtDollars(tx.Amount), tx.Type, tx.AccountName)
			}
			fmt.Println()
		}
		return nil
	},
}

var reconcileAccountCmd = &cobra.Command{
	Use:   "balance [account] [actual-balance]",
	Short: "Reconcile account balance against actual bank balance",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		note, _ := cmd.Flags().GetString("note")

		acc, err := service.GetAccountByName(args[0])
		if err != nil {
			return fmt.Errorf("account not found: %s", args[0])
		}

		bal, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("invalid balance: %s", args[1])
		}
		cents := int64(math.Round(bal * 100))

		report, err := service.ReconcileAccount(acc.ID, cents, note)
		if err != nil {
			return err
		}

		if format == formatJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}

		fmt.Printf("Reconciliation — %s\n", report.AccountName)
		fmt.Printf("  Calculated: %s\n", fmtDollars(report.CalculatedBal))
		fmt.Printf("  Actual:     %s\n", fmtDollars(report.ActualBal))
		if report.Discrepancy == 0 {
			fmt.Println("  Status:     BALANCED")
		} else {
			fmt.Printf("  Discrepancy: %s\n", fmtDollars(report.Discrepancy))
		}
		return nil
	},
}

var paycheckCmd = &cobra.Command{
	Use:   "paycheck",
	Short: "Manage paycheck records",
}

var addPaycheckCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a paycheck record",
	RunE: func(cmd *cobra.Command, args []string) error {
		date, _ := cmd.Flags().GetString("date")
		employer, _ := cmd.Flags().GetString("employer")
		gross, _ := cmd.Flags().GetFloat64("gross")
		federal, _ := cmd.Flags().GetFloat64("federal-tax")
		state, _ := cmd.Flags().GetFloat64("state-tax")
		ss, _ := cmd.Flags().GetFloat64("social-security")
		med, _ := cmd.Flags().GetFloat64("medicare")
		health, _ := cmd.Flags().GetFloat64("health-insurance")
		ret, _ := cmd.Flags().GetFloat64("401k")
		other, _ := cmd.Flags().GetFloat64("other-deductions")
		net, _ := cmd.Flags().GetFloat64("net")
		note, _ := cmd.Flags().GetString("note")

		if date == "" || gross == 0 || net == 0 {
			return fmt.Errorf("--date, --gross, and --net are required")
		}

		toCents := func(f float64) int64 { return int64(math.Round(f * 100)) }

		p, err := service.CreatePaycheck(model.Paycheck{
			Date:            date,
			Employer:        employer,
			GrossPay:        toCents(gross),
			FederalTax:      toCents(federal),
			StateTax:        toCents(state),
			SocialSecurity:  toCents(ss),
			Medicare:        toCents(med),
			HealthInsurance: toCents(health),
			Retirement401k:  toCents(ret),
			OtherDeductions: toCents(other),
			NetPay:          toCents(net),
			Note:            note,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Paycheck #%d added: %s gross %s, net %s\n",
			p.ID, p.Date, fmtDollars(p.GrossPay), fmtDollars(p.NetPay))

		// Auto-match to bank deposit
		matches, _ := service.MatchPaychecksToDeposits()
		for _, m := range matches {
			if m.Paycheck.ID == p.ID {
				if err := service.LinkPaycheckToTransaction(p.ID, m.Tx.ID); err != nil {
					return err
				}
				fmt.Printf("  Matched to deposit: #%d %s %s (%s)\n",
					m.Tx.ID, m.Tx.Date, fmtDollars(m.Tx.Amount), m.Tx.AccountName)
				break
			}
		}
		return nil
	},
}

var listPaychecksCmd = &cobra.Command{
	Use:   "list",
	Short: "List paycheck records",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")

		paychecks, err := service.ListPaychecks(20)
		if err != nil {
			return err
		}

		if format == formatJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(paychecks)
		}

		if len(paychecks) == 0 {
			fmt.Println("No paychecks recorded.")
			return nil
		}

		fmt.Printf("%-12s  %-15s  %10s  %10s  %10s  %s\n",
			"DATE", "EMPLOYER", "GROSS", "DEDUCTIONS", "NET", "LINKED")
		for _, p := range paychecks {
			linked := "—"
			if p.TxID != nil {
				linked = fmt.Sprintf("#%d", *p.TxID)
			}
			fmt.Printf("%-12s  %-15s  %10s  %10s  %10s  %s\n",
				p.Date, truncate(p.Employer, 15),
				fmtDollars(p.GrossPay), fmtDollars(p.TotalDeductions()),
				fmtDollars(p.NetPay), linked)
		}
		return nil
	},
}

var matchPaychecksCmd = &cobra.Command{
	Use:   "match",
	Short: "Match unlinked paychecks to bank deposits",
	RunE: func(cmd *cobra.Command, args []string) error {
		matches, err := service.MatchPaychecksToDeposits()
		if err != nil {
			return err
		}

		if len(matches) == 0 {
			fmt.Println("No unlinked paycheck matches found.")
			return nil
		}

		for _, m := range matches {
			if err := service.LinkPaycheckToTransaction(m.Paycheck.ID, m.Tx.ID); err != nil {
				return err
			}
			fmt.Printf("Linked paycheck %s (%s) -> deposit #%d %s %s\n",
				m.Paycheck.Date, fmtDollars(m.Paycheck.NetPay),
				m.Tx.ID, m.Tx.Date, m.Tx.AccountName)
		}
		return nil
	},
}

var importPaycheckCmd = &cobra.Command{
	Use:   "import [pdf-file]",
	Short: "Import a paycheck from a Rippling (or similar) PDF pay stub",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		employerOverride, _ := cmd.Flags().GetString("employer")

		imp := importer.NewPaycheckPDF()
		pc, err := imp.Import(path)
		if err != nil {
			return err
		}

		if employerOverride != "" {
			pc.Employer = employerOverride
		}

		fmt.Printf("Parsed pay stub:\n")
		fmt.Printf("  Date:            %s\n", pc.Date)
		fmt.Printf("  Employer:        %s\n", pc.Employer)
		fmt.Printf("  Gross Pay:       %s\n", fmtDollars(pc.GrossPay))
		fmt.Printf("  Federal Tax:     %s\n", fmtDollars(pc.FederalTax))
		fmt.Printf("  State Tax:       %s\n", fmtDollars(pc.StateTax))
		fmt.Printf("  Social Security: %s\n", fmtDollars(pc.SocialSecurity))
		fmt.Printf("  Medicare:        %s\n", fmtDollars(pc.Medicare))
		fmt.Printf("  Health Ins:      %s\n", fmtDollars(pc.HealthInsurance))
		fmt.Printf("  401(k):          %s\n", fmtDollars(pc.Retirement401k))
		fmt.Printf("  Other Deductions:%s\n", fmtDollars(pc.OtherDeductions))
		fmt.Printf("  Net Pay:         %s\n", fmtDollars(pc.NetPay))

		saved, err := service.CreatePaycheck(*pc)
		if err != nil {
			return fmt.Errorf("save paycheck: %w", err)
		}
		fmt.Printf("\nPaycheck #%d saved.\n", saved.ID)

		matches, _ := service.MatchPaychecksToDeposits()
		for _, m := range matches {
			if m.Paycheck.ID == saved.ID {
				if err := service.LinkPaycheckToTransaction(saved.ID, m.Tx.ID); err != nil {
					return err
				}
				fmt.Printf("Matched to deposit: #%d %s %s (%s)\n",
					m.Tx.ID, m.Tx.Date, fmtDollars(m.Tx.Amount), m.Tx.AccountName)
				break
			}
		}
		return nil
	},
}

func fmtDollars(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	d := cents / 100
	r := cents % 100
	s := fmt.Sprintf("%d", d)
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ",")
	}
	if neg {
		return fmt.Sprintf("-$%s.%02d", s, r)
	}
	return fmt.Sprintf("$%s.%02d", s, r)
}

func init() {
	matchTransfersCmd.Flags().Bool("dry-run", false, "show matches without applying")
	matchTransfersCmd.Flags().Int("max-days", 3, "max days apart for transfer matching")
	matchTransfersCmd.Flags().String("format", "", "output format (json)")

	findDuplicatesCmd.Flags().String("format", "", "output format (json)")

	reconcileAccountCmd.Flags().String("format", "", "output format (json)")
	reconcileAccountCmd.Flags().String("note", "", "reconciliation note")

	addPaycheckCmd.Flags().String("date", "", "pay date (YYYY-MM-DD)")
	addPaycheckCmd.Flags().String("employer", "", "employer name")
	addPaycheckCmd.Flags().Float64("gross", 0, "gross pay")
	addPaycheckCmd.Flags().Float64("federal-tax", 0, "federal tax withheld")
	addPaycheckCmd.Flags().Float64("state-tax", 0, "state tax withheld")
	addPaycheckCmd.Flags().Float64("social-security", 0, "social security")
	addPaycheckCmd.Flags().Float64("medicare", 0, "medicare")
	addPaycheckCmd.Flags().Float64("health-insurance", 0, "health insurance")
	addPaycheckCmd.Flags().Float64("401k", 0, "401k contribution")
	addPaycheckCmd.Flags().Float64("other-deductions", 0, "other deductions")
	addPaycheckCmd.Flags().Float64("net", 0, "net pay (take-home)")
	addPaycheckCmd.Flags().String("note", "", "note")

	listPaychecksCmd.Flags().String("format", "", "output format (json)")

	importPaycheckCmd.Flags().String("employer", "", "override employer name")

	paycheckCmd.AddCommand(addPaycheckCmd, listPaychecksCmd, matchPaychecksCmd, importPaycheckCmd)
	reconcileCmd.AddCommand(matchTransfersCmd, findDuplicatesCmd, reconcileAccountCmd, paycheckCmd)
	rootCmd.AddCommand(reconcileCmd)
}
