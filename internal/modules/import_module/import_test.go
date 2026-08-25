package import_module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/export"
)

func TestApplyDataExclusion_Deep(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "service"},
			{"identifier": "_rule_result"},
		},
		Entities: []api.Entity{
			{"identifier": "e1", "blueprint": "service"},
			{"identifier": "e2", "blueprint": "_rule_result"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "sc1", "blueprintIdentifier": "_rule_result"},
		},
		Actions: []api.Action{
			{"identifier": "a1", "blueprint": "_rule_result"},
			{"identifier": "a2", "blueprint": "service"},
		},
		BlueprintPermissions: map[string]api.Permissions{
			"_rule_result": {"read": []string{"everyone"}},
			"service":      {"read": []string{"everyone"}},
		},
		ActionPermissions: map[string]api.Permissions{
			"a1": {"execute": []string{"everyone"}},
			"a2": {"execute": []string{"everyone"}},
		},
	}

	applyDataExclusion(data, []string{"_rule_result"}, nil, false, false)

	if len(data.Blueprints) != 1 {
		t.Errorf("expected 1 blueprint, got %d", len(data.Blueprints))
	}
	if len(data.Entities) != 1 {
		t.Errorf("expected 1 entity (deep removes resources too), got %d", len(data.Entities))
	}
	if len(data.Scorecards) != 0 {
		t.Errorf("expected 0 scorecards, got %d", len(data.Scorecards))
	}
	if len(data.Actions) != 1 {
		t.Errorf("expected 1 action (only non-excluded blueprint action kept), got %d", len(data.Actions))
	}
	if _, ok := data.BlueprintPermissions["_rule_result"]; ok {
		t.Error("expected BlueprintPermissions entry for excluded blueprint '_rule_result' to be removed")
	}
	if _, ok := data.BlueprintPermissions["service"]; !ok {
		t.Error("expected BlueprintPermissions entry for non-excluded blueprint 'service' to be present")
	}
	if _, ok := data.ActionPermissions["a1"]; ok {
		t.Error("expected ActionPermissions entry for excluded action 'a1' to be removed")
	}
	if _, ok := data.ActionPermissions["a2"]; !ok {
		t.Error("expected ActionPermissions entry for non-excluded action 'a2' to be present")
	}
}

func TestModuleExecute_DryRunStreamsEntities(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")
	content := `{
  "blueprints": [{"identifier":"service","title":"Service"}],
  "entities": [
    {"identifier":"new","blueprint":"service","title":"New"},
    {"identifier":"changed","blueprint":"service","title":"Changed"},
    {"identifier":"same","blueprint":"service","title":"Same"}
  ]
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	getEntitiesHit := false
	searchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service", "title": "Service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 10001})
		case "/blueprints/service/entities":
			getEntitiesHit = true
			http.Error(w, "unexpected GET entities call", http.StatusInternalServerError)
		case "/blueprints/service/entities/search":
			searchCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode search body: %v", err)
			}
			if body["limit"] != float64(1000) {
				t.Fatalf("expected search limit 1000, got %#v", body["limit"])
			}
			switch searchCalls {
			case 1:
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":   true,
					"next": "cursor-1",
					"entities": []map[string]interface{}{
						{"identifier": "changed", "blueprint": "service", "title": "Old"},
					},
				})
			case 2:
				if body["from"] != "cursor-1" {
					t.Fatalf("expected cursor-1, got %#v", body["from"])
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok": true,
					"entities": []map[string]interface{}{
						{"identifier": "same", "blueprint": "service", "title": "Same"},
					},
				})
			default:
				http.Error(w, "unexpected extra search call", http.StatusInternalServerError)
			}
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		APIURL:       server.URL,
	})
	result, err := module.Execute(context.Background(), Options{
		InputPath:        inputPath,
		DryRun:           true,
		IncludeResources: []string{"blueprints", "entities"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.EntitiesCreated != 1 {
		t.Fatalf("expected 1 entity create, got %d", result.EntitiesCreated)
	}
	if result.EntitiesUpdated != 1 {
		t.Fatalf("expected 1 entity update, got %d", result.EntitiesUpdated)
	}
	if getEntitiesHit {
		t.Error("GET entities endpoint should not be called for large current blueprint")
	}
	if searchCalls != 2 {
		t.Fatalf("expected 2 current-entity search calls, got %d", searchCalls)
	}
}

func TestModuleExecute_DryRunKeepsCustomSystemBlueprintPatch(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")
	content := `{
  "blueprints": [
    {
      "identifier": "_team",
      "title": "Team",
      "properties": {
        "description": {"type": "string"},
        "cost_center": {"type": "string"}
      }
    }
  ]
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{
						"identifier": "_team",
						"title":      "Team",
						"properties": map[string]interface{}{
							"description": map[string]interface{}{"type": "string"},
						},
					},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		APIURL:       server.URL,
	})
	result, err := module.Execute(context.Background(), Options{
		InputPath:            inputPath,
		DryRun:               true,
		SkipSystemBlueprints: true,
		IncludeResources:     []string{"blueprints"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.BlueprintsUpdated != 1 {
		t.Fatalf("expected 1 system blueprint property update, got %d", result.BlueprintsUpdated)
	}
}

func TestApplyDataExclusion_SchemaOnly(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "service"},
			{"identifier": "_rule_result"},
		},
		Entities: []api.Entity{
			{"identifier": "e1", "blueprint": "service"},
			{"identifier": "e2", "blueprint": "_rule_result"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "sc1", "blueprintIdentifier": "_rule_result"},
		},
		Actions: []api.Action{
			{"identifier": "a1", "blueprint": "_rule_result"},
		},
	}

	applyDataExclusion(data, nil, []string{"_rule_result"}, false, false)

	if len(data.Blueprints) != 1 {
		t.Errorf("expected 1 blueprint (schema removed), got %d", len(data.Blueprints))
	}
	// Schema-only: entities/scorecards/actions for _rule_result are KEPT
	if len(data.Entities) != 2 {
		t.Errorf("expected 2 entities (schema-only keeps resources), got %d", len(data.Entities))
	}
	if len(data.Scorecards) != 1 {
		t.Errorf("expected 1 scorecard (schema-only keeps resources), got %d", len(data.Scorecards))
	}
	if len(data.Actions) != 1 {
		t.Errorf("expected 1 action (schema-only keeps resources), got %d", len(data.Actions))
	}
}

func TestApplyDataExclusion_NoExclude(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{{"identifier": "service"}},
		Entities:   []api.Entity{{"identifier": "e1", "blueprint": "service"}},
	}
	applyDataExclusion(data, nil, nil, false, false)
	if len(data.Blueprints) != 1 || len(data.Entities) != 1 {
		t.Error("empty exclusion lists should leave data unchanged")
	}
}

func TestIsSidebarParentNotFound(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("some other error"), false},
		{errors.New(`{"error":"not_found","message":"Sidebar item with parent \"initiatives\" was not found"}`), true},
		{errors.New("Sidebar item not found"), true},
	}
	for _, c := range cases {
		got := isSidebarParentNotFound(c.err)
		if got != c.expected {
			t.Errorf("isSidebarParentNotFound(%v) = %v, want %v", c.err, got, c.expected)
		}
	}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *api.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: srv.URL, Timeout: 0})
	return srv, client
}

func authHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/auth/access_token" {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		return true
	}
	return false
}

// TestImportPages_PreservesTypeOnCreate verifies that `type` and navigation fields are
// sent to Port when creating a new page.
func TestImportPages_PreservesTypeOnCreate(t *testing.T) {
	var receivedPage map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/pages" {
			json.NewDecoder(r.Body).Decode(&receivedPage)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": receivedPage})
			return
		}
		http.NotFound(w, r)
	})

	page := api.Page{
		"identifier":          "aws_cost_overview",
		"type":                "dashboard",
		"parent":              "initiatives",
		"sidebar":             "catalog",
		"after":               "mastering_the_estate",
		"requiredQueryParams": []interface{}{},
		"title":               "AWS Cost Overview",
		"widgets":             []interface{}{},
		"createdBy":           "user_abc",
		"createdAt":           "2026-01-01",
		"id":                  "internal-id",
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importPages(context.Background(), []api.Page{page}, result)

	if result.PagesCreated != 1 {
		t.Fatalf("expected 1 page created, got %d", result.PagesCreated)
	}
	if receivedPage["type"] != "dashboard" {
		t.Errorf("expected type=dashboard to be sent on create, got %v", receivedPage["type"])
	}
	if receivedPage["parent"] != "initiatives" {
		t.Errorf("expected parent=initiatives to be sent on create, got %v", receivedPage["parent"])
	}
	if receivedPage["sidebar"] != "catalog" {
		t.Errorf("expected sidebar=catalog to be sent on create, got %v", receivedPage["sidebar"])
	}
	// System/audit fields must be stripped
	if receivedPage["createdBy"] != nil {
		t.Errorf("expected createdBy to be stripped, got %v", receivedPage["createdBy"])
	}
}

// TestImportPages_UpdateSendsNavFields verifies that page updates include navigation
// fields (after, parent, sidebar) so Port moves the page to the correct sidebar position,
// and that `type` is stripped because the PATCH endpoint rejects it.
func TestImportPages_UpdateSendsNavFields(t *testing.T) {
	postCalls := 0
	var patchBody map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/pages" {
			postCalls++
			// Always return conflict so the importer falls through to update.
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false, "error": "conflict",
			})
			return
		}
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/aws_cost_overview" {
			json.NewDecoder(r.Body).Decode(&patchBody)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": patchBody})
			return
		}
		// GetPage — return empty existing page so agentIdentifier merge is a no-op.
		if r.Method == http.MethodGet && r.URL.Path == "/pages/aws_cost_overview" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "aws_cost_overview"}})
			return
		}
		http.NotFound(w, r)
	})

	page := api.Page{
		"identifier": "aws_cost_overview",
		"type":       "dashboard",
		"parent":     "initiatives",
		"sidebar":    "catalog",
		"after":      "mastering_the_estate",
		"title":      "AWS Cost Overview",
		"widgets":    []interface{}{},
		"createdBy":  "user_abc",
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importPages(context.Background(), []api.Page{page}, result)

	if result.PagesUpdated != 1 {
		t.Fatalf("expected 1 page updated, got %d (created=%d)", result.PagesUpdated, result.PagesCreated)
	}
	// Relevant placement fields must be present in the PATCH body.
	if patchBody["parent"] != "initiatives" {
		t.Errorf("expected parent=initiatives in update, got %v", patchBody["parent"])
	}
	if patchBody["sidebar"] != nil {
		t.Errorf("expected sidebar to be stripped from update, got %v", patchBody["sidebar"])
	}
	// `after` must be present in the PATCH body (ordering is applied inline, not in a second pass).
	if patchBody["after"] != "mastering_the_estate" {
		t.Errorf("expected after=mastering_the_estate in update, got %v", patchBody["after"])
	}
	// type must be stripped from PATCH.
	if patchBody["type"] != nil {
		t.Errorf("expected type to be stripped from update, got %v", patchBody["type"])
	}
	// Audit fields must be stripped.
	if patchBody["createdBy"] != nil {
		t.Errorf("expected createdBy to be stripped from update, got %v", patchBody["createdBy"])
	}
}

// TestImportPages_UpdateFallsBackWithoutAfterWhenSiblingMissing verifies that when
// Port rejects a page update because the `after` sibling is missing, we retry
// without only the `after` field while preserving parent placement.
func TestImportPages_UpdateFallsBackWithoutAfterWhenSiblingMissing(t *testing.T) {
	patchCalls := 0
	var secondPatchBody map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/pages" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "conflict"})
			return
		}
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/aws_cost_overview" {
			patchCalls++
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if patchCalls == 1 {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "not_found",
					"message": `Sidebar item with after "mastering_the_estate" was not found`,
				})
				return
			}
			secondPatchBody = body
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": body})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/pages/aws_cost_overview" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "aws_cost_overview"}})
			return
		}
		http.NotFound(w, r)
	})

	page := api.Page{
		"identifier": "aws_cost_overview",
		"type":       "dashboard",
		"parent":     "initiatives",
		"sidebar":    "catalog",
		"after":      "mastering_the_estate",
		"title":      "AWS Cost Overview",
		"widgets":    []interface{}{},
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importPages(context.Background(), []api.Page{page}, result)

	if patchCalls != 2 {
		t.Fatalf("expected 2 PATCH attempts, got %d", patchCalls)
	}
	if result.PagesUpdated != 1 {
		t.Fatalf("expected 1 page updated, got %d", result.PagesUpdated)
	}
	// Only the invalid `after` field should be stripped on the fallback PATCH.
	if secondPatchBody["after"] != nil {
		t.Errorf("expected after to be stripped on fallback update, got %v", secondPatchBody["after"])
	}
	if secondPatchBody["parent"] != "initiatives" {
		t.Errorf("expected parent to be preserved on fallback update, got %v", secondPatchBody["parent"])
	}
}

