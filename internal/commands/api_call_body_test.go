package commands

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadRequestBody_ReadsReader(t *testing.T) {
	got, err := readRequestBody(strings.NewReader(`{"from":"stdin"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"from":"stdin"}` {
		t.Errorf("expected reader body, got %q", got)
	}
}

func TestReadRequestBodyFile_ReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	got, err := readRequestBodyFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"from":"file"}` {
		t.Errorf("expected file body, got %q", got)
	}
}

func TestReadRequestBodyFile_MissingFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := readRequestBodyFile(path)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("expected error to name the file, got %v", err)
	}
}

func TestReadRequestBodyFile_EmptyPathFails(t *testing.T) {
	if _, err := readRequestBodyFile(""); err == nil {
		t.Fatal("expected an error for an empty path")
	}
}

// runAPICall executes `port api call` with args against server, reading stdin from stdin.
func runAPICall(t *testing.T, server *httptest.Server, stdin io.Reader, args ...string) error {
	t.Helper()

	rootCmd := &cobra.Command{Use: "port", SilenceUsage: true, SilenceErrors: true}
	RegisterAPI(rootCmd)
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(append([]string{"api", "call"}, args...))

	ctx := WithGlobalFlags(context.Background(), GlobalFlags{
		ConfigFile:   filepath.Join(t.TempDir(), "config.yaml"),
		ClientID:     "id",
		ClientSecret: "secret",
		APIURL:       server.URL,
	})
	return rootCmd.ExecuteContext(ctx)
}

func newAPICallTestServer(t *testing.T, endpoint string, gotMethod *string, gotBody *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "accessToken": "tok", "expiresIn": 3600})
			return
		}
		if r.URL.Path == endpoint {
			*gotMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(gotBody); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		http.NotFound(w, r)
	}))
}

func TestAPICall_SendsBodyFromStdin(t *testing.T) {
	const endpoint = "/blueprints/task/entities/task_1"
	var gotMethod string
	var gotBody map[string]any
	server := newAPICallTestServer(t, endpoint, &gotMethod, &gotBody)
	defer server.Close()

	// A body larger than typical per-argument limits, which is what this feature is for.
	spec := strings.Repeat("x", 20000)
	stdin := strings.NewReader(`{"properties":{"spec":"` + spec + `"}}`)

	if err := runAPICall(t, server, stdin, endpoint, "--method", "PATCH", "--data", "-"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("expected PATCH, got %q", gotMethod)
	}
	properties, ok := gotBody["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object in body, got %v", gotBody)
	}
	if properties["spec"] != spec {
		t.Errorf("expected spec of length %d, got %v", len(spec), len(properties["spec"].(string)))
	}
}

func TestAPICall_SendsBodyFromFileAndDefaultsToPost(t *testing.T) {
	const endpoint = "/actions/my-action/runs"
	var gotMethod string
	var gotBody map[string]any
	server := newAPICallTestServer(t, endpoint, &gotMethod, &gotBody)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(path, []byte(`{"properties":{"name":"from-file"}}`), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	if err := runAPICall(t, server, strings.NewReader(""), endpoint, "--data", "@"+path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %q", gotMethod)
	}
	properties, ok := gotBody["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object in body, got %v", gotBody)
	}
	if properties["name"] != "from-file" {
		t.Errorf("expected name from file, got %v", properties["name"])
	}
}

func TestAPICall_MissingBodyFileFailsBeforeRequest(t *testing.T) {
	const endpoint = "/actions/my-action/runs"
	var gotMethod string
	var gotBody map[string]any
	server := newAPICallTestServer(t, endpoint, &gotMethod, &gotBody)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "missing.json")
	err := runAPICall(t, server, strings.NewReader(""), endpoint, "--data", "@"+path)
	if err == nil {
		t.Fatal("expected an error for a missing body file")
	}
	if gotMethod != "" {
		t.Errorf("expected no request to %s, got %s", endpoint, gotMethod)
	}
}

func TestAPICall_SendsLiteralBodyAndAtDashReadsStdin(t *testing.T) {
	const endpoint = "/actions/my-action/runs"
	cases := []struct {
		name  string
		data  string
		stdin string
		want  string
	}{
		{name: "literal", data: `{"properties":{"name":"literal"}}`, stdin: `{"properties":{"name":"ignored"}}`, want: "literal"},
		{name: "@-", data: "@-", stdin: `{"properties":{"name":"stdin"}}`, want: "stdin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod string
			var gotBody map[string]any
			server := newAPICallTestServer(t, endpoint, &gotMethod, &gotBody)
			defer server.Close()

			if err := runAPICall(t, server, strings.NewReader(tc.stdin), endpoint, "--data", tc.data); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			properties, ok := gotBody["properties"].(map[string]any)
			if !ok {
				t.Fatalf("expected properties object in body, got %v", gotBody)
			}
			if properties["name"] != tc.want {
				t.Errorf("expected name %q, got %v", tc.want, properties["name"])
			}
		})
	}
}
