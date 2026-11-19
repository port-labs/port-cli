package export

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestApplyBlueprintExclusions_Deep(t *testing.T) {
	all := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "_rule_result"},
		{"identifier": "domain"},
	}
	iterList, dataList := ApplyBlueprintExclusions(all, []string{"_rule_result"}, nil)
	// Deep exclusion: removed from both iteration list and data list
	if len(iterList) != 2 {
		t.Errorf("iterList: expected 2, got %d", len(iterList))
	}
	if len(dataList) != 2 {
		t.Errorf("dataList: expected 2, got %d", len(dataList))
	}
	for _, bp := range iterList {
		if bp["identifier"] == "_rule_result" {
			t.Error("deep-excluded blueprint still in iterList")
		}
	}
}

func TestApplyBlueprintExclusions_SchemaOnly(t *testing.T) {
	all := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "_rule_result"},
	}
	iterList, dataList := ApplyBlueprintExclusions(all, nil, []string{"_rule_result"})
	// Schema-only: removed from data list, but KEPT in iteration list (so entities/scorecards/actions still fetched)
	if len(iterList) != 2 {
		t.Errorf("iterList: expected 2 (schema-only keeps blueprint for fetching), got %d", len(iterList))
	}
	if len(dataList) != 1 {
		t.Errorf("dataList: expected 1 (schema excluded from output), got %d", len(dataList))
	}
	for _, bp := range dataList {
		if bp["identifier"] == "_rule_result" {
			t.Error("schema-excluded blueprint still in dataList")
		}
	}
}

func TestApplyBlueprintExclusions_OverlapDeepWins(t *testing.T) {
	all := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "overlap"},
	}
	iterList, dataList := ApplyBlueprintExclusions(all, []string{"overlap"}, []string{"overlap"})
	if len(iterList) != 1 || len(dataList) != 1 {
		t.Errorf("deep should win when id appears in both sets: iterList=%d dataList=%d", len(iterList), len(dataList))
	}
}

func TestApplyBlueprintExclusions_Empty(t *testing.T) {
	all := []api.Blueprint{{"identifier": "service"}}
	iterList, dataList := ApplyBlueprintExclusions(all, nil, nil)
	if len(iterList) != 1 || len(dataList) != 1 {
		t.Error("empty exclusion lists should return unchanged slices")
	}
}

func TestCollector_CollectsBlueprintPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service", "title": "Service"}},
			})
		case "/blueprints/service/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"permissions": map[string]interface{}{"entities": map[string]interface{}{"view": []string{"$team"}}},
			})
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecards": []interface{}{}})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "actions": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL, Timeout: 0})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{SkipEntities: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.BlueprintPermissions["service"] == nil {
		t.Error("expected blueprint permissions for 'service'")
	}
}

func TestCollector_CollectsActionPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service", "title": "Service"}},
			})
		case "/blueprints/service/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecards": []interface{}{}})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "deploy", "title": "Deploy"}},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{{
					"identifier": "deploy",
					"title":      "Deploy",
					"trigger": map[string]interface{}{
						"type":                "self-service",
						"blueprintIdentifier": "service",
					},
				}},
			})
		case "/actions/deploy/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"permissions": map[string]interface{}{"execute": map[string]interface{}{"users": []string{}}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL, Timeout: 0})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{SkipEntities: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ActionPermissions["deploy"] == nil {
		t.Error("expected action permissions for 'deploy'")
	}
}

func TestCollector_ActionPermissionsNotCollectedWhenExcluded(t *testing.T) {
	actionPermsHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "deploy"}},
			})
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
		case "/actions/deploy/permissions":
			actionPermsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "permissions": map[string]interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL, Timeout: 0})
	collector := NewCollector(client)
	_, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "actions"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actionPermsHit {
		t.Error("action permissions endpoint should not be called when action-permissions not in IncludeResources")
	}
}

func TestCollector_SkipSystemBlueprints_ExcludesSchemaAndEntities(t *testing.T) {
	entitiesHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "_user", "title": "User"},
					{"identifier": "service", "title": "Service"},
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{SkipSystemBlueprints: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// _user schema should NOT be in output blueprints
	for _, bp := range data.Blueprints {
		id, _ := bp["identifier"].(string)
		if id == "_user" {
			t.Error("_user blueprint schema should be excluded from output when SkipSystemBlueprints=true")
		}
	}

	// service blueprint should still be present
	found := false
	for _, bp := range data.Blueprints {
		if bp["identifier"] == "service" {
			found = true
		}
	}
	if !found {
		t.Error("non-system blueprint 'service' should remain in output")
	}

	// entities endpoint for _user should NOT be called
	if entitiesHit {
		t.Error("entities endpoint for _user should not be called when SkipSystemBlueprints=true")
	}
}