// TestImportPages_NullNavFieldsNotSentOnUpdate verifies that when the source page has
// null nav fields (e.g. exported from an org where those fields weren't captured),
// the PATCH request does NOT include those null fields — sending null would clear the
// page's existing navigation context in Port.
func TestImportPages_NullNavFieldsNotSentOnUpdate(t *testing.T) {
	var patchBody map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/pages" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "conflict"})
			return
		}
		if r.Method == http.MethodPatch && r.URL.Path == "/pages/aws_cost_overview" {
			json.NewDecoder(r.Body).Decode(&patchBody)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": patchBody})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/pages/aws_cost_overview" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "aws_cost_overview"}})
			return
		}
		http.NotFound(w, r)
	})

	// Source page has null for all nav fields (common in exports from orgs that don't capture them)
	page := api.Page{
		"identifier":          "aws_cost_overview",
		"type":                nil, // null
		"parent":              nil, // null
		"sidebar":             nil, // null
		"after":               nil, // null
		"requiredQueryParams": nil, // null
		"title":               "AWS Cost Overview",
		"widgets":             []interface{}{},
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importPages(context.Background(), []api.Page{page}, result)

	if result.PagesUpdated != 1 {
		t.Fatalf("expected 1 page updated, got %d", result.PagesUpdated)
	}
	// Null string nav fields must NOT be sent in PATCH (would clear existing values)
	if _, exists := patchBody["parent"]; exists {
		t.Errorf("expected null parent to be stripped from PATCH, got %v", patchBody["parent"])
	}
	if _, exists := patchBody["sidebar"]; exists {
		t.Errorf("expected null sidebar to be stripped from PATCH, got %v", patchBody["sidebar"])
	}
	if _, exists := patchBody["after"]; exists {
		t.Errorf("expected null after to be stripped from PATCH, got %v", patchBody["after"])
	}
	// requiredQueryParams: null must be stripped from PATCH (not sent as null or []).
	if _, exists := patchBody["requiredQueryParams"]; exists {
		t.Errorf("expected null requiredQueryParams to be stripped from PATCH, got %v", patchBody["requiredQueryParams"])
	}
	// type is always stripped from PATCH regardless
	if _, exists := patchBody["type"]; exists {
		t.Errorf("expected type to be stripped from PATCH, got %v", patchBody["type"])
	}
}

func TestImportBlueprints_RestoresOwnershipAfterCreate(t *testing.T) {
	var createBody map[string]interface{}
	var ownershipPatchBody map[string]interface{}
	var mu sync.Mutex

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints":
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": createBody})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "service",
					"title":      "Service",
					"relations": map[string]interface{}{
						"system": map[string]interface{}{"target": "system"},
					},
				},
			})
			return
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}

			mu.Lock()
			if ownership, ok := body["ownership"].(map[string]interface{}); ok && ownership["type"] == "Inherited" {
				ownershipPatchBody = body
			}
			mu.Unlock()

			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"title":      "Service",
			"relations": map[string]interface{}{
				"system": map[string]interface{}{"target": "system"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "system.$identifier",
			},
		},
	}

	if err := importer.importBlueprints(context.Background(), blueprints, result); err != nil {
		t.Fatalf("importBlueprints returned error: %v", err)
	}

	if createBody["ownership"] != nil {
		t.Fatalf("expected ownership to be deferred during create, got %v", createBody["ownership"])
	}
	if ownershipPatchBody == nil {
		t.Fatal("expected ownership to be restored in a later blueprint update")
	}

	ownership, ok := ownershipPatchBody["ownership"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ownership in update body, got %T", ownershipPatchBody["ownership"])
	}
	if ownership["type"] != "Inherited" {
		t.Fatalf("expected ownership type Inherited, got %v", ownership["type"])
	}
	if ownership["path"] != "system.$identifier" {
		t.Fatalf("expected ownership path system.$identifier, got %v", ownership["path"])
	}
}

func TestImportBlueprints_AppliesOwnershipInDependencyOrder(t *testing.T) {
	var mu sync.Mutex
	var ownershipUpdateOrder []string

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "deployment"},
					{"identifier": "pod"},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": "service", "title": "Service"},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/deployment":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "deployment",
					"title":      "Deployment",
					"relations": map[string]interface{}{
						"service": map[string]interface{}{"target": "service"},
					},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/pod":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "pod",
					"title":      "Pod",
					"relations": map[string]interface{}{
						"deployment": map[string]interface{}{"target": "deployment"},
					},
				},
			})
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			if _, ok := body["ownership"].(map[string]interface{}); ok {
				mu.Lock()
				ownershipUpdateOrder = append(ownershipUpdateOrder, body["identifier"].(string))
				mu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"title":      "Service",
			"ownership":  map[string]interface{}{"type": "Direct"},
		},
		{
			"identifier": "deployment",
			"title":      "Deployment",
			"relations": map[string]interface{}{
				"service": map[string]interface{}{"target": "service"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "service.$identifier",
			},
		},
		{
			"identifier": "pod",
			"title":      "Pod",
			"relations": map[string]interface{}{
				"deployment": map[string]interface{}{"target": "deployment"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "deployment.$identifier",
			},
		},
	}

	if err := importer.importBlueprints(context.Background(), blueprints, result); err != nil {
		t.Fatalf("importBlueprints returned error: %v", err)
	}

	if len(ownershipUpdateOrder) != 3 {
		t.Fatalf("expected 3 ownership updates, got %d (%v)", len(ownershipUpdateOrder), ownershipUpdateOrder)
	}

	expectedOrder := []string{"service", "deployment", "pod"}
	for i, id := range expectedOrder {
		if ownershipUpdateOrder[i] != id {
			t.Fatalf("expected ownership update %d to be %s, got %s", i, id, ownershipUpdateOrder[i])
		}
	}
}

// TestSortPagesByAfterDeps verifies topological sort respects after-dependencies.
func TestSortPagesByAfterDeps(t *testing.T) {
	// Chain: alpha <- beta <- gamma (beta after alpha, gamma after beta)
	pages := []api.Page{
		{"identifier": "gamma", "after": "beta"},
		{"identifier": "alpha"},
		{"identifier": "beta", "after": "alpha"},
	}
	sorted := sortPagesByAfterDeps(pages)

	// Build position map
	pos := make(map[string]int)
	for i, p := range sorted {
		pos[p["identifier"].(string)] = i
	}

	if pos["alpha"] >= pos["beta"] {
		t.Errorf("expected alpha before beta, got alpha=%d beta=%d", pos["alpha"], pos["beta"])
	}
	if pos["beta"] >= pos["gamma"] {
		t.Errorf("expected beta before gamma, got beta=%d gamma=%d", pos["beta"], pos["gamma"])
	}
}

// TestImportPages_OrderingRespectedInline verifies that importPages processes pages
// in topological `after` order so that `after` targets exist before dependents.
func TestImportPages_OrderingRespectedInline(t *testing.T) {
	var mu sync.Mutex
	var patchOrder []string

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		// All pages "already exist" — POST returns 409 conflict → update path
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "conflict"})
			return
		}
		if r.Method == http.MethodGet {
			pageID := r.URL.Path[len("/pages/"):]
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": pageID}})
			return
		}
		if r.Method == http.MethodPatch {
			mu.Lock()
			patchOrder = append(patchOrder, r.URL.Path)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{}})
			return
		}
		http.NotFound(w, r)
	})

	// gamma depends on beta, beta depends on alpha
	pages := []api.Page{
		{"identifier": "gamma", "after": "beta", "title": "Gamma"},
		{"identifier": "alpha", "title": "Alpha"},
		{"identifier": "beta", "after": "alpha", "title": "Beta"},
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importPages(context.Background(), pages, result)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// alpha has no dependency — it must come before beta, beta before gamma
	pos := make(map[string]int)
	for i, path := range patchOrder {
		switch path {
		case "/pages/alpha":
			pos["alpha"] = i
		case "/pages/beta":
			pos["beta"] = i
		case "/pages/gamma":
			pos["gamma"] = i
		}
	}
	if pos["alpha"] >= pos["beta"] {
		t.Errorf("expected alpha before beta, got alpha=%d beta=%d", pos["alpha"], pos["beta"])
	}
	if pos["beta"] >= pos["gamma"] {
		t.Errorf("expected beta before gamma, got beta=%d gamma=%d", pos["beta"], pos["gamma"])
	}
}

