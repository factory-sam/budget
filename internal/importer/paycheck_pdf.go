package importer

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/sam/budget/internal/model"
)

type PaycheckPDFImporter struct{}

func NewPaycheckPDF() *PaycheckPDFImporter {
	return &PaycheckPDFImporter{}
}

func (p *PaycheckPDFImporter) Import(path string) (*model.Paycheck, error) {
	text, err := extractPDFText(path)
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}

	paycheck, err := parsePayStub(text)
	if err != nil {
		return nil, fmt.Errorf("parse pay stub: %w", err)
	}
	return paycheck, nil
}

func extractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

var (
	amountRe = regexp.MustCompile(`\$?([\d,]+\.\d{2})`)
	dateRe   = regexp.MustCompile(`(?i)(?:pay\s*date|check\s*date|period\s*end(?:ing)?)\s*[:\s]*(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`)
)

func parsePayStub(text string) (*model.Paycheck, error) {
	lines := strings.Split(text, "\n")
	pc := &model.Paycheck{}

	// Try to find pay date
	if m := dateRe.FindStringSubmatch(text); len(m) > 1 {
		pc.Date = normalizePayDate(m[1])
	}

	// Extract labeled amounts by scanning for known patterns
	labelMap := map[string]*int64{
		"gross pay":          &pc.GrossPay,
		"total gross":        &pc.GrossPay,
		"gross earnings":     &pc.GrossPay,
		"federal income tax": &pc.FederalTax,
		"federal tax":        &pc.FederalTax,
		"fed income tax":     &pc.FederalTax,
		"fed withholding":    &pc.FederalTax,
		"state income tax":   &pc.StateTax,
		"state tax":          &pc.StateTax,
		"ca income tax":      &pc.StateTax,
		"ca sdi":             &pc.StateTax,
		"social security":    &pc.SocialSecurity,
		"oasdi":              &pc.SocialSecurity,
		"fica ss":            &pc.SocialSecurity,
		"medicare":           &pc.Medicare,
		"fica med":           &pc.Medicare,
		"health":             &pc.HealthInsurance,
		"medical":            &pc.HealthInsurance,
		"dental":             &pc.HealthInsurance,
		"vision":             &pc.HealthInsurance,
		"401k":               &pc.Retirement401k,
		"401(k)":             &pc.Retirement401k,
		"retirement":         &pc.Retirement401k,
		"net pay":            &pc.NetPay,
		"take home":          &pc.NetPay,
		"total net pay":      &pc.NetPay,
	}

	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		for label, target := range labelMap {
			if strings.Contains(lower, label) {
				amounts := amountRe.FindAllStringSubmatch(line, -1)
				if len(amounts) > 0 {
					// Use the last amount on the line (usually the current period amount)
					last := amounts[len(amounts)-1][1]
					if val := parseDollarAmount(last); val > 0 {
						// For deductions that accumulate across lines (health = medical+dental+vision),
						// add rather than replace
						if label == "dental" || label == "vision" {
							*target += val
						} else if *target == 0 {
							*target = val
						}
					}
				}
			}
		}
	}

	// Try to find employer name from the text (usually near the top)
	for _, line := range lines[:min(20, len(lines))] {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 3 && len(trimmed) < 60 && !strings.Contains(trimmed, "$") && !amountRe.MatchString(trimmed) {
			if !strings.Contains(strings.ToLower(trimmed), "pay") &&
				!strings.Contains(strings.ToLower(trimmed), "stub") &&
				!strings.Contains(strings.ToLower(trimmed), "period") &&
				!strings.Contains(strings.ToLower(trimmed), "employee") &&
				!strings.Contains(strings.ToLower(trimmed), "address") {
				if pc.Employer == "" {
					pc.Employer = trimmed
				}
			}
		}
	}

	if pc.GrossPay == 0 && pc.NetPay == 0 {
		return nil, fmt.Errorf("could not extract pay amounts from PDF — try manual entry with: budget reconcile paycheck add")
	}

	// Calculate other deductions if we have gross and net
	if pc.GrossPay > 0 && pc.NetPay > 0 {
		knownDeductions := pc.FederalTax + pc.StateTax + pc.SocialSecurity + pc.Medicare +
			pc.HealthInsurance + pc.Retirement401k
		totalDeductions := pc.GrossPay - pc.NetPay
		if totalDeductions > knownDeductions {
			pc.OtherDeductions = totalDeductions - knownDeductions
		}
	}

	return pc, nil
}

func parseDollarAmount(s string) int64 {
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 100))
}

func normalizePayDate(s string) string {
	// Convert MM/DD/YYYY or MM-DD-YYYY to YYYY-MM-DD
	s = strings.ReplaceAll(s, "-", "/")
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return s
	}
	month := parts[0]
	day := parts[1]
	year := parts[2]
	if len(year) == 2 {
		year = "20" + year
	}
	if len(month) == 1 {
		month = "0" + month
	}
	if len(day) == 1 {
		day = "0" + day
	}
	return year + "-" + month + "-" + day
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
