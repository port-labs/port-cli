package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
	"github.com/port-labs/port-cli/internal/modules/import_module"
)

func TestMarkMigrationStoppedPreservesPartialCounts(t *testing.T) {
	result := &Result{
		BlueprintsCreated: 2,
		EntitiesCreated:   3,
		ScorecardsUpdated: 1,
	}

	markMigrationStopped(result, nil, errors.New("entities service: gone"))

	if result.Success {
		t.Fatal("expected stopped migration to be unsuccessful")
	}
	if result.BlueprintsCreated != 2 || result.EntitiesCreated != 3 || result.ScorecardsUpdated != 1 {
		t.Fatalf("expected partial counts to be preserved, got %#v", result)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "gone") {
		t.Fatalf("expected stop error to be recorded, got %v", result.Errors)
	}
	if !strings.Contains(result.Message, "Migration stopped with 1 error(s)") {
		t.Fatalf("expected stopped migration message, got %q", result.Message)
	}
}

func TestExportFromSource_SkipEntities_SkipsTeamsAndUsers(t *testing.T) {
	teamsHit := false
	usersHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/teams":
			teamsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "teams": []interface{}{}})
		case "/users":
			usersHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "users": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	opts := Options{SkipEntities: true}
	_, _, _, err := m.exportFromSource(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if teamsHit {
		t.Error("teams endpoint should not be called when SkipEntities=true")
	}
	if usersHit {
		t.Error("users endpoint should not be called when SkipEntities=true")
	}
}

func TestExportFromSource_SkipSystemBlueprints_ExcludesSchemaAndEntities(t *testing.T) {
	entitiesHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "_user"},
					{"identifier": "service"},
				},
			})
		case "/blueprints/_user/entities":
			entitiesHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entities": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	opts := Options{SkipSystemBlueprints: true}
	data, entityBlueprints, _, err := m.exportFromSource(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, bp := range data.Blueprints {
		id, _ := bp["identifier"].(string)
		if id == "_user" {
			t.Error("_user blueprint schema should be excluded from migrate data")
		}
	}
	if entitiesHit {
		t.Error("entities endpoint for _user should not be called in migrate when SkipSystemBlueprints=true")
	}
	for _, bp := range entityBlueprints {
		if id, _ := bp["identifier"].(string); id == "_user" {
			t.Error("_user should not be returned as an entity streaming blueprint")
		}
	}
}