func TestImportFolders_CreatedBeforePages(t *testing.T) {
	var calls []string
	var folderPayloads []map[string]interface{}
	var mu sync.Mutex

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sidebars/catalog/folders":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode folder body: %v", err)
			}
			mu.Lock()
			calls = append(calls, "folder")
			folderPayloads = append(folderPayloads, body)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/pages":
			mu.Lock()
			calls = append(calls, "page")
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "service_overview"}})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	data := &export.Data{
		Blueprints: []api.Blueprint{{"identifier": "service", "title": "Service"}},
		Folders: []api.Folder{
			{"identifier": "root", "title": "Root"},
			{"identifier": "child", "title": "Child", "parent": "root"},
		},
		Pages: []api.Page{{"identifier": "service_overview", "title": "Service Overview", "parent": "root"}},
	}

	result = &Result{}
	if err := importer.importOtherResources(context.Background(), data, Options{IncludeResources: []string{"pages"}}, result); err != nil {
		t.Fatalf("importOtherResources error: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d (%v)", len(calls), calls)
	}
	if calls[0] != "folder" {
		t.Fatalf("expected the root folder to be created before any page, got %v", calls)
	}
	if len(folderPayloads) != 2 {
		t.Fatalf("expected 2 folder payloads, got %d", len(folderPayloads))
	}
	if folderPayloads[0]["identifier"] != "root" {
		t.Fatalf("expected root folder first, got %v", folderPayloads[0]["identifier"])
	}
	if folderPayloads[1]["identifier"] != "child" {
		t.Fatalf("expected child folder to be created, got %v", folderPayloads[1]["identifier"])
	}
	if folderPayloads[1]["parent"] != "root" {
		t.Fatalf("expected child folder payload to preserve parent=root, got %v", folderPayloads[1]["parent"])
	}
	if result.PagesCreated != 1 {
		t.Fatalf("expected 1 page created, got %d", result.PagesCreated)
	}
}

func createTempConfig(t *testing.T) *config.ConfigManager {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	return config.NewConfigManager(configPath)
}

func TestImportOtherResources_SkipEntities_SkipsTeamsAndUsers(t *testing.T) {
	teamsHit := false
	usersHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case strings.HasPrefix(r.URL.Path, "/teams") && r.Method == http.MethodPost:
			teamsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case strings.HasPrefix(r.URL.Path, "/users") && r.Method == http.MethodPatch:
			usersHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	importer := NewImporter(client)
	data := &export.Data{
		Teams: []api.Team{{"identifier": "t1", "name": "Team1"}},
		Users: []api.User{{"email": "u@example.com"}},
	}
	result := &Result{}
	opts := Options{SkipEntities: true}
	if err := importer.importOtherResources(context.Background(), data, opts, result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if teamsHit {
		t.Error("teams import should not be called when SkipEntities=true")
	}
	if usersHit {
		t.Error("users import should not be called when SkipEntities=true")
	}
}

func TestApplyDataExclusion_SkipSystemBlueprints(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "_user"},
			{"identifier": "_team"},
			{"identifier": "service"},
		},
		Entities: []api.Entity{
			{"identifier": "u1", "blueprint": "_user"},
			{"identifier": "s1", "blueprint": "service"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "sc1", "blueprintIdentifier": "_user"},
		},
		Actions: []api.Action{
			{"identifier": "a1", "blueprint": "_user"},
		},
		BlueprintPermissions: map[string]api.Permissions{
			"_user":   {"read": []string{"everyone"}},
			"service": {"read": []string{"everyone"}},
		},
	}

	applyDataExclusion(data, nil, nil, true, true)

	// _* blueprint schemas removed
	if len(data.Blueprints) != 1 {
		t.Errorf("expected 1 blueprint (service only), got %d", len(data.Blueprints))
	}
	if id, _ := data.Blueprints[0]["identifier"].(string); id != "service" {
		t.Errorf("expected remaining blueprint to be 'service', got %q", id)
	}

	// _* entities removed
	if len(data.Entities) != 1 {
		t.Errorf("expected 1 entity (s1 only), got %d", len(data.Entities))
	}

	// Scorecards for _user STILL present (shallow skip)
	if len(data.Scorecards) != 1 {
		t.Errorf("expected 1 scorecard (shallow skip keeps scorecards), got %d", len(data.Scorecards))
	}

	// Actions for _user STILL present
	if len(data.Actions) != 1 {
		t.Errorf("expected 1 action (shallow skip keeps actions), got %d", len(data.Actions))
	}

	// Blueprint permissions for _user STILL present
	if _, ok := data.BlueprintPermissions["_user"]; !ok {
		t.Error("blueprint permissions for _user should be kept (shallow skip)")
	}
}

func TestApplyDataExclusion_SkipSystemBlueprintsKeepsCustomSystemPatch(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "_user",
				"properties": map[string]interface{}{
					"status":     map[string]interface{}{"type": "string"},
					"department": map[string]interface{}{"type": "string"},
				},
			},
			{
				"identifier": "_unknown",
				"properties": map[string]interface{}{
					"custom": map[string]interface{}{"type": "string"},
				},
			},
			{"identifier": "service"},
		},
		Entities: []api.Entity{
			{"identifier": "u1", "blueprint": "_user"},
			{"identifier": "s1", "blueprint": "service"},
		},
	}

	applyDataExclusion(data, nil, nil, true, false)

	if len(data.Blueprints) != 2 {
		t.Fatalf("expected _user patch and service blueprint, got %#v", data.Blueprints)
	}
	userPatch := data.Blueprints[0]
	if userPatch["identifier"] != "_user" {
		t.Fatalf("expected _user patch first, got %#v", userPatch)
	}
	props := userPatch["properties"].(map[string]interface{})
	if _, ok := props["status"]; ok {
		t.Fatal("managed _user status property should be stripped")
	}
	if _, ok := props["department"]; !ok {
		t.Fatal("custom _user department property should be preserved")
	}
	if len(data.Entities) != 1 || data.Entities[0]["blueprint"] != "service" {
		t.Fatalf("expected only non-system entities to remain, got %#v", data.Entities)
	}
}

func TestSanitizeTeamFields_NullDescription(t *testing.T) {
	team := api.Team{
		"name":        "my-team",
		"description": nil,
		"color":       "#ff0000",
	}

	result := sanitizeTeamFields(team)

	if _, exists := result["description"]; exists {
		t.Error("nil description should be removed from team map")
	}
	if result["name"] != "my-team" {
		t.Error("non-nil fields should be preserved")
	}
	if result["color"] != "#ff0000" {
		t.Error("non-nil fields should be preserved")
	}
}

func TestSanitizeTeamFields_NoNulls(t *testing.T) {
	team := api.Team{
		"name":        "my-team",
		"description": "A great team",
	}

	result := sanitizeTeamFields(team)

	if result["description"] != "A great team" {
		t.Error("non-nil description should be preserved")
	}
}

