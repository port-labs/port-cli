package commands

import "testing"

func TestErrorLimitDefaultLimitsToFive(t *testing.T) {
	if got := errorLimit(10, defaultMaxErrors); got != 5 {
		t.Fatalf("expected default limit 5, got %d", got)
	}
}

func TestErrorLimitZeroShowsAll(t *testing.T) {
	if got := errorLimit(10, 0); got != 10 {
		t.Fatalf("expected zero max to show all 10 errors, got %d", got)
	}
}

func TestErrorLimitMinusOneHidesAll(t *testing.T) {
	if got := errorLimit(10, hideAllErrors); got != 0 {
		t.Fatalf("expected -1 max to hide all errors, got %d", got)
	}
}

func TestValidateMaxErrorsFlagAllowsMinusOne(t *testing.T) {
	if err := validateMaxErrorsFlag(hideAllErrors); err != nil {
		t.Fatalf("expected -1 to be allowed, got %v", err)
	}
}

func TestValidateMaxErrorsFlagRejectsLowerNegativeValues(t *testing.T) {
	if err := validateMaxErrorsFlag(-2); err == nil {
		t.Fatal("expected -2 to be rejected")
	}
}

func TestShouldPrintErrorsFalseWhenHidden(t *testing.T) {
	if shouldPrintErrors(10, hideAllErrors) {
		t.Fatal("expected shouldPrintErrors to be false when max-errors is -1")
	}
}

func TestShouldPrintErrorsTrueByDefault(t *testing.T) {
	if !shouldPrintErrors(10, defaultMaxErrors) {
		t.Fatal("expected shouldPrintErrors to be true with the default max-errors")
	}
}