func TestCollector_SkipSystemBlueprints_KeepsCustomSystemBlueprintPatch(t *testing.T) {
	entitiesHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{
						"identifier": "_user",
						"title":      "User",
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
					{"identifier": "service", "title": "Service"},
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{SkipSystemBlueprints: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var userPatch api.Blueprint
	for _, bp := range data.Blueprints {
		if bp["identifier"] == "_unknown" {
			t.Fatal("unknown system blueprint should be omitted when system blueprints are skipped")
		}
		if bp["identifier"] == "_user" {
			userPatch = bp
		}
	}
	if userPatch == nil {
		t.Fatal("expected _user custom-property patch")
	}
	if _, ok := userPatch["title"]; ok {
		t.Fatalf("expected minimal _user patch without title, got %#v", userPatch)
	}
	props := userPatch["properties"].(map[string]interface{})
	if _, ok := props["status"]; ok {
		t.Fatal("managed _user status property should be stripped")
	}
	if _, ok := props["department"]; !ok {
		t.Fatal("custom _user department property should be preserved")
	}
	if entitiesHit {
		t.Fatal("system blueprint entities should still be skipped")
	}
}

func TestCollector_SkipSystemBlueprintProperties_DropsSystemBlueprintPatches(t *testing.T) {
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
						"properties": map[string]interface{}{
							"description": map[string]interface{}{"type": "string"},
							"cost_center": map[string]interface{}{"type": "string"},
						},
					},
					{"identifier": "service"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipSystemBlueprints:          true,
		SkipSystemBlueprintProperties: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, bp := range data.Blueprints {
		if bp["identifier"] == "_team" {
			t.Fatal("_team patch should be omitted when SkipSystemBlueprintProperties=true")
		}
	}
}

func TestCollector_SkipSystemBlueprints_StillCollectsScorecards(t *testing.T) {
	scorecardsHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"blueprints": []map[string]interface{}{
					{"identifier": "_user", "title": "User"},
				},
			})
		case "/blueprints/_user/scorecards":
			scorecardsHit = true
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "scorecards": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	_, err := collector.Collect(context.Background(), Options{SkipSystemBlueprints: true, SkipEntities: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !scorecardsHit {
		t.Error("scorecards endpoint for _user should still be called when SkipSystemBlueprints=true (shallow skip)")
	}
}

func TestCollector_SkipEntities_SkipsTeamsAndUsers(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	_, err := collector.Collect(context.Background(), Options{SkipEntities: true})
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

func TestCollector_CollectsPagePermissions(t *testing.T) {
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"permissions": map[string]interface{}{"read": map[string]interface{}{"roles": []string{"Admin"}}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL, Timeout: 0})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{SkipEntities: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.PagePermissions["home"] == nil {
		t.Error("expected page permissions for 'home'")
	}
}

// --- ID filter unit tests ---

func TestFilterByField(t *testing.T) {
	items := []api.Entity{
		{"identifier": "ent1", "title": "E1"},
		{"identifier": "ent2", "title": "E2"},
		{"identifier": "ent3", "title": "E3"},
	}

	t.Run("empty filter returns all", func(t *testing.T) {
		result := FilterByField(items, nil, "identifier")
		if len(result) != 3 {
			t.Errorf("expected 3, got %d", len(result))
		}
	})

	t.Run("filters to matching IDs", func(t *testing.T) {
		result := FilterByField(items, []string{"ent1", "ent3"}, "identifier")
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
		for _, item := range result {
			id := item["identifier"].(string)
			if id != "ent1" && id != "ent3" {
				t.Errorf("unexpected item: %s", id)
			}
		}
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		result := FilterByField(items, []string{"nonexistent"}, "identifier")
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("works with different field names", func(t *testing.T) {
		teams := []api.Team{
			{"name": "Backend"},
			{"name": "Frontend"},
		}
		result := FilterByField(teams, []string{"Backend"}, "name")
		if len(result) != 1 || result[0]["name"] != "Backend" {
			t.Errorf("expected Backend team, got %v", result)
		}
	})

	t.Run("works with email field", func(t *testing.T) {
		users := []api.User{
			{"email": "alice@co.com"},
			{"email": "bob@co.com"},
		}
		result := FilterByField(users, []string{"alice@co.com"}, "email")
		if len(result) != 1 || result[0]["email"] != "alice@co.com" {
			t.Errorf("expected alice, got %v", result)
		}
	})

	t.Run("works with installationId field", func(t *testing.T) {
		integrations := []api.Integration{
			{"installationId": "int1", "name": "GitHub"},
			{"installationId": "int2", "name": "GitLab"},
		}
		result := FilterByField(integrations, []string{"int1"}, "installationId")
		if len(result) != 1 || result[0]["installationId"] != "int1" {
			t.Errorf("expected int1, got %v", result)
		}
	})
}

// --- Integration tests for ID filtering in Collect ---

func TestCollector_EntityFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/entities":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"entities": []map[string]interface{}{
					{"identifier": "ent1", "blueprint": "service"},
					{"identifier": "ent2", "blueprint": "service"},
					{"identifier": "ent3", "blueprint": "service"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		IncludeResources: []string{"blueprints", "entities"},
		Entities:         []string{"ent1", "ent3"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(data.Entities))
	}
}

func TestCollector_ScorecardFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc1", "blueprintIdentifier": "service"},
					{"identifier": "sc2", "blueprintIdentifier": "service"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "scorecards"},
		Scorecards:       []string{"sc1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Scorecards) != 1 {
		t.Errorf("expected 1 scorecard, got %d", len(data.Scorecards))
	}
	if data.Scorecards[0]["identifier"] != "sc1" {
		t.Errorf("expected sc1, got %s", data.Scorecards[0]["identifier"])
	}
}

func TestCollector_ActionFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{"identifier": "deploy"},
					{"identifier": "restart"},
				},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{
						"identifier": "deploy",
						"trigger": map[string]interface{}{
							"type":                "self-service",
							"blueprintIdentifier": "service",
						},
					},
					{
						"identifier": "restart",
						"trigger": map[string]interface{}{
							"type":                "self-service",
							"blueprintIdentifier": "service",
						},
					},
					{"identifier": "org_action"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "actions"},
		Actions:          []string{"deploy", "org_action"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Actions) != 2 {
		t.Errorf("expected 2 actions (deploy + org_action), got %d", len(data.Actions))
	}
}

func TestCollector_TeamFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/teams":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"teams": []map[string]interface{}{
					{"name": "Backend"},
					{"name": "Frontend"},
					{"name": "DevOps"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		IncludeResources: []string{"teams"},
		Teams:            []string{"Backend"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Teams) != 1 || data.Teams[0]["name"] != "Backend" {
		t.Errorf("expected 1 team (Backend), got %v", data.Teams)
	}
}

func TestCollector_UserFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/users":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"users": []map[string]interface{}{
					{"email": "alice@co.com"},
					{"email": "bob@co.com"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		IncludeResources: []string{"users"},
		Users:            []string{"alice@co.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Users) != 1 || data.Users[0]["email"] != "alice@co.com" {
		t.Errorf("expected 1 user (alice), got %v", data.Users)
	}
}

func TestCollector_PageFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/pages":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"pages": []map[string]interface{}{
					{"identifier": "home"},
					{"identifier": "dashboard"},
					{"identifier": "settings"},
				},
			})
		case "/folders":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "folders": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"pages"},
		Pages:            []string{"home", "settings"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Pages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(data.Pages))
	}
}