func TestImportPermissions_CountsOnlySuccesses(t *testing.T) {
	// bp1/action1 succeed; bp2/action2 fail — only successes should be counted.
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		switch r.URL.Path {
		case "/blueprints/bp1/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case "/blueprints/bp2/permissions":
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "server_error"})
		case "/actions/action1/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		case "/actions/action2/permissions":
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "server_error"})
		}
	})

	importer := NewImporter(client)
	diff := &DiffResult{
		BlueprintPermissions: []PermissionsChange{
			{Identifier: "bp1", Permissions: api.Permissions{"read": "everyone"}},
			{Identifier: "bp2", Permissions: api.Permissions{"read": "everyone"}},
		},
		ActionPermissions: []PermissionsChange{
			{Identifier: "action1", Permissions: api.Permissions{"read": "everyone"}},
			{Identifier: "action2", Permissions: api.Permissions{"read": "everyone"}},
		},
	}

	bpUpdated, actionUpdated, pageUpdated, _ := importer.importPermissions(context.Background(), diff)

	if bpUpdated != 1 {
		t.Errorf("expected 1 blueprint permission updated, got %d", bpUpdated)
	}
	if actionUpdated != 1 {
		t.Errorf("expected 1 action permission updated, got %d", actionUpdated)
	}
	if pageUpdated != 0 {
		t.Errorf("expected 0 page permissions updated, got %d", pageUpdated)
	}
	errs := importer.errors.ToStringSlice()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors (one per failing permission), got %d: %v", len(errs), errs)
	}
}

func TestIsInvalidPermissionsError(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("some other error"), false},
		{errors.New("422 Unprocessable Entity"), false},
		{errors.New(`API request to /blueprints/_rule_result/permissions PATCH failed: 422 Unprocessable Entity. Body: {"ok":false,"error":"invalid_permissions","message":"You cannot update permissions on unknown fields"}`), true},
	}
	for _, c := range cases {
		got := isInvalidPermissionsError(c.err)
		if got != c.expected {
			t.Errorf("isInvalidPermissionsError(%v) = %v, want %v", c.err, got, c.expected)
		}
	}
}

func TestParseInvalidPermissionFields(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		r, p := ParseInvalidPermissionFields(nil)
		if r != nil || p != nil {
			t.Errorf("expected nil, nil; got %v, %v", r, p)
		}
	})

	t.Run("non-matching error", func(t *testing.T) {
		r, p := ParseInvalidPermissionFields(errors.New("something else"))
		if r != nil || p != nil {
			t.Errorf("expected nil, nil; got %v, %v", r, p)
		}
	})

	t.Run("exact error from bug report", func(t *testing.T) {
		err := errors.New(`API request to https://api.us.getport.io/v1/blueprints/_rule_result/permissions PATCH failed: 422 Unprocessable Entity. Body: {"ok":false,"error":"invalid_permissions","message":"You cannot update permissions on unknown fields","details":{"invalidProperties":[],"invalidRelations":["_sonarQubeProject"]}}`)
		relations, properties := ParseInvalidPermissionFields(err)
		if len(relations) != 1 || relations[0] != "_sonarQubeProject" {
			t.Errorf("expected relations=[_sonarQubeProject], got %v", relations)
		}
		if len(properties) != 0 {
			t.Errorf("expected empty properties, got %v", properties)
		}
	})

	t.Run("multiple invalid fields", func(t *testing.T) {
		err := errors.New(`API request to /blueprints/bp1/permissions PATCH failed: 422 Unprocessable Entity. Body: {"ok":false,"error":"invalid_permissions","message":"msg","details":{"invalidProperties":["prop1","prop2"],"invalidRelations":["rel1","rel2"]}}`)
		relations, properties := ParseInvalidPermissionFields(err)
		if len(relations) != 2 || relations[0] != "rel1" || relations[1] != "rel2" {
			t.Errorf("expected relations=[rel1 rel2], got %v", relations)
		}
		if len(properties) != 2 || properties[0] != "prop1" || properties[1] != "prop2" {
			t.Errorf("expected properties=[prop1 prop2], got %v", properties)
		}
	})

	t.Run("non-permission error code", func(t *testing.T) {
		err := errors.New(`API request to /foo PATCH failed: 422 Unprocessable Entity. Body: {"ok":false,"error":"something_else","details":{"invalidRelations":["rel1"]}}`)
		r, p := ParseInvalidPermissionFields(err)
		if r != nil || p != nil {
			t.Errorf("expected nil for non-permission error code, got %v, %v", r, p)
		}
	})
}

func TestSanitizePermissions_TopLevel(t *testing.T) {
	original := api.Permissions{
		"entities":          map[string]interface{}{"view": []string{"$team"}},
		"_sonarQubeProject": map[string]interface{}{"connect": []string{"$team"}},
		"_anotherRelation":  map[string]interface{}{"connect": []string{"$admin"}},
		"orphanedProp":      map[string]interface{}{"read": []string{"$team"}},
	}

	cleaned := SanitizePermissions(original, []string{"_sonarQubeProject", "_anotherRelation"}, []string{"orphanedProp"})

	if _, exists := cleaned["entities"]; !exists {
		t.Error("standard scope 'entities' should be preserved")
	}
	if _, exists := cleaned["_sonarQubeProject"]; exists {
		t.Error("orphaned relation '_sonarQubeProject' should have been removed")
	}
	if _, exists := cleaned["_anotherRelation"]; exists {
		t.Error("orphaned relation '_anotherRelation' should have been removed")
	}
	if _, exists := cleaned["orphanedProp"]; exists {
		t.Error("orphaned property 'orphanedProp' should have been removed")
	}
	if len(cleaned) != 1 {
		t.Errorf("expected 1 key remaining, got %d: %v", len(cleaned), cleaned)
	}

	// Verify original is not mutated
	if len(original) != 4 {
		t.Error("original permissions should not be mutated")
	}
}

func TestSanitizePermissions_NestedUpdateRelations(t *testing.T) {
	original := api.Permissions{
		"entities": map[string]interface{}{
			"read": map[string]interface{}{"roles": []string{"Admin"}},
			"updateRelations": map[string]interface{}{
				"service": map[string]interface{}{"roles": []string{"Admin"}},
				"_test_3": map[string]interface{}{"roles": []string{"Admin"}},
				"rule":    map[string]interface{}{"roles": []string{"Admin"}},
			},
			"updateProperties": map[string]interface{}{
				"validProp":  map[string]interface{}{"roles": []string{"Admin"}},
				"orphanProp": map[string]interface{}{"roles": []string{"Admin"}},
			},
		},
	}

	cleaned := SanitizePermissions(original, []string{"_test_3"}, []string{"orphanProp"})

	entities := cleaned["entities"].(map[string]interface{})
	ur := entities["updateRelations"].(map[string]interface{})
	up := entities["updateProperties"].(map[string]interface{})

	if _, exists := ur["_test_3"]; exists {
		t.Error("orphaned relation '_test_3' should be removed from updateRelations")
	}
	if _, exists := ur["service"]; !exists {
		t.Error("valid relation 'service' should be preserved in updateRelations")
	}
	if _, exists := ur["rule"]; !exists {
		t.Error("valid relation 'rule' should be preserved in updateRelations")
	}
	if _, exists := up["orphanProp"]; exists {
		t.Error("orphaned property 'orphanProp' should be removed from updateProperties")
	}
	if _, exists := up["validProp"]; !exists {
		t.Error("valid property 'validProp' should be preserved in updateProperties")
	}

	// Verify read scope is untouched
	if _, exists := entities["read"]; !exists {
		t.Error("'read' scope should be preserved")
	}

	// Verify original is not mutated
	origUR := original["entities"].(map[string]interface{})["updateRelations"].(map[string]interface{})
	if _, exists := origUR["_test_3"]; !exists {
		t.Error("original should not be mutated")
	}
}

