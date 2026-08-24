package export

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		wantErr bool
	}{
		{
			name: "valid options",
			options: Options{
				OutputPath: "/tmp/test.json",
				Format:     "json",
			},
			wantErr: false,
		},
		{
			name: "missing output path",
			options: Options{
				Format: "json",
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			options: Options{
				OutputPath: "/tmp/test.json",
				Format:     "xml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCollector(t *testing.T) {
	// This is a simple test to ensure NewCollector doesn't panic
	// We can't test collection without a real API client
	client := api.NewClient(api.ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: "https://api.getport.io/v1", Timeout: 0})
	_ = NewCollector(client)
}

func TestWriteJSON(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.json")

	data := &Data{
		Blueprints: []api.Blueprint{
			{"identifier": "test-bp", "title": "Test Blueprint"},
		},
		Entities: []api.Entity{},
	}

	if err := writeJSON(data, outputPath); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Verify file contains valid JSON
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Errorf("Output file is not valid JSON: %v", err)
	}
	if _, ok := result["_folders"]; !ok {
		t.Error("expected _folders key in JSON export output")
	}
}

func TestWriteTar(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.tar.gz")

	data := &Data{
		Blueprints: []api.Blueprint{
			{"identifier": "test-bp", "title": "Test Blueprint"},
		},
		Entities: []api.Entity{},
	}

	if err := writeTar(data, outputPath); err != nil {
		t.Fatalf("writeTar() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}
}

func TestDataHasPermissionFields(t *testing.T) {
	d := &Data{
		BlueprintPermissions: map[string]api.Permissions{"bp1": {"view": []string{"$team"}}},
		ActionPermissions:    map[string]api.Permissions{"act1": {"execute": []string{"$admin"}}},
	}
	if d.BlueprintPermissions["bp1"] == nil {
		t.Error("expected blueprint permissions")
	}
	if d.ActionPermissions["act1"] == nil {
		t.Error("expected action permissions")
	}

	// Verify JSON serialisation uses PascalCase (consistent with other Data fields)
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, `"BlueprintPermissions"`) {
		t.Errorf("expected PascalCase key BlueprintPermissions in JSON, got: %s", outStr)
	}
	if !strings.Contains(outStr, `"ActionPermissions"`) {
		t.Errorf("expected PascalCase key ActionPermissions in JSON, got: %s", outStr)
	}
}

func TestWriteJSON_IncludesPermissions(t *testing.T) {
	d := &Data{
		Blueprints: []api.Blueprint{{"identifier": "service", "title": "Service"}},
		Folders:    []api.Folder{{"identifier": "catalog", "title": "Catalog"}},
		BlueprintPermissions: map[string]api.Permissions{
			"service": {"entities": map[string]interface{}{"view": []string{"$team"}}},
		},
		ActionPermissions: map[string]api.Permissions{
			"deploy": {"execute": map[string]interface{}{"users": []string{}}},
		},
		PagePermissions: map[string]api.Permissions{
			"dashboard": {"roles": map[string]interface{}{"view": []string{"Admin"}}},
		},
	}
	var buf bytes.Buffer
	if err := d.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"blueprint_permissions"`) {
		t.Errorf("expected blueprint_permissions key in JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"action_permissions"`) {
		t.Errorf("expected action_permissions key in JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"page_permissions"`) {
		t.Errorf("expected page_permissions key in JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"_folders"`) {
		t.Errorf("expected _folders key in JSON output, got: %s", output)
	}
	if !strings.Contains(output, "service") {
		t.Errorf("expected 'service' identifier in permissions JSON, got: %s", output)
	}
}

func TestExecute_StreamsEntitiesFromGetEndpoint(t *testing.T) {
	countCalls := 0
	entitiesCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			countCalls++
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 2})
		case "/blueprints/service/entities":
			entitiesCalls++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"entities": []map[string]interface{}{
					{"identifier": "svc-1", "blueprint": "service"},
					{"identifier": "svc-2", "blueprint": "service"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := &Module{client: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})}
	outputPath := filepath.Join(t.TempDir(), "export.json")
	result, err := module.Execute(context.Background(), Options{
		OutputPath:       outputPath,
		Format:           "json",
		IncludeResources: []string{"entities"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("export failed: %v", result.Error)
	}
	if result.EntitiesCount != 2 {
		t.Fatalf("expected 2 streamed entities, got %d", result.EntitiesCount)
	}
	if countCalls != 1 {
		t.Fatalf("expected 1 entities-count call, got %d", countCalls)
	}
	if entitiesCalls != 1 {
		t.Fatalf("expected 1 entities GET call, got %d", entitiesCalls)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed struct {
		Entities []map[string]interface{} `json:"entities"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output JSON was invalid: %v", err)
	}
	if len(parsed.Entities) != 2 {
		t.Fatalf("expected 2 entities in output, got %d", len(parsed.Entities))
	}
}

func TestExecute_StreamsLargeBlueprintEntitiesFromSearch(t *testing.T) {
	countCalls := 0
	getCalls := 0
	searchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			countCalls++
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 10001})
		case "/blueprints/service/entities":
			getCalls++
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
			if _, ok := body["query"].(map[string]interface{}); !ok {
				t.Fatalf("expected wrapped query body, got %#v", body)
			}
			switch searchCalls {
			case 1:
				if _, ok := body["from"]; ok {
					t.Fatalf("first search request should not include from: %#v", body)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":       true,
					"next":     "cursor-1",
					"entities": []map[string]interface{}{{"identifier": "svc-1", "blueprint": "service"}},
				})
			case 2:
				if body["from"] != "cursor-1" {
					t.Fatalf("expected cursor-1, got %#v", body["from"])
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":       true,
					"entities": []map[string]interface{}{{"identifier": "svc-2", "blueprint": "service"}},
				})
			default:
				http.Error(w, "unexpected extra search call", http.StatusInternalServerError)
			}
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := &Module{client: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})}
	outputPath := filepath.Join(t.TempDir(), "export.json")
	result, err := module.Execute(context.Background(), Options{
		OutputPath:       outputPath,
		Format:           "json",
		IncludeResources: []string{"entities"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("export failed: %v", result.Error)
	}
	if result.EntitiesCount != 2 {
		t.Fatalf("expected 2 streamed entities, got %d", result.EntitiesCount)
	}
	if countCalls != 1 {
		t.Fatalf("expected 1 entities-count call, got %d", countCalls)
	}
	if getCalls != 0 {
		t.Fatalf("expected no entities GET calls, got %d", getCalls)
	}
	if searchCalls != 2 {
		t.Fatalf("expected 2 search calls, got %d", searchCalls)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed struct {
		Entities []map[string]interface{} `json:"entities"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output JSON was invalid: %v", err)
	}
	if len(parsed.Entities) != 2 {
		t.Fatalf("expected 2 entities in output, got %d", len(parsed.Entities))
	}
}

func TestExecute_ActionsOnly_ScopesBlueprintsToReferenced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "domain"},
				},
			})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "deploy"}},
			})
		case "/blueprints/domain/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "actions": []interface{}{}})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{{
					"identifier": "deploy",
					"trigger": map[string]interface{}{
						"type":                "self-service",
						"blueprintIdentifier": "service",
					},
				}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := &Module{client: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})}
	outputPath := filepath.Join(t.TempDir(), "export.json")
	result, err := module.Execute(context.Background(), Options{
		OutputPath:          outputPath,
		Format:              "json",
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("export failed: %v", result.Error)
	}
	if result.BlueprintsCount != 1 {
		t.Fatalf("expected 1 blueprint in result count, got %d", result.BlueprintsCount)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed struct {
		Blueprints []map[string]interface{} `json:"blueprints"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output JSON was invalid: %v", err)
	}
	if len(parsed.Blueprints) != 1 || parsed.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' blueprint in output, got %v", parsed.Blueprints)
	}
}

func TestExecute_EntitiesOnly_ScopesBlueprintsToReferenced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "domain"},
				},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 1})
		case "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"entities": []map[string]interface{}{
					{"identifier": "ent1", "blueprint": "service"},
				},
			})
		case "/blueprints/domain/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 1})
		case "/blueprints/domain/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"entities": []map[string]interface{}{
					{"identifier": "ent2", "blueprint": "domain"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := &Module{client: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})}
	outputPath := filepath.Join(t.TempDir(), "export.json")
	// entity ID filter selects only "ent1", which belongs to "service".
	result, err := module.Execute(context.Background(), Options{
		OutputPath:          outputPath,
		Format:              "json",
		IncludeResources:    []string{"blueprints", "entities"},
		Entities:            []string{"ent1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("export failed: %v", result.Error)
	}
	if result.BlueprintsCount != 1 {
		t.Fatalf("expected 1 blueprint, got %d", result.BlueprintsCount)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed struct {
		Blueprints []map[string]interface{} `json:"blueprints"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("output JSON was invalid: %v", err)
	}
	if len(parsed.Blueprints) != 1 || parsed.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' blueprint in output, got %v", parsed.Blueprints)
	}
}

func TestExecute_BlueprintsExplicit_KeepsFullSetAlongsideActions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "domain"},
				},
			})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "deploy"}},
			})
		case "/blueprints/domain/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "actions": []interface{}{}})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{{
					"identifier": "deploy",
					"trigger": map[string]interface{}{
						"type":                "self-service",
						"blueprintIdentifier": "service",
					},
				}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	module := &Module{client: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})}
	outputPath := filepath.Join(t.TempDir(), "export.json")
	// Simulates `--blueprints --actions deploy`: AutoScopeBlueprints is false
	// because the caller explicitly asked for blueprints (computed at the
	// command layer in Task 3).
	result, err := module.Execute(context.Background(), Options{
		OutputPath:          outputPath,
		Format:              "json",
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: false,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("export failed: %v", result.Error)
	}
	if result.BlueprintsCount != 2 {
		t.Fatalf("expected both blueprints kept when AutoScopeBlueprints is false, got %d", result.BlueprintsCount)
	}
}

// Note: Integration tests for actual API calls would require:
// 1. A test Port organization
// 2. Valid credentials
// 3. Mock HTTP server or test fixtures
