package output

import (
	"strings"
	"testing"
)

// TestBannerDoesNotWrapLogo guards against a regression where the tagline
// (wider than the ASCII logo) caused the outer container to be constrained to
// the narrower logo width, wrapping every line and shattering the art.
func TestBannerDoesNotWrapLogo(t *testing.T) {
	out := Banner(VersionInfo{
		Version:   "v0.0.0",
		BuildDate: "unknown",
		Commit:    "unknown",
		GoVersion: "go1.0",
		Platform:  "test/test",
	})

	// The logo is 6 rows. The tagline must stay on a single line and the
	// version block adds 5 rows, plus blank/separator rows. If the logo had
	// wrapped, the row count would balloon well past this.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	// Every full-width logo row contains exactly one trailing block-art glyph
	// run; a simple proxy is that the tagline appears intact on its own line.
	if !strings.Contains(out, "Port CLI") ||
		!strings.Contains(out, "Agentic Engineering Platform") {
		t.Fatalf("tagline missing or wrapped:\n%s", out)
	}

	// Sanity: the banner should be a compact block, not an over-wrapped mess.
	if len(lines) > 16 {
		t.Fatalf("banner has %d lines, expected a compact block (logo likely wrapped):\n%s", len(lines), out)
	}
}
