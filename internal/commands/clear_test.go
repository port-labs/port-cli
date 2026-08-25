package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/spf13/cobra"
)

func TestClearPagesFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterClear(rootCmd)

	clearCmd, _, err := rootCmd.Find([]string{"clear"})
	if err != nil || clearCmd == nil {
		t.Fatal("clear command not found")
	}

	if err := clearCmd.ParseFlags([]string{"--pages", "--force"}); err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	pages, err := clearCmd.Flags().GetBool("pages")
	if err != nil {
		t.Fatalf("could not get --pages: %v", err)
	}
	if !pages {
		t.Fatalf("expected --pages to be true")
	}
}

func TestClearNewResourceFlagsParsed(t *testing.T) {
	flags := []string{"blueprints", "entities", "actions", "automations", "scorecards"}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			rootCmd := &cobra.Command{Use: "port"}
			RegisterClear(rootCmd)

			clearCmd, _, err := rootCmd.Find([]string{"clear"})
			if err != nil || clearCmd == nil {
				t.Fatal("clear command not found")
			}

			if err := clearCmd.ParseFlags([]string{"--" + flag, "--force"}); err != nil {
				t.Fatalf("unexpected error parsing flags: %v", err)
			}

			val, err := clearCmd.Flags().GetBool(flag)
			if err != nil {
				t.Fatalf("could not get --%s: %v", flag, err)
			}
			if !val {
				t.Fatalf("expected --%s to be true", flag)
			}
		})
	}
}

func TestDeleteProtectedFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterClear(rootCmd)

	clearCmd, _, err := rootCmd.Find([]string{"clear"})
	if err != nil || clearCmd == nil {
		t.Fatal("clear command not found")
	}

	if err := clearCmd.ParseFlags([]string{"--pages", "--delete-protected-pages", "--force"}); err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	deleteProtected, err := clearCmd.Flags().GetBool("delete-protected-pages")
	if err != nil {
		t.Fatalf("could not get --delete-protected-pages: %v", err)
	}
	if !deleteProtected {
		t.Fatalf("expected --delete-protected-pages to be true")
	}
}

func TestScopeBlueprintsFiltersToGivenIdentifiers(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "repository"},
		{"identifier": "environment"},
	}

	scoped, err := scopeBlueprints(blueprints, []string{"service", "environment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scoped) != 2 {
		t.Fatalf("expected 2 blueprints, got %d", len(scoped))
	}
	if scoped[0]["identifier"] != "service" || scoped[1]["identifier"] != "environment" {
		t.Fatalf("unexpected scoped blueprints: %v", scoped)
	}
}

func TestScopeBlueprintsReturnsErrorOnMissingIdentifiers(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "repository"},
	}

	scoped, err := scopeBlueprints(blueprints, []string{"service", "aispec", "foo"})
	if err == nil {
		t.Fatalf("expected error for missing blueprints, got nil")
	}
	if len(scoped) != 0 {
		t.Fatalf("expected 0 scoped blueprints on error, got %d", len(scoped))
	}
	expectedErr := "blueprint(s) not found in organization: aispec, foo"
	if err.Error() != expectedErr {
		t.Fatalf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestScopeBlueprintsReturnsAllWhenScopeEmpty(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "repository"},
	}

	scoped, err := scopeBlueprints(blueprints, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scoped) != 2 {
		t.Fatalf("expected all 2 blueprints, got %d", len(scoped))
	}
}

func TestFilterProtectedBlueprintsExcludesUnderscorePrefixed(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "_user"},
		{"identifier": "_team"},
		{"identifier": "environment"},
	}

	filtered := filterProtectedBlueprints(blueprints, false)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 blueprints, got %d", len(filtered))
	}
	if filtered[0]["identifier"] != "service" || filtered[1]["identifier"] != "environment" {
		t.Fatalf("unexpected filtered blueprints: %v", filtered)
	}
}

func TestFilterProtectedBlueprintsIncludesAllWhenEnabled(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "_user"},
	}

	filtered := filterProtectedBlueprints(blueprints, true)
	if len(filtered) != 2 {
		t.Fatalf("expected all 2 blueprints, got %d", len(filtered))
	}
}

func TestRootFoldersForDeletionSelectsRootFoldersWithoutSidebarFilter(t *testing.T) {
	folders := []api.Folder{
		{"identifier": "root-a"},
		{"identifier": "hidden-root"},
		{"identifier": "child", "parent": "root-a"},
		{"identifier": "_system"},
	}

	roots := rootFoldersForDeletion(folders, false)
	if len(roots) != 2 {
		t.Fatalf("expected 2 deletable root folders, got %d", len(roots))
	}
	if roots[0]["identifier"] != "root-a" || roots[1]["identifier"] != "hidden-root" {
		t.Fatalf("unexpected root folder selection: %v", roots)
	}
}