func TestImportPermissions_RetriesOnOrphanedFields(t *testing.T) {
	callCount := 0
	var retryBody map[string]interface{}
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.URL.Path == "/blueprints/_rule_result/permissions" && r.Method == "PATCH" {
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "invalid_permissions",
					"message": "You cannot update permissions on unknown fields",
					"details": map[string]interface{}{
						"invalidProperties": []string{},
						"invalidRelations":  []string{"_sonarQubeProject"},
					},
				})
				return
			}
			// Capture the retried payload for verification
			json.NewDecoder(r.Body).Decode(&retryBody)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
			return
		}
	})

	importer := NewImporter(client)
	diff := &DiffResult{
		BlueprintPermissions: []PermissionsChange{
			{
				Identifier: "_rule_result",
				Permissions: api.Permissions{
					"entities": map[string]interface{}{
						"read": map[string]interface{}{"roles": []string{"Admin"}},
						"updateRelations": map[string]interface{}{
							"service":           map[string]interface{}{"roles": []string{"Admin"}},
							"_sonarQubeProject": map[string]interface{}{"roles": []string{"Admin"}},
						},
					},
				},
			},
		},
	}

	bpUpdated, _, _, warnings := importer.importPermissions(context.Background(), diff)

	if bpUpdated != 1 {
		t.Errorf("expected 1 blueprint permission updated after retry, got %d", bpUpdated)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (original + retry), got %d", callCount)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning about stripped fields, got %d", len(warnings))
	}
	if len(warnings) > 0 && !strings.Contains(warnings[0], "_sonarQubeProject") {
		t.Errorf("warning should mention the stripped relation, got: %s", warnings[0])
	}
	errs := importer.errors.ToStringSlice()
	if len(errs) != 0 {
		t.Errorf("expected no errors after successful retry, got: %v", errs)
	}

	// Verify the retried payload had _sonarQubeProject stripped from updateRelations
	if retryBody == nil {
		t.Fatal("retryBody should have been captured on the second API call — retry never fired")
	}
	if entities, ok := retryBody["entities"].(map[string]interface{}); ok {
		if ur, ok := entities["updateRelations"].(map[string]interface{}); ok {
			if _, exists := ur["_sonarQubeProject"]; exists {
				t.Error("retried payload should NOT contain _sonarQubeProject in updateRelations")
			}
			if _, exists := ur["service"]; !exists {
				t.Error("retried payload should still contain 'service' in updateRelations")
			}
		}
	}
}

func TestImportPermissions_RetryStillFails(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.URL.Path == "/blueprints/bp1/permissions" && r.Method == "PATCH" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"error":   "invalid_permissions",
				"message": "You cannot update permissions on unknown fields",
				"details": map[string]interface{}{
					"invalidProperties": []string{},
					"invalidRelations":  []string{"badRel"},
				},
			})
			return
		}
	})

	importer := NewImporter(client)
	diff := &DiffResult{
		BlueprintPermissions: []PermissionsChange{
			{
				Identifier:  "bp1",
				Permissions: api.Permissions{"entities": "view", "badRel": "connect"},
			},
		},
	}

	bpUpdated, _, _, warnings := importer.importPermissions(context.Background(), diff)

	if bpUpdated != 0 {
		t.Errorf("expected 0 updated (retry also failed), got %d", bpUpdated)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	errs := importer.errors.ToStringSlice()
	if len(errs) != 1 {
		t.Errorf("expected 1 error from failed retry, got %d: %v", len(errs), errs)
	}
}

func TestImportPermissions_PagePermissions_RetriesOnOrphanedFields(t *testing.T) {
	callCount := 0
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.URL.Path == "/pages/home/permissions" && r.Method == "PATCH" {
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "invalid_permissions",
					"message": "You cannot update permissions on unknown fields",
					"details": map[string]interface{}{
						"invalidProperties": []string{},
						"invalidRelations":  []string{"staleRelation"},
					},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
			return
		}
	})

	importer := NewImporter(client)
	diff := &DiffResult{
		PagePermissions: []PermissionsChange{
			{
				Identifier: "home",
				Permissions: api.Permissions{
					"read": map[string]interface{}{
						"roles": []string{"Admin"},
					},
					"staleRelation": map[string]interface{}{
						"roles": []string{"Admin"},
					},
				},
			},
		},
	}

	_, _, pageUpdated, warnings := importer.importPermissions(context.Background(), diff)

	if pageUpdated != 1 {
		t.Errorf("expected 1 page permission updated after retry, got %d", pageUpdated)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (original + retry), got %d", callCount)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning about stripped fields, got %d", len(warnings))
	}
	if len(warnings) > 0 && !strings.Contains(warnings[0], "staleRelation") {
		t.Errorf("warning should mention the stripped relation, got: %s", warnings[0])
	}
	errs := importer.errors.ToStringSlice()
	if len(errs) != 0 {
		t.Errorf("expected no errors after successful retry, got: %v", errs)
	}
}

func TestImportPermissions_PagePermissions_RetryStillFails(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.URL.Path == "/pages/home/permissions" && r.Method == "PATCH" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"error":   "invalid_permissions",
				"message": "You cannot update permissions on unknown fields",
				"details": map[string]interface{}{
					"invalidProperties": []string{},
					"invalidRelations":  []string{"badRel"},
				},
			})
			return
		}
	})

	importer := NewImporter(client)
	diff := &DiffResult{
		PagePermissions: []PermissionsChange{
			{
				Identifier:  "home",
				Permissions: api.Permissions{"read": "view", "badRel": "connect"},
			},
		},
	}

	_, _, pageUpdated, warnings := importer.importPermissions(context.Background(), diff)

	if pageUpdated != 0 {
		t.Errorf("expected 0 updated (retry also failed), got %d", pageUpdated)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	errs := importer.errors.ToStringSlice()
	if len(errs) != 1 {
		t.Errorf("expected 1 error from failed retry, got %d: %v", len(errs), errs)
	}
}