func TestCollector_IntegrationFilter(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
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
}

func TestCollector_NoFilterReturnsAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc1"},
					{"identifier": "sc2"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "scorecards"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Scorecards) != 2 {
		t.Errorf("no filter should return all scorecards, got %d", len(data.Scorecards))
	}
}

func TestCollector_CombinedBlueprintAndScorecardFilter(t *testing.T) {
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
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc1", "blueprintIdentifier": "service"},
					{"identifier": "sc2", "blueprintIdentifier": "service"},
				},
			})
		case "/blueprints/domain/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc3", "blueprintIdentifier": "domain"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		Blueprints:       []string{"service"},
		IncludeResources: []string{"blueprints", "scorecards"},
		Scorecards:       []string{"sc1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Scorecards) != 1 {
		t.Errorf("expected 1 scorecard (sc1 from service only), got %d", len(data.Scorecards))
	}
}

func TestCollector_PagePermissionsNotCollectedWhenExcluded(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	_, err := collector.Collect(context.Background(), Options{
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

func TestFilterFoldersToAncestors(t *testing.T) {
	folders := []api.Folder{
		{"identifier": "root_folder"},
		{"identifier": "child_folder", "parent": "root_folder"},
		{"identifier": "unrelated_folder"},
		{"identifier": "deep_folder", "parent": "child_folder"},
	}

	t.Run("keeps only ancestor chain", func(t *testing.T) {
		pages := []api.Page{
			{"identifier": "my_page", "parent": "child_folder"},
		}
		result := FilterFoldersToAncestors(folders, pages)
		if len(result) != 2 {
			t.Errorf("expected 2 folders (child_folder + root_folder), got %d", len(result))
			return
		}
		ids := map[string]bool{}
		for _, f := range result {
			ids[f["identifier"].(string)] = true
		}
		if !ids["child_folder"] || !ids["root_folder"] {
			t.Errorf("expected child_folder and root_folder, got %v", ids)
		}
	})

	t.Run("page with no parent returns no folders", func(t *testing.T) {
		pages := []api.Page{
			{"identifier": "top_level_page"},
		}
		result := FilterFoldersToAncestors(folders, pages)
		if len(result) != 0 {
			t.Errorf("expected 0 folders for page without parent, got %d", len(result))
		}
	})

	t.Run("multiple pages share ancestor", func(t *testing.T) {
		pages := []api.Page{
			{"identifier": "page_a", "parent": "child_folder"},
			{"identifier": "page_b", "parent": "root_folder"},
		}
		result := FilterFoldersToAncestors(folders, pages)
		if len(result) != 2 {
			t.Errorf("expected 2 unique folders, got %d", len(result))
		}
	})
}

func TestCollector_PageFilter_IncludesPermissionsAndFiltersFolders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "blueprints": []interface{}{}})
		case "/pages":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"pages": []map[string]interface{}{
					{"identifier": "home", "parent": "catalog"},
					{"identifier": "dashboard", "parent": "analytics"},
					{"identifier": "settings"},
				},
			})
		case "/sidebars/catalog":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"identifier": "catalog", "sidebarType": "folder"},
				{"identifier": "analytics", "sidebarType": "folder"},
				{"identifier": "admin", "sidebarType": "folder"},
			})
		case "/pages/home/permissions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"permissions": map[string]interface{}{"read": map[string]interface{}{"roles": []string{"Admin"}}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"pages", "page-permissions"},
		Pages:            []string{"home"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(data.Pages))
	}
	if data.PagePermissions["home"] == nil {
		t.Error("expected page permissions for 'home'")
	}
	if len(data.Folders) != 1 {
		t.Errorf("expected 1 folder (catalog, parent of home), got %d", len(data.Folders))
	} else if data.Folders[0]["identifier"] != "catalog" {
		t.Errorf("expected folder 'catalog', got '%s'", data.Folders[0]["identifier"])
	}
}