func TestRootFoldersForDeletionProtectedAppendedLastWhenEnabled(t *testing.T) {
	folders := []api.Folder{
		{"identifier": "root-a"},
		{"identifier": "with_under_score"},
		{"identifier": "_system"},
		{"identifier": "child", "parent": "root-a"},
	}

	roots := rootFoldersForDeletion(folders, true)
	got := []string{
		roots[0]["identifier"].(string),
		roots[1]["identifier"].(string),
		roots[2]["identifier"].(string),
	}
	want := []string{"root-a", "with_under_score", "_system"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected root folder order: got %v want %v", got, want)
		}
	}
}

func TestRootPagesForDeletionFiltersBySidebarVisibilityFirst(t *testing.T) {
	pages := []api.Page{
		{"identifier": "home", "showInSidebar": true},
		{"identifier": "hidden-home", "showInSidebar": false},
		{"identifier": "_catalog", "showInSidebar": true},
		{"identifier": "details", "parent": "folder-a", "showInSidebar": true},
	}

	roots := rootPagesForDeletion(pages, false)
	if len(roots) != 1 {
		t.Fatalf("expected 1 deletable root page, got %d", len(roots))
	}
	if roots[0]["identifier"] != "home" {
		t.Fatalf("unexpected root page selection: %v", roots)
	}
}

func TestRootPagesForDeletionProtectedAppendedLastWhenEnabled(t *testing.T) {
	pages := []api.Page{
		{"identifier": "home", "showInSidebar": true},
		{"identifier": "with_under_score", "showInSidebar": true},
		{"identifier": "_catalog", "showInSidebar": true},
		{"identifier": "hidden-home", "showInSidebar": false},
	}

	roots := rootPagesForDeletion(pages, true)
	got := []string{
		roots[0]["identifier"].(string),
		roots[1]["identifier"].(string),
		roots[2]["identifier"].(string),
	}
	want := []string{"home", "with_under_score", "_catalog"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected root page order: got %v want %v", got, want)
		}
	}
}

func TestIsDeletablePage(t *testing.T) {
	if !isDeletablePage(api.Page{"showInSidebar": true}) {
		t.Fatal("expected showInSidebar=true page to be deletable")
	}
	if isDeletablePage(api.Page{"showInSidebar": false}) {
		t.Fatal("expected showInSidebar=false page to be skipped")
	}
	if isDeletablePage(api.Page{}) {
		t.Fatal("expected page without showInSidebar to be skipped")
	}
}

func TestIsProtectedSidebarItemIdentifier(t *testing.T) {
	if isProtectedSidebarItemIdentifier("plain") {
		t.Fatal("expected plain identifier to be non-protected")
	}
	if isProtectedSidebarItemIdentifier("with_under_score") {
		t.Fatal("expected identifier with underscore (but not leading) to be non-protected")
	}
	if !isProtectedSidebarItemIdentifier("_system") {
		t.Fatal("expected underscore-prefixed identifier to be protected")
	}
}