// TestImportBlueprints_Phase1MergePreservesRelations verifies the core safety
// mechanism: Phase 1 strips relations for ordering, but the fetch-and-merge on
// the 409 update path preserves the existing relation definitions so entity
// relation data is never cascade-deleted by a bare PUT.
// Phase 2a then restores the import file's relations as the final state.
func TestImportBlueprints_Phase1MergePreservesRelations(t *testing.T) {
	var mu sync.Mutex
	var putBodies []map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints":
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Conflict"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "service",
					"title":      "Service",
					"relations": map[string]interface{}{
						"system": map[string]interface{}{"target": "system"},
					},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}, {"identifier": "system"}},
			})
			return
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			putBodies = append(putBodies, body)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"title":      "Service",
			"relations": map[string]interface{}{
				"system": map[string]interface{}{"target": "system"},
			},
		},
	}

	if err := importer.importBlueprints(context.Background(), blueprints, result); err != nil {
		t.Fatalf("importBlueprints returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(putBodies) == 0 {
		t.Fatal("expected at least one PUT to /blueprints/service but none was recorded")
	}

	// Phase 1 PUT: relations were stripped from the import payload, but the
	// fetch-and-merge should have preserved the existing relations from the target.
	phase1Body := putBodies[0]
	relations, ok := phase1Body["relations"].(map[string]interface{})
	if !ok || len(relations) == 0 {
		t.Fatalf("Phase 1 PUT should preserve existing relations via merge, got %v", phase1Body["relations"])
	}
	if _, hasSystem := relations["system"]; !hasSystem {
		t.Fatalf("Phase 1 PUT should preserve 'system' relation, got %v", relations)
	}

	// Phase 2a PUT: should contain the import file's relations.
	if len(putBodies) < 2 {
		t.Fatal("expected a Phase 2a PUT to restore import relations")
	}
	phase2Body := putBodies[1]
	phase2Rels, ok := phase2Body["relations"].(map[string]interface{})
	if !ok || len(phase2Rels) == 0 {
		t.Fatalf("Phase 2a PUT should contain import relations, got %v", phase2Body["relations"])
	}
}

// TestImportBlueprints_FetchAndMerge_PreservesExistingFields verifies that when
// a blueprint update is triggered (409 on create), the existing blueprint is fetched
// and merged so that fields absent from the import payload are not destroyed.
func TestImportBlueprints_FetchAndMerge_PreservesExistingFields(t *testing.T) {
	var mu sync.Mutex
	var putBody map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints":
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Conflict"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "service",
					"title":      "Service",
					"relations": map[string]interface{}{
						"system": map[string]interface{}{"target": "system"},
					},
					"calculationProperties": map[string]interface{}{
						"daysSinceCreation": map[string]interface{}{"type": "number"},
					},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}, {"identifier": "system"}},
			})
			return
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			putBody = body
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}

	// Import a blueprint that has been stripped of relations and dependent
	// fields (simulating what Phase 1 does when skipEntities=false).
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"title":      "Service Updated",
		},
	}

	if err := importer.importBlueprints(context.Background(), blueprints, result); err != nil {
		t.Fatalf("importBlueprints returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if putBody == nil {
		t.Fatal("expected a PUT to /blueprints/service but none was recorded")
	}

	// The title should be updated
	if putBody["title"] != "Service Updated" {
		t.Fatalf("expected title 'Service Updated', got %v", putBody["title"])
	}

	// Relations from the existing blueprint should be preserved (merged in)
	relations, ok := putBody["relations"].(map[string]interface{})
	if !ok || len(relations) == 0 {
		t.Fatalf("expected existing relations to be preserved in PUT body, got %v", putBody["relations"])
	}
	if _, hasSystem := relations["system"]; !hasSystem {
		t.Fatalf("expected 'system' relation to be preserved, got %v", relations)
	}

	// calculationProperties from the existing blueprint should also be preserved
	calcProps, ok := putBody["calculationProperties"].(map[string]interface{})
	if !ok || len(calcProps) == 0 {
		t.Fatalf("expected existing calculationProperties to be preserved, got %v", putBody["calculationProperties"])
	}
}

// TestImportScorecards_CreateSuccess verifies that new scorecards are created
// via POST and counted correctly.
func TestImportScorecards_CreateSuccess(t *testing.T) {
	var mu sync.Mutex
	var createdIDs []string

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints/service/scorecards":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			createdIDs = append(createdIDs, body["identifier"].(string))
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecard": body})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	pool := NewWorkerPool(1)

	scorecards := []api.Scorecard{
		{"identifier": "readiness", "blueprintIdentifier": "service", "title": "Readiness"},
		{"identifier": "quality", "blueprintIdentifier": "service", "title": "Quality"},
	}

	importer.importScorecards(context.Background(), scorecards, result, pool)
	pool.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(createdIDs) != 2 {
		t.Fatalf("expected 2 POST calls, got %d: %v", len(createdIDs), createdIDs)
	}
	if result.ScorecardsCreated != 2 {
		t.Fatalf("expected ScorecardsCreated=2, got %d", result.ScorecardsCreated)
	}
	if result.ScorecardsUpdated != 0 {
		t.Fatalf("expected ScorecardsUpdated=0, got %d", result.ScorecardsUpdated)
	}
}

// TestImportScorecards_ConflictUsesFetchMergePUT verifies that when a
// scorecard already exists (409 on create), the importer fetches the full
// set, merges the updates, and performs a single bulk PUT instead of using
// the non-existent PATCH endpoint.
func TestImportScorecards_ConflictUsesFetchMergePUT(t *testing.T) {
	var mu sync.Mutex
	bulkPutCalled := false
	var bulkPutBody []map[string]interface{}

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints/service/scorecards":
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Conflict"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []interface{}{
					map[string]interface{}{"identifier": "readiness", "title": "Old Readiness"},
					map[string]interface{}{"identifier": "quality", "title": "Old Quality"},
					map[string]interface{}{"identifier": "sibling", "title": "Sibling"},
				},
			})
			return
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service/scorecards":
			mu.Lock()
			bulkPutCalled = true
			var body []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			bulkPutBody = body
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecards": []interface{}{}})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	pool := NewWorkerPool(1)

	scorecards := []api.Scorecard{
		{"identifier": "readiness", "blueprintIdentifier": "service", "title": "Readiness"},
		{"identifier": "quality", "blueprintIdentifier": "service", "title": "Quality"},
	}

	importer.importScorecards(context.Background(), scorecards, result, pool)
	pool.Wait()

	mu.Lock()
	defer mu.Unlock()

	if !bulkPutCalled {
		t.Fatal("expected bulk PUT to be called for fetch-merge-PUT flow")
	}
	if len(bulkPutBody) != 3 {
		t.Fatalf("expected 3 scorecards in bulk PUT (2 updated + 1 sibling), got %d", len(bulkPutBody))
	}
	if result.ScorecardsUpdated != 2 {
		t.Fatalf("expected ScorecardsUpdated=2, got %d", result.ScorecardsUpdated)
	}
}