func TestCollector_ActionsOnly_RecordsReferencedBlueprintIDs(t *testing.T) {
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "publish"}},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"actions": []map[string]interface{}{
					{
						"identifier": "deploy",
						"trigger": map[string]interface{}{
							"type":                "self-service",
							"blueprintIdentifier": "service",
						},
					},
					{
						"identifier": "publish",
						"trigger": map[string]interface{}{
							"type":                "self-service",
							"blueprintIdentifier": "domain",
						},
					},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		Actions:             []string{"deploy"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data.Blueprints) != 2 {
		t.Fatalf("Collect must NOT narrow Data.Blueprints itself, expected 2, got %d", len(data.Blueprints))
	}
	if !data.ReferencedBlueprintIDs["service"] {
		t.Error("expected 'service' in ReferencedBlueprintIDs (has matching action 'deploy')")
	}
	if data.ReferencedBlueprintIDs["domain"] {
		t.Error("did not expect 'domain' in ReferencedBlueprintIDs (its action 'publish' didn't match the filter)")
	}
}

func TestCollector_ScorecardsOnly_RecordsReferencedBlueprintIDs(t *testing.T) {
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
		case "/blueprints/service/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc1", "blueprintIdentifier": "service"},
				},
			})
		case "/blueprints/domain/scorecards":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"scorecards": []map[string]interface{}{
					{"identifier": "sc2", "blueprintIdentifier": "domain"},
				},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "scorecards"},
		Scorecards:          []string{"sc1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !data.ReferencedBlueprintIDs["service"] || data.ReferencedBlueprintIDs["domain"] {
		t.Fatalf("expected only 'service' referenced, got %v", data.ReferencedBlueprintIDs)
	}
}

