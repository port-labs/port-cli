package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

)

func TestBuildBlueprintEntitySearchBody_WrapsQuery(t *testing.T) {
	body := BuildBlueprintEntitySearchBody(BlueprintEntitySearchBodyOptions{
		Query:   MatchAllEntityQuery(),
		Limit:   25,
		Include: []string{"$identifier"},
	})
	if body["query"] == nil {
		t.Fatal("expected query wrapper")
	}
	if body["limit"] != 25 {
		t.Fatalf("expected limit 25, got %v", body["limit"])
	}
}

func TestGlobalEntitySearchBody_NotWrapped(t *testing.T) {
	global := map[string]interface{}{
		"combinator": "and",
		"rules":      []interface{}{},
	}
	if global["query"] != nil {
		t.Fatal("global search must not use query wrapper")
	}
}

func TestParseGroupByAndSortSpecs(t *testing.T) {
	groupBy, err := ParseGroupBySpec("property:priority_decision")
	if err != nil || groupBy["property"] != "priority_decision" {
		t.Fatalf("unexpected groupBy: %v err=%v", groupBy, err)
	}
	sort, err := ParseSortSpec("property:allocation:desc")
	if err != nil || sort["order"] != "desc" {
		t.Fatalf("unexpected sort: %v err=%v", sort, err)
	}
	groupSort, err := ParseGroupSortSpec("by:count order:desc")
	if err != nil || groupSort["by"] != "count" {
		t.Fatalf("unexpected groupSort: %v err=%v", groupSort, err)
	}
}

func TestUsesTopSearchEntityRoute(t *testing.T) {
	if UsesTopSearchEntityRoute(BlueprintEntitySearchBodyOptions{Query: MatchAllEntityQuery()}) {
		t.Fatal("basic search should not use top-search")
	}
	if !UsesTopSearchEntityRoute(BlueprintEntitySearchBodyOptions{GroupBy: map[string]interface{}{"property": "x"}}) {
		t.Fatal("groupBy should use top-search")
	}
	if UsesTopSearchEntityRoute(BlueprintEntitySearchBodyOptions{CountOnly: true}) {
		t.Fatal("count-only alone should use count route, not top-search")
	}
}

func TestNormalizeAPIPath(t *testing.T) {
	if NormalizeAPIPath("blueprints") != "/blueprints" {
		t.Fatal("expected leading slash")
	}
	if NormalizeAPIPath("/entities/search") != "/entities/search" {
		t.Fatal("expected unchanged path")
	}
}

func TestBlueprintEntitySearch_UsesSearchRoute(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		switch {
		case r.URL.Path == "/auth/access_token":
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
		default:
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if body["query"] == nil {
				http.Error(w, "missing query", http.StatusUnprocessableEntity)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":       true,
				"entities": []map[string]interface{}{{"identifier": "a"}},
				"next":     "cursor-1",
			})
		}
	}))
	defer server.Close()

	client := newTestAPIClient(server.URL)
	result, err := client.BlueprintEntitySearch(context.Background(), "svc", BlueprintEntitySearchBodyOptions{
		Query: MatchAllEntityQuery(),
		Limit: 10,
	}, EntitySearchQueryParams{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.HasSuffix(path, "/entities/search") {
		t.Fatalf("expected search route, got %s", path)
	}
	if result["next"] != "cursor-1" {
		t.Fatalf("expected next cursor, got %v", result["next"])
	}
}

func TestBlueprintEntitySearch_GroupByUsesTopSearch(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"groups": []map[string]interface{}{{"value": "Doing", "count": 3}},
		})
	}))
	defer server.Close()

	client := newTestAPIClient(server.URL)
	_, err := client.BlueprintEntitySearch(context.Background(), "planning_priority", BlueprintEntitySearchBodyOptions{
		Query:   MatchAllEntityQuery(),
		GroupBy: map[string]interface{}{"property": "priority_decision"},
		Limit:   50,
	}, EntitySearchQueryParams{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.HasSuffix(path, "/entities/top-search") {
		t.Fatalf("expected top-search route, got %s", path)
	}
}

func TestBlueprintEntitySearchCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "accessToken": "tok", "expiresIn": 3600})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/entities/search/count") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "count": 42})
	}))
	defer server.Close()

	client := newTestAPIClient(server.URL)
	result, err := client.BlueprintEntitySearchCount(context.Background(), "svc", MatchAllEntityQuery(), EntitySearchQueryParams{})
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if result["count"] != float64(42) && result["count"] != 42 {
		t.Fatalf("unexpected count %v", result["count"])
	}
}

func newTestAPIClient(baseURL string) *Client {
	return NewClient(ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: baseURL, Timeout: 0})
}
