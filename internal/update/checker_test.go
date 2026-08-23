package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/useragent"
)

// TestChecker_UserAgent verifies that the GitHub release check sends a
// User-Agent header that begins with "port-cli/".
func TestChecker_UserAgent(t *testing.T) {
	useragent.SetVersion("test-version")
	t.Cleanup(func() { useragent.SetVersion("dev") })

	wantUA := useragent.String()

	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
		}{TagName: "v1.0.0", HTMLURL: "https://example.com"})
	}))
	defer server.Close()

	// Temporarily override the releases URL to point at our test server.
	orig := releasesURL
	releasesURL = server.URL
	t.Cleanup(func() { releasesURL = orig })

	checker := NewChecker()
	_, err := checker.CheckLatestVersion(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatestVersion failed: %v", err)
	}

	if !strings.HasPrefix(gotUA, "port-cli/") {
		t.Errorf("User-Agent = %q, want prefix \"port-cli/\"", gotUA)
	}
	if gotUA != wantUA {
		t.Errorf("User-Agent = %q, want %q", gotUA, wantUA)
	}
}
