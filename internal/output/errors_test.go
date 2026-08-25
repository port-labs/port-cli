package output

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestFormatErrorNoColorSuggestionHasNoANSI(t *testing.T) {
	Init(true)
	formatted := FormatError(errors.New("invalid resource: nope"))
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if ansi.MatchString(formatted) {
		t.Fatalf("expected no ANSI escapes, got %q", formatted)
	}
	if !strings.Contains(formatted, "Suggestion:") {
		t.Fatalf("expected plain suggestion, got %q", formatted)
	}
}