// TestImportScorecards_BulkPutFailureRecordsError verifies that when the
// bulk PUT for scorecard updates fails, the error is recorded and the
// scorecards are not counted as updated.
func TestImportScorecards_BulkPutFailureRecordsError(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/blueprints/service/scorecards":
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Conflict"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"scorecards": []interface{}{map[string]interface{}{"identifier": "readiness", "title": "Old"}},
			})
			return
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service/scorecards":
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "validation failed"})
			return
		default:
			http.NotFound(w, r)
		}
	})

	importer := NewImporter(client)
	result := &Result{}
	pool := NewWorkerPool(1)

	scorecards := []api.Scorecard{
		{"identifier": "readiness", "blueprintIdentifier": "service", "title": "Readiness"},
	}

	importer.importScorecards(context.Background(), scorecards, result, pool)
	pool.Wait()

	if result.ScorecardsUpdated != 0 {
		t.Fatalf("expected ScorecardsUpdated=0 on failure, got %d", result.ScorecardsUpdated)
	}
	if result.ScorecardsCreated != 0 {
		t.Fatalf("expected ScorecardsCreated=0 on failure, got %d", result.ScorecardsCreated)
	}
	errs := importer.errors.GetByResource("scorecard")
	if len(errs) != 1 {
		t.Fatalf("expected 1 scorecard error recorded, got %d", len(errs))
	}
}

func TestBulkUpsertEntities_AllSucceed(t *testing.T) {
	var bulkPaths []string
	var mu sync.Mutex

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		// blueprints endpoint for detectInheritedOwnershipBlueprints
		if r.Method == http.MethodGet && r.URL.Path == "/blueprints" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entities/bulk") {
			mu.Lock()
			bulkPaths = append(bulkPaths, r.URL.Path)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			return
		}
		http.NotFound(w, r)
	})

	importer := NewImporter(client)

	// 25 service entities → 2 bulk calls (20 + 5)
	entities := make([]api.Entity, 25)
	for idx := range entities {
		entities[idx] = api.Entity{"identifier": fmt.Sprintf("svc-%d", idx), "blueprint": "service"}
	}

	result := &Result{}
	successful := make(map[string]bool)
	var successMu sync.Mutex
	count := 0
	var progressMu sync.Mutex

	importer.bulkUpsertEntities(context.Background(), entities, false, result, successful, &successMu, "Test", len(entities), &count, &progressMu)

	if len(bulkPaths) != 2 {
		t.Errorf("expected 2 bulk calls, got %d", len(bulkPaths))
	}
	if result.EntitiesCreated != 25 {
		t.Errorf("expected EntitiesCreated=25, got %d", result.EntitiesCreated)
	}
	if result.EntitiesUpdated != 0 {
		t.Errorf("expected EntitiesUpdated=0, got %d", result.EntitiesUpdated)
	}
	if len(successful) != 25 {
		t.Errorf("expected 25 successful entities, got %d", len(successful))
	}
}

func TestBulkUpsertEntities_ConflictsRetried(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/blueprints" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entities/bulk") {
			mu.Lock()
			n := callCount
			callCount++
			mu.Unlock()
			if n == 0 {
				// First call: svc-0 is a conflict
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errors": []map[string]interface{}{
						{"identifier": "svc-0", "statusCode": 409.0, "error": "Conflict", "message": "already exists", "index": 0.0},
					},
				})
			} else {
				// Retry call: all succeed
				json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			}
			return
		}
		http.NotFound(w, r)
	})

	importer := NewImporter(client)
	entities := []api.Entity{
		{"identifier": "svc-0", "blueprint": "service"},
		{"identifier": "svc-1", "blueprint": "service"},
	}

	result := &Result{}
	successful := make(map[string]bool)
	var successMu sync.Mutex
	count := 0
	var progressMu sync.Mutex

	importer.bulkUpsertEntities(context.Background(), entities, false, result, successful, &successMu, "Test", len(entities), &count, &progressMu)

	if callCount != 2 {
		t.Errorf("expected 2 bulk calls (original + retry), got %d", callCount)
	}
	if result.EntitiesCreated != 1 {
		t.Errorf("expected EntitiesCreated=1, got %d", result.EntitiesCreated)
	}
	if result.EntitiesUpdated != 1 {
		t.Errorf("expected EntitiesUpdated=1, got %d", result.EntitiesUpdated)
	}
	if len(successful) != 2 {
		t.Errorf("expected 2 successful entities, got %d", len(successful))
	}
}

func TestImportEntities_UsesBulkForBothPhases(t *testing.T) {
	var bulkCalls []struct{ path, upsert string }
	var singleCreateCalls, singleUpdateCalls int
	var mu sync.Mutex

	// Capture progress events to assert phase names are unchanged
	type progressEvent struct {
		phase          string
		current, total int
	}
	var progressEvents []progressEvent
	var progressMu sync.Mutex

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/blueprints" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entities/bulk") {
			mu.Lock()
			bulkCalls = append(bulkCalls, struct{ path, upsert string }{r.URL.Path, r.URL.Query().Get("upsert")})
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entities") && !strings.Contains(r.URL.Path, "bulk") {
			mu.Lock()
			singleCreateCalls++
			mu.Unlock()
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/entities/") {
			mu.Lock()
			singleUpdateCalls++
			mu.Unlock()
			return
		}
		http.NotFound(w, r)
	})

	importer := NewImporter(client)
	importer.SetProgressCallback(func(phase string, current, total int) {
		progressMu.Lock()
		progressEvents = append(progressEvents, progressEvent{phase, current, total})
		progressMu.Unlock()
	})

	// Entity with a relation → triggers Phase 2
	entities := []api.Entity{
		{
			"identifier": "svc-1",
			"blueprint":  "service",
			"relations":  map[string]interface{}{"env": "prod"},
		},
		{
			"identifier": "svc-2",
			"blueprint":  "service",
		},
	}

	result := &Result{}
	err := importer.ImportEntities(context.Background(), entities, false, result)
	if err != nil {
		t.Fatalf("ImportEntities returned error: %v", err)
	}

	if singleCreateCalls != 0 {
		t.Errorf("must not call single create endpoint, got %d calls", singleCreateCalls)
	}
	if singleUpdateCalls != 0 {
		t.Errorf("must not call single update endpoint, got %d calls", singleUpdateCalls)
	}

	// Phase 1: 1 bulk call (upsert=false), Phase 2: 1 bulk call (upsert=true)
	if len(bulkCalls) != 2 {
		t.Errorf("expected 2 bulk calls, got %d: %v", len(bulkCalls), bulkCalls)
	}
	var phase1, phase2 int
	for _, c := range bulkCalls {
		if c.upsert == "false" {
			phase1++
		} else if c.upsert == "true" {
			phase2++
		}
	}
	if phase1 != 1 {
		t.Errorf("Phase 1 uses upsert=false: expected 1 call, got %d", phase1)
	}
	if phase2 != 1 {
		t.Errorf("Phase 2 uses upsert=true: expected 1 call, got %d", phase2)
	}

	if result.EntitiesCreated != 2 {
		t.Errorf("expected EntitiesCreated=2, got %d", result.EntitiesCreated)
	}

	// Phase names must match what users see in the terminal — do not change these strings
	phaseNames := make(map[string]bool)
	for _, ev := range progressEvents {
		phaseNames[ev.phase] = true
	}
	if !phaseNames["Entities Phase 1"] {
		t.Error("progress must report 'Entities Phase 1'")
	}
	if !phaseNames["Entities Phase 2 (relations)"] {
		t.Error("progress must report 'Entities Phase 2 (relations)'")
	}
}