// newClearTestServer returns an httptest.Server that handles auth and dispatches
// search/delete calls to the provided handlers keyed by blueprint identifier.
func newClearTestServer(t *testing.T, bpID string, entities []map[string]interface{}, deletedIDs *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true, "accessToken": "tok", "expiresIn": 3600,
			})
			return
		}
		if r.URL.Path == "/blueprints/"+bpID+"/entities/search" {
			json.NewEncoder(w).Encode(map[string]interface{}{"entities": entities})
			return
		}
		if r.URL.Path == "/blueprints/"+bpID+"/bulk/entities/delete" {
			var body struct {
				Entities []string `json:"entities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode bulk-delete body: %v", err)
			}
			*deletedIDs = append(*deletedIDs, body.Entities...)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
			return
		}
		http.NotFound(w, r)
	}))
}

func TestClearAllEntities_JQFilterMatchesSubset(t *testing.T) {
	entities := []map[string]interface{}{
		{"identifier": "e1", "properties": map[string]interface{}{"status": "active"}},
		{"identifier": "e2", "properties": map[string]interface{}{"status": "inactive"}},
		{"identifier": "e3", "properties": map[string]interface{}{"status": "inactive"}},
	}
	var deletedIDs []string

	server := newClearTestServer(t, "svc", entities, &deletedIDs)
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	defer client.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	blueprints := []api.Blueprint{{"identifier": "svc"}}
	if err := clearAllEntities(cmd, client, blueprints, `.properties.status == "inactive"`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(deletedIDs)
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 deleted entities, got %d: %v", len(deletedIDs), deletedIDs)
	}
	if deletedIDs[0] != "e2" || deletedIDs[1] != "e3" {
		t.Errorf("expected [e2 e3] deleted, got %v", deletedIDs)
	}
}

func TestClearAllEntities_NoJQFilterDeletesAll(t *testing.T) {
	entities := []map[string]interface{}{
		{"identifier": "e1", "properties": map[string]interface{}{"status": "active"}},
		{"identifier": "e2", "properties": map[string]interface{}{"status": "inactive"}},
	}
	var deletedIDs []string

	server := newClearTestServer(t, "svc", entities, &deletedIDs)
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	defer client.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	blueprints := []api.Blueprint{{"identifier": "svc"}}
	if err := clearAllEntities(cmd, client, blueprints, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(deletedIDs)
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 deleted entities, got %d: %v", len(deletedIDs), deletedIDs)
	}
	if deletedIDs[0] != "e1" || deletedIDs[1] != "e2" {
		t.Errorf("expected [e1 e2] deleted, got %v", deletedIDs)
	}
}

func TestClearAllEntities_JQFilterRuntimeErrorReturnsError(t *testing.T) {
	// ascii_downcase on a number causes a gojq runtime type error.
	entities := []map[string]interface{}{
		{"identifier": "bad-entity", "properties": map[string]interface{}{"count": 5}},
	}
	var deletedIDs []string

	server := newClearTestServer(t, "svc", entities, &deletedIDs)
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	defer client.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	blueprints := []api.Blueprint{{"identifier": "svc"}}
	err := clearAllEntities(cmd, client, blueprints, `.properties.count | ascii_downcase`)
	if err == nil {
		t.Fatal("expected error from JQ runtime type error, got nil")
	}
	if !strings.Contains(err.Error(), "bad-entity") {
		t.Errorf("expected error to mention entity ID, got: %v", err)
	}
	if len(deletedIDs) != 0 {
		t.Errorf("expected no entities deleted on error, got %v", deletedIDs)
	}
}

func TestClearAllActionsUsesOrganizationWideActionsEndpoint(t *testing.T) {
	actions := []map[string]interface{}{
		{
			"identifier": "deploy",
			"trigger": map[string]interface{}{
				"type":                "self-service",
				"blueprintIdentifier": "svc",
			},
		},
		{
			"identifier": "publish",
			"trigger": map[string]interface{}{
				"type":                "self-service",
				"blueprintIdentifier": "repo",
			},
		},
		{
			"identifier": "expire-env",
			"trigger": map[string]interface{}{
				"type":  "automation",
				"event": map[string]interface{}{"blueprintIdentifier": "svc"},
			},
		},
	}
	var deletedIDs []string

	server := newClearActionsTestServer(t, actions, &deletedIDs)
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	defer client.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	blueprints := []api.Blueprint{{"identifier": "svc"}}
	if err := clearAllActions(cmd, client, blueprints); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deletedIDs) != 1 || deletedIDs[0] != "deploy" {
		t.Fatalf("expected only deploy to be deleted, got %v", deletedIDs)
	}
}

func TestClearAllAutomationsSkipsSelfServiceActions(t *testing.T) {
	actions := []map[string]interface{}{
		{
			"identifier": "deploy",
			"trigger": map[string]interface{}{
				"type":                "self-service",
				"blueprintIdentifier": "svc",
			},
		},
		{
			"identifier": "expire-env",
			"trigger": map[string]interface{}{
				"type":  "automation",
				"event": map[string]interface{}{"blueprintIdentifier": "svc"},
			},
		},
	}
	var deletedIDs []string

	server := newClearActionsTestServer(t, actions, &deletedIDs)
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	defer client.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if err := clearAllAutomations(cmd, client); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deletedIDs) != 1 || deletedIDs[0] != "expire-env" {
		t.Fatalf("expected only expire-env to be deleted, got %v", deletedIDs)
	}
}

func newClearActionsTestServer(t *testing.T, actions []map[string]interface{}, deletedIDs *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true, "accessToken": "tok", "expiresIn": 3600,
			})
			return
		}
		if r.URL.Path == "/actions" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{"actions": actions})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/blueprints/") && strings.HasSuffix(r.URL.Path, "/actions") {
			t.Errorf("deprecated blueprint action list endpoint was called: %s", r.URL.Path)
			http.Error(w, "deprecated", http.StatusGone)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/actions/") && r.Method == http.MethodDelete {
			*deletedIDs = append(*deletedIDs, strings.TrimPrefix(r.URL.Path, "/actions/"))
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
			return
		}
		http.NotFound(w, r)
	}))
}