func TestCollector_EntitiesOnly_RecordsReferencedBlueprintIDs(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		IncludeResources:    []string{"blueprints", "entities"},
		Entities:            []string{"ent1"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !data.ReferencedBlueprintIDs["service"] || data.ReferencedBlueprintIDs["domain"] {
		t.Fatalf("expected only 'service' referenced, got %v", data.ReferencedBlueprintIDs)
	}
}

func TestCollector_AutoScopeBlueprintsFalse_LeavesReferencedBlueprintIDsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		case "/blueprints":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":         true,
				"blueprints": []map[string]interface{}{{"identifier": "service"}},
			})
		case "/blueprints/service/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"actions": []map[string]interface{}{{"identifier": "deploy"}},
			})
		case "/actions":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "actions": []interface{}{}})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer server.Close()

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:     true,
		IncludeResources: []string{"blueprints", "actions"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ReferencedBlueprintIDs == nil {
		t.Fatal("Data.ReferencedBlueprintIDs must always be non-nil")
	}
	if len(data.ReferencedBlueprintIDs) != 0 {
		t.Errorf("expected empty ReferencedBlueprintIDs when AutoScopeBlueprints is false, got %v", data.ReferencedBlueprintIDs)
	}
}

func TestFilterBlueprintsToReferenced(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "service"},
		{"identifier": "domain"},
	}
	scoped := FilterBlueprintsToReferenced(blueprints, map[string]bool{"service": true})
	if len(scoped) != 1 || scoped[0]["identifier"] != "service" {
		t.Fatalf("expected only 'service', got %v", scoped)
	}
	if empty := FilterBlueprintsToReferenced(blueprints, map[string]bool{}); len(empty) != 0 {
		t.Fatalf("expected empty result for empty referenced set, got %v", empty)
	}
}

func TestSelectActionsForResourcesSeparatesActionsAndAutomations(t *testing.T) {
	all := []api.Action{
		{"identifier": "deploy", "trigger": map[string]interface{}{"blueprintIdentifier": "service", "type": "self-service"}},
		{"identifier": "ttl-expire", "trigger": map[string]interface{}{"type": "automation", "event": map[string]interface{}{"blueprintIdentifier": "service"}}},
		{"identifier": "org_action"},
	}

	actionIDs := func(actions []api.Action) []string {
		ids := make([]string, 0, len(actions))
		for _, action := range actions {
			id, _ := action["identifier"].(string)
			ids = append(ids, id)
		}
		return ids
	}

	if got := actionIDs(SelectActionsForResources(all, true, false, nil)); len(got) != 2 || got[0] != "deploy" || got[1] != "org_action" {
		t.Fatalf("actions selection = %v", got)
	}
	if got := actionIDs(SelectActionsForResources(all, false, true, nil)); len(got) != 1 || got[0] != "ttl-expire" {
		t.Fatalf("automations selection = %v", got)
	}
	if got := actionIDs(SelectActionsForResources(all, true, true, []string{"ttl-expire"})); len(got) != 1 || got[0] != "ttl-expire" {
		t.Fatalf("filtered selection = %v", got)
	}
}

func TestCollector_ActionsUseSingleOrgWideEndpointAndExcludeAutomations(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
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

func TestCollector_AutomationsIncludeOnlyAutomationTriggers(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
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

func TestCollector_OrgWideActionOnly_RecordsReferencedBlueprintIDs(t *testing.T) {
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

	client := api.NewClient(api.ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	collector := NewCollector(client)
	data, err := collector.Collect(context.Background(), Options{
		SkipEntities:        true,
		IncludeResources:    []string{"blueprints", "actions"},
		AutoScopeBlueprints: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !data.ReferencedBlueprintIDs["service"] {
		t.Errorf("expected 'service' referenced via org-wide action's trigger.blueprintIdentifier, got %v", data.ReferencedBlueprintIDs)
	}
	if data.ReferencedBlueprintIDs["domain"] {
		t.Errorf("did not expect 'domain' referenced, got %v", data.ReferencedBlueprintIDs)
	}
}