func TestExportFromSource_ReturnsEntityBlueprintsWithoutFetchingEntities(t *testing.T) {
	getEntitiesHit := false
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
			if _, ok := body["query"].(map[string]interface{}); !ok {
				t.Fatalf("expected wrapped query body, got %#v", body)
			}
			switch searchCalls {
			case 1:
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

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, entityBlueprints, _, err := m.exportFromSource(context.Background(), Options{IncludeResources: []string{"entities"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getEntitiesHit {
		t.Error("GET entities endpoint should not be called while exporting migrate metadata")
	}
	if searchCalls != 0 {
		t.Fatalf("expected no search calls while exporting migrate metadata, got %d", searchCalls)
	}
	if len(data.Entities) != 0 {
		t.Fatalf("expected no entities in metadata export, got %d", len(data.Entities))
	}
	if len(entityBlueprints) != 1 || entityBlueprints[0]["identifier"] != "service" {
		t.Fatalf("expected service as the streaming entity blueprint, got %v", entityBlueprints)
	}
}

func TestExportFromSource_ActionsOnly_ScopesBlueprintsToReferenced(t *testing.T) {
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

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 1 || data.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' blueprint, got %v", data.Blueprints)
	}
}

func TestExportFromSource_EntitiesOnly_PreScanScopesBlueprintsToReferenced(t *testing.T) {
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

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	// Entity ID filter selects only "ent1" (belongs to "service") — the
	// pre-scan must check entity existence per blueprint BEFORE data.Blueprints
	// is finalized, since entities themselves are migrated later by
	// migrateEntities (after blueprint schema diff/import already ran).
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		IncludeResources:    []string{"blueprints", "entities"},
		Entities:            []string{"ent1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 1 || data.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' blueprint, got %v", data.Blueprints)
	}
}

func TestExportFromSource_EntitiesOnly_ScopesEntityBlueprintsConsistently(t *testing.T) {
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

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	// Entity ID filter selects only "ent1" (belongs to "service"). Before the
	// fix, data.Blueprints narrowed correctly to ["service"] but
	// entityBlueprints stayed unscoped at ["service", "domain"] — migrateEntities
	// would then try (and, against a real org, fail) to sync entities for
	// "domain" even though its schema was excluded from this migration.
	data, entityBlueprints, _, err := m.exportFromSource(context.Background(), Options{
		IncludeResources:    []string{"blueprints", "entities"},
		Entities:            []string{"ent1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 1 || data.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected data.Blueprints to be only 'service', got %v", data.Blueprints)
	}
	if len(entityBlueprints) != 1 || entityBlueprints[0]["identifier"] != "service" {
		t.Fatalf("expected entityBlueprints to be scoped identically to data.Blueprints (only 'service'), got %v", entityBlueprints)
	}
}

func TestExportFromSource_AutoScopeBlueprints_DoesNotPullInUnrelatedRelationTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{
						"identifier": "service",
						"relations": map[string]interface{}{
							"domain_rel": map[string]interface{}{"target": "domain"},
						},
					},
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

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	// "service" has action "deploy" (directly referenced) and a relation to
	// "domain", which has no actions of its own. data.Blueprints must stay
	// scoped to just "service" — relation-target existence in the target is
	// importToTarget's job (see the TestImportToTarget_RelationTarget* tests),
	// not exportFromSource's. Pulling "domain" in here would mean touching a
	// blueprint the caller never asked about, purely because of an unrelated
	// relation.
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 1 || data.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' in data.Blueprints, got %v", data.Blueprints)
	}
}

func TestExecute_AutoScopeBlueprints_ReusesEntityPreScanInsteadOfRefetching(t *testing.T) {
	var entitiesFetchCount int
	var mu sync.Mutex

	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 2})
		case "/blueprints/service/entities":
			mu.Lock()
			entitiesFetchCount++
			mu.Unlock()
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
	defer sourceServer.Close()

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entities": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer targetServer.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: sourceServer.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: targetServer.URL}),
	}
	// AutoScopeBlueprints' relevance pre-scan (blueprintHasMatchingEntity) has
	// to page through "service"'s entities to find "svc-1" — the fix caches
	// what it finds so migrateEntities doesn't hit /blueprints/service/entities
	// a second time for the same blueprint.
	result, err := m.Execute(context.Background(), Options{
		DryRun:              true,
		IncludeResources:    []string{"blueprints", "entities"},
		Entities:            []string{"svc-1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.EntitiesCreated != 1 {
		t.Fatalf("expected 1 filtered entity, got %d", result.EntitiesCreated)
	}
	mu.Lock()
	defer mu.Unlock()
	if entitiesFetchCount != 1 {
		t.Fatalf("expected /blueprints/service/entities to be fetched exactly once (pre-scan result reused), got %d fetches", entitiesFetchCount)
	}
}

func TestExportFromSource_BoundsConcurrentBlueprintFetches(t *testing.T) {
	const numBlueprints = 3 * maxConcurrentBlueprints

	blueprints := make([]map[string]interface{}, numBlueprints)
	for i := 0; i < numBlueprints; i++ {
		blueprints[i] = map[string]interface{}{"identifier": fmt.Sprintf("bp-%d", i)}
	}

	var current, peak int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": blueprints})
		case strings.HasSuffix(r.URL.Path, "/scorecards"):
			n := atomic.AddInt32(&current, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			// Hold the "in-flight" request open briefly so concurrent callers
			// actually pile up — without this, requests could complete faster
			// than goroutines are scheduled and the semaphore would never be
			// observed under contention.
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&current, -1)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecards": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	// Scorecards-only keeps this to exactly one semaphore-gated goroutine per
	// blueprint (actions would also spawn ungated per-action permission
	// fetches, muddying the exact peak we're asserting on).
	_, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "scorecards"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotPeak := atomic.LoadInt32(&peak)
	if gotPeak > maxConcurrentBlueprints {
		t.Fatalf("expected peak concurrent scorecard fetches to stay <= %d, got %d", maxConcurrentBlueprints, gotPeak)
	}
	if gotPeak < 2 {
		t.Fatalf("test didn't exercise meaningful concurrency (peak=%d) — this assertion wouldn't catch an unbounded regression", gotPeak)
	}
}

func TestExportFromSource_BlueprintsExplicit_KeepsFullSetAlongsideActions(t *testing.T) {
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
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "actions": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	// Simulates `--blueprints --actions deploy`: AutoScopeBlueprints is false
	// because the caller explicitly asked for blueprints.
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 2 {
		t.Fatalf("expected both blueprints kept when --blueprints is explicit, got %d: %v", len(data.Blueprints), data.Blueprints)
	}
}

func TestExecute_StreamsEntitiesBlueprintByBlueprint(t *testing.T) {
	sourceSearchCalls := 0
	sourceGetEntitiesHit := false
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 10001})
		case "/blueprints/service/entities":
			sourceGetEntitiesHit = true
			http.Error(w, "unexpected source GET entities call", http.StatusInternalServerError)
		case "/blueprints/service/entities/search":
			sourceSearchCalls++
			switch sourceSearchCalls {
			case 1:
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":   true,
					"next": "cursor-1",
					"entities": []map[string]interface{}{
						{
							"identifier": "svc-1",
							"blueprint":  "service",
							"relations":  map[string]interface{}{"owner": "team-a"},
						},
					},
				})
			case 2:
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode search body: %v", err)
				}
				if body["from"] != "cursor-1" {
					t.Fatalf("expected cursor-1, got %#v", body["from"])
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":       true,
					"entities": []map[string]interface{}{{"identifier": "svc-2", "blueprint": "service"}},
				})
			default:
				http.Error(w, "unexpected extra source search call", http.StatusInternalServerError)
			}
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer sourceServer.Close()

	type bulkRequest struct {
		upsert   string
		entities []map[string]interface{}
	}
	var mu sync.Mutex
	var bulkRequests []bulkRequest
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entities": []interface{}{}})
		case "/blueprints/service/entities/bulk":
			var payload struct {
				Entities []map[string]interface{} `json:"entities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode bulk body: %v", err)
			}
			mu.Lock()
			bulkRequests = append(bulkRequests, bulkRequest{
				upsert:   r.URL.Query().Get("upsert"),
				entities: payload.Entities,
			})
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer targetServer.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: sourceServer.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: targetServer.URL}),
	}
	result, err := m.Execute(context.Background(), Options{IncludeResources: []string{"entities"}})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if sourceGetEntitiesHit {
		t.Fatal("source GET entities endpoint should not be used for large streaming blueprint")
	}
	if sourceSearchCalls != 2 {
		t.Fatalf("expected 2 source search calls, got %d", sourceSearchCalls)
	}
	if result.EntitiesCreated != 2 {
		t.Fatalf("expected 2 created entities, got %d", result.EntitiesCreated)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(bulkRequests) != 2 {
		t.Fatalf("expected phase 1 and phase 2 bulk calls, got %d", len(bulkRequests))
	}
	if bulkRequests[0].upsert != "false" {
		t.Fatalf("phase 1 should use upsert=false, got %q", bulkRequests[0].upsert)
	}
	if _, hasRelations := bulkRequests[0].entities[0]["relations"]; hasRelations {
		t.Fatalf("phase 1 should strip relations, got %v", bulkRequests[0].entities[0])
	}
	if bulkRequests[1].upsert != "true" {
		t.Fatalf("phase 2 should use upsert=true, got %q", bulkRequests[1].upsert)
	}
	if len(bulkRequests[1].entities) != 1 || bulkRequests[1].entities[0]["identifier"] != "svc-1" {
		t.Fatalf("phase 2 should only include changed relation entities, got %v", bulkRequests[1].entities)
	}
}

func TestExecute_StreamingEntityReadErrorStopsMigration(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "gone"},
					{"identifier": "later"},
				},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":       true,
				"entities": []map[string]interface{}{{"identifier": "svc-1", "blueprint": "service"}},
			})
		case "/blueprints/gone/entities-count":
			w.WriteHeader(http.StatusGone)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "gone"})
		case "/blueprints/later/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case "/blueprints/later/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":       true,
				"entities": []map[string]interface{}{{"identifier": "later-1", "blueprint": "later"}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer sourceServer.Close()

	var mu sync.Mutex
	var bulkBlueprints []string
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "service"},
					{"identifier": "gone"},
					{"identifier": "later"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/entities-count"):
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case strings.HasSuffix(r.URL.Path, "/entities"):
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entities": []interface{}{}})
		case strings.HasSuffix(r.URL.Path, "/entities/bulk"):
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) >= 3 {
				mu.Lock()
				bulkBlueprints = append(bulkBlueprints, parts[2])
				mu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer targetServer.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: sourceServer.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: targetServer.URL}),
	}
	result, err := m.Execute(context.Background(), Options{IncludeResources: []string{"entities"}})
	if err == nil {
		t.Fatal("expected streaming entity read error to stop migration")
	}
	if result == nil {
		t.Fatal("expected partial result on fatal streaming entity read error")
	}
	if result.Success {
		t.Fatal("expected partial result to be marked unsuccessful")
	}
	if result.EntitiesCreated != 1 {
		t.Fatalf("expected partial result to keep entities created before failure, got %d", result.EntitiesCreated)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected partial result to include the entity read error")
	}
	if !strings.Contains(result.Message, "Migration stopped with") {
		t.Fatalf("expected stopped migration message, got %q", result.Message)
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Fatalf("expected error to mention unreadable blueprint, got %v", err)
	}
	if !strings.Contains(strings.Join(result.Errors, "\n"), "gone") {
		t.Fatalf("expected result errors to mention unreadable blueprint, got %v", result.Errors)
	}
	if len(bulkBlueprints) != 1 || bulkBlueprints[0] != "service" {
		t.Fatalf("expected migration to stop before later blueprint, got bulk calls %v", bulkBlueprints)
	}
}

func TestExecute_StreamingEntitiesDryRunAppliesEntityFilter(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case "/blueprints/service/entities":
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
	defer sourceServer.Close()

	bulkCalled := false
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case r.URL.Path == "/blueprints/service/entities-count":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 0})
		case r.URL.Path == "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entities": []interface{}{}})
		case strings.HasSuffix(r.URL.Path, "/entities/bulk"):
			bulkCalled = true
			http.Error(w, "bulk should not be called in dry run", http.StatusInternalServerError)
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer targetServer.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: sourceServer.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: targetServer.URL}),
	}
	result, err := m.Execute(context.Background(), Options{
		DryRun:           true,
		IncludeResources: []string{"entities"},
		Entities:         []string{"svc-1"},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if bulkCalled {
		t.Fatal("bulk endpoint should not be called during dry run")
	}
	if result.EntitiesCreated != 1 {
		t.Fatalf("expected only filtered entity to be counted as created, got %d", result.EntitiesCreated)
	}
	if result.EntitiesUpdated != 0 {
		t.Fatalf("expected no updated entities, got %d", result.EntitiesUpdated)
	}
}

func TestExecute_DryRunCarriesCustomSystemBlueprintPatch(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
							"cost_center": map[string]interface{}{"type": "string"},
						},
					},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer sourceServer.Close()

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer targetServer.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: sourceServer.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: targetServer.URL}),
	}
	result, err := m.Execute(context.Background(), Options{
		DryRun:               true,
		SkipSystemBlueprints: true,
		IncludeResources:     []string{"blueprints"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BlueprintsUpdated != 1 {
		t.Fatalf("expected 1 system blueprint property update, got %d", result.BlueprintsUpdated)
	}
}

func TestMigrateOptionsHasExcludeFields(t *testing.T) {
	opts := Options{
		ExcludeBlueprints:      []string{"service"},
		ExcludeBlueprintSchema: []string{"region"},
	}
	if len(opts.ExcludeBlueprints) != 1 {
		t.Error("ExcludeBlueprints not set")
	}
	if len(opts.ExcludeBlueprintSchema) != 1 {
		t.Error("ExcludeBlueprintSchema not set")
	}
}

func TestExportFromSource_IntegrationFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/integration":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"integrations": []map[string]interface{}{
					{"installationId": "int1", "name": "GitHub"},
					{"installationId": "int2", "name": "GitLab"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"integrations"},
		Integrations:     []string{"int1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Integrations) != 1 || data.Integrations[0]["installationId"] != "int1" {
		t.Errorf("expected 1 integration (int1), got %v", data.Integrations)
	}
	if len(data.Blueprints) != 0 {
		t.Errorf("expected no blueprints when only integrations included, got %d", len(data.Blueprints))
	}
}

func TestExportFromSource_ActionPermissionsNotCollectedWhenExcluded(t *testing.T) {
	actionPermsHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "svc"}},
			})
		case "/blueprints/svc/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "act1"}},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{{
					"identifier": "act1",
					"trigger": map[string]interface{}{
						"type":                "self-service",
						"blueprintIdentifier": "svc",
					},
				}},
			})
		case "/actions/act1/permissions":
			actionPermsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	opts := Options{IncludeResources: []string{"actions"}}
	_, _, _, err := m.exportFromSource(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actionPermsHit {
		t.Error("action permissions endpoint should not be called when action-permissions not in IncludeResources")
	}
}

func TestExportFromSource_ActionPermissionsFetchFailureRecordsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "svc"}},
			})
		case "/blueprints/svc/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "act1"}},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{{
					"identifier": "act1",
					"trigger": map[string]interface{}{
						"type":                "self-service",
						"blueprintIdentifier": "svc",
					},
				}},
			})
		case "/actions/act1/permissions":
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Warnings) == 0 {
		t.Error("expected a warning when action permissions fetch fails")
	}
}

func TestExportFromSource_PagePermissionsFetchFailureRecordsWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/pages":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    true,
				"pages": []map[string]interface{}{{"identifier": "home", "title": "Home"}},
			})
		case "/pages/home/permissions":
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{SkipEntities: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Warnings) == 0 {
		t.Error("expected a warning when page permissions fetch fails")
	}
}

func TestExportFromSource_PagePermissionsNotCollectedWhenExcluded(t *testing.T) {
	pagePermsHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/pages":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    true,
				"pages": []map[string]interface{}{{"identifier": "home"}},
			})
		case "/pages/home/permissions":
			pagePermsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	_, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"pages"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pagePermsHit {
		t.Error("page permissions endpoint should not be called when page-permissions not in IncludeResources")
	}
}

func TestImportToTarget_PagePermissions_RetriesOnOrphanedFields(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/pages/home/permissions":
			if r.Method != "PATCH" {
				json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
				return
			}
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      false,
					"error":   "invalid_permissions",
					"message": "You cannot update permissions on unknown fields",
					"details": map[string]interface{}{
						"invalidProperties": []string{},
						"invalidRelations":  []string{"staleRel"},
					},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	data := &export.Data{
		Blueprints:           []api.Blueprint{},
		Entities:             []api.Entity{},
		Scorecards:           []api.Scorecard{},
		Actions:              []api.Action{},
		Teams:                []api.Team{},
		Users:                []api.User{},
		Folders:              []api.Folder{},
		Pages:                []api.Page{},
		Integrations:         []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		PagePermissions: []import_module.PermissionsChange{
			{
				Identifier: "home",
				Permissions: api.Permissions{
					"read":     map[string]interface{}{"roles": []string{"Admin"}},
					"staleRel": map[string]interface{}{"roles": []string{"Admin"}},
				},
			},
		},
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PagePermissionsUpdated != 1 {
		t.Errorf("expected 1 page permission updated after retry, got %d", result.PagePermissionsUpdated)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (original + retry), got %d", callCount)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning about stripped fields, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors after successful retry, got: %v", result.Errors)
	}
}

func TestImportToTarget_AggPropsAppliedInTopologicalOrder(t *testing.T) {
	var mu sync.Mutex
	var updateOrder []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": id},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if _, hasAgg := body["aggregationProperties"]; hasAgg {
				mu.Lock()
				updateOrder = append(updateOrder, id)
				mu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	// bizApp aggregates component's agg prop "bugs", so component must run first.
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "component",
				"aggregationProperties": map[string]interface{}{
					"bugs": map[string]interface{}{
						"target": "snykTarget",
						"calculationSpec": map[string]interface{}{
							"calculationBy": "property",
							"func":          "sum",
							"property":      "numberOfBugs",
						},
					},
				},
			},
			{
				"identifier": "bizApp",
				"aggregationProperties": map[string]interface{}{
					"totalBugs": map[string]interface{}{
						"target": "component",
						"calculationSpec": map[string]interface{}{
							"calculationBy": "property",
							"func":          "sum",
							"property":      "bugs",
						},
					},
				},
			},
		},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}

	componentIdx, bizAppIdx := -1, -1
	for i, id := range updateOrder {
		if id == "component" {
			componentIdx = i
		}
		if id == "bizApp" {
			bizAppIdx = i
		}
	}
	if componentIdx == -1 || bizAppIdx == -1 {
		t.Fatalf("expected both component and bizApp agg prop updates, got order: %v", updateOrder)
	}
	if componentIdx >= bizAppIdx {
		t.Errorf("component agg props must be applied before bizApp, got order: %v", updateOrder)
	}
}

func TestImportToTarget_RelationTargetAlreadyInTarget_NotFlaggedMissing(t *testing.T) {
	var mu sync.Mutex
	var appliedRelations map[string]interface{}
	domainFetched := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "GET" && r.URL.Path == "/blueprints":
			// The target already has "domain" — it just isn't part of this
			// migration's scoped diff at all (data.Blueprints only has "service").
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "domain"}},
			})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{"identifier": "service"}})
		case r.Method == "GET" && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": "service"},
			})
		case r.Method == "GET" && r.URL.Path == "/blueprints/domain":
			domainFetched = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{"identifier": "domain"}})
		case r.Method == "PUT" && r.URL.Path == "/blueprints/service":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if rels, ok := body["relations"]; ok {
				mu.Lock()
				appliedRelations = rels.(map[string]interface{})
				mu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	// data.Blueprints is scoped to just "service" (simulating AutoScopeBlueprints
	// after the fix) — "domain" is never part of this run's diff, but it does
	// already exist in the target.
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "service",
				"relations": map[string]interface{}{
					"domain_rel": map[string]interface{}{"target": "domain"},
				},
			},
		},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range result.Errors {
		if strings.Contains(e, "missing target blueprints") {
			t.Fatalf("did not expect a missing-target-blueprints error, got: %v", result.Errors)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if appliedRelations == nil {
		t.Fatal("expected service's relations to be applied via PUT, but relations field was never sent")
	}
	if _, ok := appliedRelations["domain_rel"]; !ok {
		t.Fatalf("expected domain_rel relation to be applied, got %v", appliedRelations)
	}
	if domainFetched {
		t.Error("domain was never part of this migration's scope and should not have been fetched/touched")
	}
}

func TestImportToTarget_RelationTargetGenuinelyMissing_ReportsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "GET" && r.URL.Path == "/blueprints":
			// The target does NOT have "domain" at all — a genuine gap.
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{"identifier": "service"}})
		case r.Method == "GET" && r.URL.Path == "/blueprints/service":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": "service"},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	data := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "service",
				"relations": map[string]interface{}{
					"domain_rel": map[string]interface{}{"target": "domain"},
				},
			},
		},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "missing target blueprints") && strings.Contains(e, "domain") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a missing-target-blueprints error mentioning 'domain', got: %v", result.Errors)
	}
}

func TestImportToTarget_PageCreateConflictFallsBackToUpdate(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var patchBody api.Page

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == http.MethodPost && r.URL.Path == "/pages":
			mu.Lock()
			calls = append(calls, "create")
			mu.Unlock()
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Conflict", "message": "already exists"})
		case r.Method == http.MethodPatch && r.URL.Path == "/pages/home":
			mu.Lock()
			calls = append(calls, "update")
			json.NewDecoder(r.Body).Decode(&patchBody)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "home"}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data := &export.Data{
		Pages: []api.Page{
			{"identifier": "home", "title": "Home", "type": "dashboard", "widgets": []interface{}{}},
		},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		PagesToCreate: data.Pages,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors after conflict-to-update fallback, got %v", result.Errors)
	}
	if result.PagesCreated != 0 {
		t.Fatalf("expected no created pages after conflict fallback, got %d", result.PagesCreated)
	}
	if result.PagesUpdated != 1 {
		t.Fatalf("expected one updated page after conflict fallback, got %d", result.PagesUpdated)
	}
	if strings.Join(calls, ",") != "create,update" {
		t.Fatalf("expected create then update calls, got %v", calls)
	}
	if _, sentType := patchBody["type"]; sentType {
		t.Fatalf("expected update payload to strip page type, got %v", patchBody)
	}
}

func TestImportToTarget_FailedAggPropsRetried(t *testing.T) {
	var mu sync.Mutex
	aggAttempts := make(map[string]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": id},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if _, hasAgg := body["aggregationProperties"]; hasAgg {
				mu.Lock()
				aggAttempts[id]++
				attempt := aggAttempts[id]
				mu.Unlock()
				if attempt == 1 {
					w.WriteHeader(http.StatusUnprocessableEntity)
					json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "relation not found"})
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	data := &export.Data{
		Blueprints: []api.Blueprint{{
			"identifier": "component",
			"aggregationProperties": map[string]interface{}{
				"bugs": map[string]interface{}{"target": "snykTarget"},
			},
		}},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if aggAttempts["component"] != 2 {
		t.Errorf("expected 2 agg prop attempts (original + retry), got %d", aggAttempts["component"])
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors after successful retry, got: %v", result.Errors)
	}
}

func TestImportToTarget_OwnershipAppliedInTopologicalOrder(t *testing.T) {
	var mu sync.Mutex
	var ownershipOrder []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprint": map[string]interface{}{
					"identifier": id,
					"relations":  map[string]interface{}{},
				},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if _, hasOwnership := body["ownership"]; hasOwnership {
				mu.Lock()
				ownershipOrder = append(ownershipOrder, id)
				mu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	// service has direct ownership; component inherits from service via relation.
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "service",
				"ownership":  map[string]interface{}{"type": "Direct"},
			},
			{
				"identifier": "component",
				"relations": map[string]interface{}{
					"service": map[string]interface{}{"target": "service"},
				},
				"ownership": map[string]interface{}{"type": "Inherited", "path": "service"},
			},
		},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}

	serviceIdx, componentIdx := -1, -1
	for i, id := range ownershipOrder {
		if id == "service" {
			serviceIdx = i
		}
		if id == "component" {
			componentIdx = i
		}
	}
	if serviceIdx == -1 || componentIdx == -1 {
		t.Fatalf("expected both service and component ownership updates, got order: %v", ownershipOrder)
	}
	if serviceIdx >= componentIdx {
		t.Errorf("service ownership must be applied before component (inherited), got order: %v", ownershipOrder)
	}
}

func TestImportToTarget_FailedAggPropsRetryAlsoFails_ReportsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": id},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if _, hasAgg := body["aggregationProperties"]; hasAgg {
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "relation not found"})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	data := &export.Data{
		Blueprints: []api.Blueprint{{
			"identifier": "component",
			"aggregationProperties": map[string]interface{}{
				"bugs": map[string]interface{}{"target": "missingBP"},
			},
		}},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected an error when agg prop retry also fails")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "component") && strings.Contains(e, "aggregationProperties") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning component aggregationProperties, got: %v", result.Errors)
	}
}

func TestImportToTarget_FailedMirrorPropsRetriedAfterAggProps(t *testing.T) {
	var mu sync.Mutex
	mirrorAttempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case r.Method == "GET" && r.URL.Path == "/blueprints":
			// "service" already exists in the target (it's the relation
			// target of "component" below, and is skipped from this run's
			// diff via BlueprintsToSkip) — the relation validation now
			// queries the target's real state directly.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case r.Method == "POST" && r.URL.Path == "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": map[string]interface{}{}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			id := strings.TrimPrefix(r.URL.Path, "/blueprints/")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":        true,
				"blueprint": map[string]interface{}{"identifier": id},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/blueprints/"):
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if _, hasMirror := body["mirrorProperties"]; hasMirror {
				mu.Lock()
				mirrorAttempts++
				attempt := mirrorAttempts
				mu.Unlock()
				if attempt == 1 {
					w.WriteHeader(http.StatusUnprocessableEntity)
					json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "property not found"})
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprint": body})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	data := &export.Data{
		Blueprints: []api.Blueprint{{
			"identifier": "component",
			"relations": map[string]interface{}{
				"service": map[string]interface{}{"target": "service"},
			},
			"mirrorProperties": map[string]interface{}{
				"serviceName": map[string]interface{}{"path": "service.name"},
			},
		}},
		Entities: []api.Entity{}, Scorecards: []api.Scorecard{}, Actions: []api.Action{},
		Teams: []api.Team{}, Users: []api.User{}, Folders: []api.Folder{},
		Pages: []api.Page{}, Integrations: []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		BlueprintsToCreate: data.Blueprints,
		BlueprintsToSkip:   []api.Blueprint{{"identifier": "service"}},
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mirrorAttempts != 2 {
		t.Errorf("expected 2 mirror prop attempts (original + retry), got %d", mirrorAttempts)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors after successful retry, got: %v", result.Errors)
	}
}

func TestImportToTarget_UsersCreated(t *testing.T) {
	type capturedReq struct {
		entities []map[string]interface{}
		upsert   string
	}
	var mu sync.Mutex
	var requests []capturedReq

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints/_user/entities/bulk":
			if r.Method == http.MethodPost {
				var payload struct {
					Entities []map[string]interface{} `json:"entities"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				mu.Lock()
				requests = append(requests, capturedReq{entities: payload.Entities, upsert: r.URL.Query().Get("upsert")})
				mu.Unlock()
				json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	alice := api.User{"email": "alice@example.com", "type": "MEMBER"}
	bob := api.User{"email": "bob@example.com", "type": "ADMIN"}

	data := &export.Data{
		Blueprints:           []api.Blueprint{},
		Entities:             []api.Entity{},
		Scorecards:           []api.Scorecard{},
		Actions:              []api.Action{},
		Teams:                []api.Team{},
		Users:                []api.User{alice, bob},
		Folders:              []api.Folder{},
		Pages:                []api.Page{},
		Integrations:         []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		UsersToCreate: []api.User{alice},
		UsersToUpdate: []api.User{bob},
	}

	result, err := m.importToTarget(context.Background(), data, diff, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UsersCreated != 1 {
		t.Errorf("UsersCreated = %d; want 1", result.UsersCreated)
	}
	if result.UsersUpdated != 1 {
		t.Errorf("UsersUpdated = %d; want 1", result.UsersUpdated)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 bulk API calls, got %d", len(requests))
	}
	if requests[0].upsert != "false" {
		t.Errorf("create call: expected upsert=false, got %q", requests[0].upsert)
	}
	if requests[1].upsert != "true" {
		t.Errorf("update call: expected upsert=true, got %q", requests[1].upsert)
	}
}

func TestImportToTarget_UsersAsDisabled(t *testing.T) {
	var mu sync.Mutex
	var capturedEntities []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints/_user/entities/bulk":
			if r.Method == http.MethodPost {
				var payload struct {
					Entities []map[string]interface{} `json:"entities"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				mu.Lock()
				capturedEntities = append(capturedEntities, payload.Entities...)
				mu.Unlock()
				json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}

	alice := api.User{"email": "alice@example.com", "type": "MEMBER"}
	carol := api.User{"email": "carol@example.com", "type": "ADMIN"}

	data := &export.Data{
		Blueprints:           []api.Blueprint{},
		Entities:             []api.Entity{},
		Scorecards:           []api.Scorecard{},
		Actions:              []api.Action{},
		Teams:                []api.Team{},
		Users:                []api.User{alice, carol},
		Folders:              []api.Folder{},
		Pages:                []api.Page{},
		Integrations:         []api.Integration{},
		BlueprintPermissions: map[string]api.Permissions{},
		ActionPermissions:    map[string]api.Permissions{},
		PagePermissions:      map[string]api.Permissions{},
	}
	diff := &import_module.DiffResult{
		UsersToCreate: []api.User{alice, carol},
	}

	_, err := m.importToTarget(context.Background(), data, diff, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statusByEmail := make(map[string]string)
	for _, e := range capturedEntities {
		email, _ := e["identifier"].(string)
		props, _ := e["properties"].(map[string]interface{})
		status, _ := props["status"].(string)
		statusByEmail[email] = status
	}

	if statusByEmail["alice@example.com"] != "DISABLED" {
		t.Errorf("alice (MEMBER) usersAsDisabled=true: status = %q; want DISABLED", statusByEmail["alice@example.com"])
	}
	if statusByEmail["carol@example.com"] != "STAGED" {
		t.Errorf("carol (ADMIN) usersAsDisabled=true: status = %q; want STAGED", statusByEmail["carol@example.com"])
	}
}

func TestMigrate_EntitiesUseBulkEndpoint(t *testing.T) {
	var bulkCalls int
	var singleEntityCalls int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/blueprints" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
			return
		}
		if strings.Contains(r.URL.Path, "/entities/bulk") {
			mu.Lock()
			bulkCalls++
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			return
		}
		if (r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.Contains(r.URL.Path, "/entities") {
			mu.Lock()
			singleEntityCalls++
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "entity": map[string]interface{}{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	targetClient := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})

	entities := []api.Entity{
		{"identifier": "svc-1", "blueprint": "service"},
		{"identifier": "svc-2", "blueprint": "service"},
		{"identifier": "svc-3", "blueprint": "service"},
	}
	entitiesToCreate := map[string]bool{
		"service:svc-1": true,
		"service:svc-2": true,
		"service:svc-3": true,
	}
	entitiesToUpdate := map[string]bool{}

	importResult := &import_module.Result{}
	entityImporter := import_module.NewImporter(targetClient)
	filtered := filterEntitiesByDiff(entities, entitiesToCreate, entitiesToUpdate)
	err := entityImporter.ImportEntities(context.Background(), filtered, false, importResult)
	if err != nil {
		t.Fatalf("ImportEntities returned error: %v", err)
	}
	if singleEntityCalls != 0 {
		t.Errorf("must not call single entity endpoints, got %d calls", singleEntityCalls)
	}
	if bulkCalls != 1 {
		t.Errorf("3 entities = 1 bulk call, got %d", bulkCalls)
	}
	if importResult.EntitiesCreated != 3 {
		t.Errorf("expected 3 entities created, got %d", importResult.EntitiesCreated)
	}
}

func TestExportFromSource_OrgWideActionOnly_ScopesBlueprintsToReferenced(t *testing.T) {
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
		case "/blueprints/service/actions", "/blueprints/domain/actions":
			w.WriteHeader(http.StatusGone)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "deprecated"})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{"identifier": "deploy", "trigger": map[string]interface{}{"blueprintIdentifier": "service", "type": "self-service"}},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 1 || data.Blueprints[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service' blueprint (referenced via org-wide action), got %v", data.Blueprints)
	}
}

func TestExportFromSource_ActionsUseSingleOrgWideEndpointAndExcludeAutomations(t *testing.T) {
	var actionsHits atomic.Int32
	var legacyHits atomic.Int32
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
		case "/blueprints/service/actions", "/blueprints/domain/actions":
			legacyHits.Add(1)
			w.WriteHeader(http.StatusGone)
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "deprecated"})
		case "/actions":
			actionsHits.Add(1)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{"identifier": "deploy", "trigger": map[string]interface{}{"blueprintIdentifier": "service", "type": "self-service"}},
					{"identifier": "ttl-expire", "trigger": map[string]interface{}{"type": "automation", "event": map[string]interface{}{"blueprintIdentifier": "service"}}},
					{"identifier": "org_action"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"actions"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actionsHits.Load() != 1 {
		t.Fatalf("expected one /actions call, got %d", actionsHits.Load())
	}
	if legacyHits.Load() != 0 {
		t.Fatalf("legacy per-blueprint action endpoint was called %d time(s)", legacyHits.Load())
	}
	if len(data.Actions) != 2 {
		t.Fatalf("expected 2 non-automation actions, got %d: %#v", len(data.Actions), data.Actions)
	}
}

func TestExportFromSource_AutomationsIncludeOnlyAutomationTriggers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []map[string]interface{}{{"identifier": "service"}}})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{"identifier": "deploy", "trigger": map[string]interface{}{"blueprintIdentifier": "service", "type": "self-service"}},
					{"identifier": "ttl-expire", "trigger": map[string]interface{}{"type": "automation", "event": map[string]interface{}{"blueprintIdentifier": "service"}}},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	m := &Module{
		sourceClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
		targetClient: api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL}),
	}
	data, _, _, err := m.exportFromSource(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"automations"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Actions) != 1 || data.Actions[0]["identifier"] != "ttl-expire" {
		t.Fatalf("expected only automation action, got %#v", data.Actions)
	}
}
