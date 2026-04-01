package importer

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseDollarAmount
// ---------------------------------------------------------------------------

func TestParseDollarAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"simple", "1234.56", 123456},
		{"with commas", "1,234.56", 123456},
		{"large amount", "12,345.67", 1234567},
		{"small amount", "5.99", 599},
		{"zero", "0.00", 0},
		{"no decimals implicit", "100.00", 10000},
		{"invalid", "abc", 0},
		{"empty", "", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseDollarAmount(tc.input)
			if got != tc.want {
				t.Errorf("parseDollarAmount(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizePayDate
// ---------------------------------------------------------------------------

func TestNormalizePayDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"MM/DD/YYYY", "03/15/2026", "2026-03-15"},
		{"M/D/YYYY", "3/5/2026", "2026-03-05"},
		{"MM-DD-YYYY", "03-15-2026", "2026-03-15"},
		{"two digit year", "03/15/26", "2026-03-15"},
		{"single digit month and day with dash", "3-5-2026", "2026-03-05"},
		{"no separator", "foobar", "foobar"},
		{"too few parts", "03/15", "03/15"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizePayDate(tc.input)
			if got != tc.want {
				t.Errorf("normalizePayDate(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// min
// ---------------------------------------------------------------------------

func TestMin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, 1, -1},
		{10, 10, 10},
	}

	for _, tc := range tests {
		got := min(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("min(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parsePayStub
// ---------------------------------------------------------------------------

func TestParsePayStub(t *testing.T) {
	t.Parallel()

	sampleText := `
ACME Corporation
123 Main Street
Anytown, CA 90210

EARNINGS STATEMENT

Employee: John Doe
Pay Date: 03/15/2026
Pay Period: 03/01/2026 - 03/15/2026

Gross Pay              $5,000.00
Federal Income Tax       $750.00
State Income Tax         $250.00
Social Security          $310.00
Medicare                  $72.50
Health Insurance         $200.00
401k                     $500.00
Net Pay                $2,917.50
`

	pc, err := parsePayStub(sampleText)
	if err != nil {
		t.Fatalf("parsePayStub: %v", err)
	}

	if pc.Date != "2026-03-15" {
		t.Errorf("Date = %q, want 2026-03-15", pc.Date)
	}
	if pc.GrossPay != 500000 {
		t.Errorf("GrossPay = %d, want 500000", pc.GrossPay)
	}
	if pc.FederalTax != 75000 {
		t.Errorf("FederalTax = %d, want 75000", pc.FederalTax)
	}
	if pc.StateTax != 25000 {
		t.Errorf("StateTax = %d, want 25000", pc.StateTax)
	}
	if pc.SocialSecurity != 31000 {
		t.Errorf("SocialSecurity = %d, want 31000", pc.SocialSecurity)
	}
	if pc.Medicare != 7250 {
		t.Errorf("Medicare = %d, want 7250", pc.Medicare)
	}
	if pc.HealthInsurance != 20000 {
		t.Errorf("HealthInsurance = %d, want 20000", pc.HealthInsurance)
	}
	if pc.Retirement401k != 50000 {
		t.Errorf("Retirement401k = %d, want 50000", pc.Retirement401k)
	}
	if pc.NetPay != 291750 {
		t.Errorf("NetPay = %d, want 291750", pc.NetPay)
	}
	if pc.Employer != "ACME Corporation" {
		t.Errorf("Employer = %q, want 'ACME Corporation'", pc.Employer)
	}
}

func TestParsePayStub_DentalVisionAccumulate(t *testing.T) {
	t.Parallel()

	sampleText := `
TechCorp Inc

Pay Date: 01/31/2026

Gross Pay              $4,000.00
Medical                  $150.00
Dental                    $30.00
Vision                    $20.00
Net Pay                $3,800.00
`

	pc, err := parsePayStub(sampleText)
	if err != nil {
		t.Fatalf("parsePayStub: %v", err)
	}

	// Medical sets HealthInsurance to 15000, dental adds 3000, vision adds 2000
	if pc.HealthInsurance != 20000 {
		t.Errorf("HealthInsurance = %d, want 20000 (medical+dental+vision)", pc.HealthInsurance)
	}
}

func TestParsePayStub_NoAmounts(t *testing.T) {
	t.Parallel()

	_, err := parsePayStub("This is just random text with no pay info")
	if err == nil {
		t.Error("expected error for text with no pay amounts")
	}
}

func TestParsePayStub_OtherDeductions(t *testing.T) {
	t.Parallel()

	// Gross - Net = 200000, but only federal = 50000 known
	// So OtherDeductions = 150000
	sampleText := `
SomeCo

Pay Date: 02/28/2026

Gross Pay              $3,000.00
Federal Income Tax       $500.00
Net Pay                $1,000.00
`

	pc, err := parsePayStub(sampleText)
	if err != nil {
		t.Fatalf("parsePayStub: %v", err)
	}

	if pc.OtherDeductions != 150000 {
		t.Errorf("OtherDeductions = %d, want 150000", pc.OtherDeductions)
	}
}
