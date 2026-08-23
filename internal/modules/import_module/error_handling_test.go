package import_module

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/port-labs/port-cli/internal/api"
)

func TestBlueprintUpdaterForbiddenFormatChangeIgnoreProperty(t *testing.T) {
	var updateCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "service",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{"type": "string"},
					},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service":
			updateCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			if updateCalls == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "forbidden_format_change",
					"message": "Cannot change format",
					"details": map[string]interface{}{"property": "url"},
				})
				return
			}
			props := body["properties"].(map[string]interface{})
			urlProp := props["url"].(map[string]interface{})
			if _, hasFormat := urlProp["format"]; hasFormat {
				t.Fatalf("expected retry to preserve existing url property without format, got %#v", urlProp)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	var warnings []string
	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	updater := NewBlueprintUpdater(client, ErrorHandlingOptions{
		Policies: map[string]ErrorAction{"forbidden_format_change": ErrorActionIgnoreProperty},
		AddWarning: func(message string) {
			warnings = append(warnings, message)
		},
	})
	err := updater.Update(context.Background(), "service", api.Blueprint{
		"identifier": "service",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{"type": "string", "format": "url"},
		},
	}, BlueprintUpdatePUT)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updateCalls != 2 {
		t.Fatalf("expected 2 update calls, got %d", updateCalls)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "ignored property url") {
		t.Fatalf("expected ignore warning, got %#v", warnings)
	}
}

func TestBlueprintUpdaterForbiddenFormatChangeRecreateProperty(t *testing.T) {
	var updateCalls, migrationCalls int
	var tempProperty string
	var completedCopyToTemp, completedCopyBack bool
	migrationStatusCalls := make(map[string]int)
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == http.MethodGet && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": "service",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{"type": "string"},
					},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/blueprints/service":
			updateCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			props, _ := body["properties"].(map[string]interface{})
			switch updateCalls {
			case 1:
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "forbidden_format_change",
					"message": "Cannot change format",
					"details": map[string]interface{}{"property": "url"},
				})
				return
			case 2:
				for key := range props {
					if strings.HasPrefix(key, "port_cli_tmp_url_") {
						tempProperty = key
					}
				}
				if tempProperty == "" {
					t.Fatalf("expected temporary property in %#v", props)
				}
			case 3:
				if !completedCopyToTemp {
					t.Fatalf("original property removed before migration into temporary property completed")
				}
				if _, ok := props["url"]; ok {
					t.Fatalf("expected original property removed before recreate, got %#v", props)
				}
			case 4:
				urlProp := props["url"].(map[string]interface{})
				if urlProp["format"] != "url" {
					t.Fatalf("expected desired url format, got %#v", urlProp)
				}
			case 5:
				if !completedCopyBack {
					t.Fatalf("temporary property removed before migration back into recreated property completed")
				}
				if _, ok := props[tempProperty]; ok {
					t.Fatalf("expected temporary property cleaned up, got %#v", props)
				}
			case 6:
				urlProp := props["url"].(map[string]interface{})
				if urlProp["format"] != "url" {
					t.Fatalf("expected final retry with desired url format, got %#v", urlProp)
				}
			default:
				t.Fatalf("unexpected update call %d", updateCalls)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		case r.Method == http.MethodPost && r.URL.Path == "/migrations":
			migrationCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode migration body: %v", err)
			}
			if body["sourceBlueprint"] != "service" {
				t.Fatalf("sourceBlueprint = %#v", body["sourceBlueprint"])
			}
			migrationID := "copy-to-temp"
			if migrationCalls == 2 {
				migrationID = "copy-back"
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"migration": map[string]interface{}{
					"identifier": migrationID,
					"status":     "RUNNING",
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/migrations/"):
			migrationID := strings.TrimPrefix(r.URL.Path, "/migrations/")
			migrationStatusCalls[migrationID]++
			status := "RUNNING"
			if migrationStatusCalls[migrationID] > 1 {
				status = "COMPLETED"
				if migrationID == "copy-to-temp" {
					completedCopyToTemp = true
				}
				if migrationID == "copy-back" {
					completedCopyBack = true
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"migration": map[string]interface{}{
					"identifier": migrationID,
					"status":     status,
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	var warnings []string
	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	updater := NewBlueprintUpdater(client, ErrorHandlingOptions{
		Policies: map[string]ErrorAction{"forbidden_format_change": ErrorActionRecreateProperty},
		AddWarning: func(message string) {
			warnings = append(warnings, message)
		},
		MigrationPollInterval: time.Millisecond,
	})
	err := updater.Update(context.Background(), "service", api.Blueprint{
		"identifier": "service",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{"type": "string", "format": "url"},
		},
	}, BlueprintUpdatePUT)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updateCalls != 6 {
		t.Fatalf("expected 6 update calls, got %d", updateCalls)
	}
	if migrationCalls != 2 {
		t.Fatalf("expected 2 migration calls, got %d", migrationCalls)
	}
	if migrationStatusCalls["copy-to-temp"] != 2 || migrationStatusCalls["copy-back"] != 2 {
		t.Fatalf("expected each migration to be polled until success, got %#v", migrationStatusCalls)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "recreated property url") {
		t.Fatalf("expected recreate warning, got %#v", warnings)
	}
}

func TestBlueprintUpdaterForbiddenFormatChangeNoPolicyFailsWithGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints/service":
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      false,
				"error":   "forbidden_format_change",
				"message": "Cannot change format",
				"details": map[string]interface{}{"property": "url"},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	updater := NewBlueprintUpdater(client, ErrorHandlingOptions{})
	err := updater.Update(context.Background(), "service", api.Blueprint{"identifier": "service"}, BlueprintUpdatePUT)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--on-error forbidden_format_change=ignore-property") {
		t.Fatalf("expected --on-error guidance, got %v", err)
	}
}

func TestMigrationStatusEnumHandling(t *testing.T) {
	if !migrationStatusSucceeded("COMPLETED") {
		t.Fatal("COMPLETED should be successful")
	}
	for _, status := range []string{"RUNNING", "PENDING", "INITIALIZING"} {
		if migrationStatusSucceeded(status) || migrationStatusFailed(status) {
			t.Fatalf("%s should keep polling", status)
		}
	}
	for _, status := range []string{"FAILURE", "CANCELLED", "PENDING_CANCELLATION"} {
		if !migrationStatusFailed(status) {
			t.Fatalf("%s should be failed", status)
		}
	}
	if migrationStatusSucceeded("Migrated successfully") || migrationStatusFailed("Migrated with errors") {
		t.Fatal("non-enum statuses should not be classified")
	}
}
