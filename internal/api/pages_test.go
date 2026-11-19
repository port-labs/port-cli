package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
)

// pageCall records one request made to the test server.
type pageCall struct {
	method string
	path   string
	query  string
	body   map[string]interface{}
}

// pageServer returns a client whose requests are recorded into calls and
// answered with status and response (nil response means an empty body).
func pageServer(t *testing.T, status int, response interface{}, calls *[]pageCall) *Client {
	t.Helper()
	return testClientWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		call := pageCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &call.body); err != nil {
				t.Fatalf("request body is not a JSON object: %s", raw)
			}
		}
		*calls = append(*calls, call)
		w.WriteHeader(status)
		if response != nil {
			_ = json.NewEncoder(w).Encode(response)
		}
	})
}

func assertCall(t *testing.T, calls []pageCall, method, path, query string) pageCall {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("expected 1 request, got %d", len(calls))
	}
	got := calls[0]
	if got.method != method || got.path != path || got.query != query {
		t.Fatalf("got %s %s?%s, want %s %s?%s", got.method, got.path, got.query, method, path, query)
	}
	return got
}

func TestGetPages_ListOptions(t *testing.T) {
	tests := []struct {
		name  string
		opts  PageListOptions
		query string
	}{
		{"defaults send no query", PageListOptions{}, ""},
		{"compact", PageListOptions{Compact: true}, "compact=true"},
		{"include private", PageListOptions{IncludePrivate: true}, "includePrivate=true"},
		{"both", PageListOptions{Compact: true, IncludePrivate: true}, "compact=true&includePrivate=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []pageCall
			client := pageServer(t, http.StatusOK, map[string]interface{}{
				"ok":    true,
				"pages": []map[string]interface{}{{"identifier": "home"}},
			}, &calls)

			pages, err := client.GetPages(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("GetPages: %v", err)
			}
			assertCall(t, calls, http.MethodGet, "/pages", tt.query)
			if len(pages) != 1 || pages[0]["identifier"] != "home" {
				t.Fatalf("unexpected pages: %v", pages)
			}
		})
	}
}

func TestCreatePage_ResponseShapes(t *testing.T) {
	// The reference documents a 201 with no body schema, so every shape below
	// must resolve to the page's identifier.
	tests := []struct {
		name     string
		response interface{}
	}{
		{"page object", map[string]interface{}{"ok": true, "page": map[string]interface{}{"identifier": "svc"}}},
		{"identifier only", map[string]interface{}{"ok": true, "identifier": "svc"}},
		{"ok only", map[string]interface{}{"ok": true}},
		{"empty body", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []pageCall
			client := pageServer(t, http.StatusCreated, tt.response, &calls)

			page := Page{"identifier": "svc", "type": "entity", "requiredQueryParams": []interface{}{"identifier"}, "parent": nil}
			id, err := client.CreatePage(context.Background(), page)
			if err != nil {
				t.Fatalf("CreatePage: %v", err)
			}
			if id != "svc" {
				t.Fatalf("expected identifier svc, got %q", id)
			}
			got := assertCall(t, calls, http.MethodPost, "/pages", "")
			if v, exists := got.body["parent"]; !exists || v != nil {
				t.Errorf("expected parent: null to be sent, got present=%v value=%v", exists, v)
			}
			if !reflect.DeepEqual(got.body["requiredQueryParams"], []interface{}{"identifier"}) {
				t.Errorf("expected requiredQueryParams to be sent, got %v", got.body["requiredQueryParams"])
			}
		})
	}
}

func TestPageWrites_DecodeIdentifierResponse(t *testing.T) {
	tests := []struct {
		name   string
		call   func(context.Context, *Client) (string, error)
		method string
		path   string
		want   string
	}{
		{"patch", func(ctx context.Context, c *Client) (string, error) {
			return c.UpdatePage(ctx, "home", Page{"title": "Home", "after": nil})
		}, http.MethodPatch, "/pages/home", "home"},
		{"put", func(ctx context.Context, c *Client) (string, error) {
			return c.ReplacePage(ctx, "home", Page{"identifier": "home", "type": "dashboard"})
		}, http.MethodPut, "/pages/home", "home"},
		{"duplicate", func(ctx context.Context, c *Client) (string, error) {
			return c.DuplicatePage(ctx, "home")
		}, http.MethodPost, "/pages/home/duplicate", "home_copy"},
		{"widget patch", func(ctx context.Context, c *Client) (string, error) {
			return c.UpdatePageWidget(ctx, "home", "w1", map[string]interface{}{"title": nil})
		}, http.MethodPatch, "/pages/home/widgets/w1", "w1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []pageCall
			status := http.StatusOK
			if tt.name == "duplicate" {
				status = http.StatusCreated
			}
			client := pageServer(t, status, map[string]interface{}{"ok": true, "identifier": tt.want}, &calls)

			got, err := tt.call(context.Background(), client)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected identifier %q, got %q", tt.want, got)
			}
			assertCall(t, calls, tt.method, tt.path, "")
		})
	}
}

func TestUpdatePage_SendsExplicitNulls(t *testing.T) {
	var calls []pageCall
	client := pageServer(t, http.StatusOK, map[string]interface{}{"ok": true, "identifier": "home"}, &calls)

	if _, err := client.UpdatePage(context.Background(), "home", Page{"parent": nil, "after": nil}); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	body := assertCall(t, calls, http.MethodPatch, "/pages/home", "").body
	for _, field := range []string{"parent", "after"} {
		if v, exists := body[field]; !exists || v != nil {
			t.Errorf("expected %s: null in PATCH body, got present=%v value=%v", field, exists, v)
		}
	}
}

func TestPageWrites_MissingIdentifier(t *testing.T) {
	var calls []pageCall
	client := pageServer(t, http.StatusOK, map[string]interface{}{"ok": true}, &calls)

	id, err := client.UpdatePage(context.Background(), "home", Page{"title": "Home"})
	if err != nil || id != "home" {
		t.Fatalf("expected PATCH to fall back to the requested identifier, got %q, %v", id, err)
	}

	calls = nil
	if _, err := client.DuplicatePage(context.Background(), "home"); err == nil {
		t.Fatal("expected duplicate without an identifier in the response to fail")
	}
}

func TestPageWrites_PropagateAPIErrors(t *testing.T) {
	var calls []pageCall
	client := pageServer(t, http.StatusUnprocessableEntity, map[string]interface{}{
		"ok": false, "error": "invalid_request", "message": "The json provided does not match the route's schema",
	}, &calls)

	if _, err := client.UpdatePage(context.Background(), "home", Page{"type": "dashboard"}); err == nil {
		t.Fatal("expected a 422 from PATCH to be returned as an error")
	}
}

func TestGetPagesAPIVersion(t *testing.T) {
	var calls []pageCall
	client := pageServer(t, http.StatusOK, map[string]interface{}{"ok": true, "version": "GA"}, &calls)

	version, err := client.GetPagesAPIVersion(context.Background())
	if err != nil {
		t.Fatalf("GetPagesAPIVersion: %v", err)
	}
	assertCall(t, calls, http.MethodGet, "/pages/version", "")
	if version != "GA" {
		t.Fatalf("expected GA, got %q", version)
	}
}

func TestPagePermissionEndpoints(t *testing.T) {
	createPerms := map[string]interface{}{"create": map[string]interface{}{"roles": []interface{}{"Admin"}}}
	readPerms := map[string]interface{}{"read": map[string]interface{}{"users": []interface{}{"a@example.com"}}}

	tests := []struct {
		name     string
		call     func(context.Context, *Client) (Permissions, error)
		method   string
		path     string
		response map[string]interface{}
		wantBody map[string]interface{}
	}{
		{"org get", func(ctx context.Context, c *Client) (Permissions, error) {
			return c.GetOrgPagePermissions(ctx)
		}, http.MethodGet, "/pages/permissions", createPerms, nil},
		{"org update", func(ctx context.Context, c *Client) (Permissions, error) {
			return c.UpdateOrgPagePermissions(ctx, Permissions(createPerms))
		}, http.MethodPatch, "/pages/permissions", createPerms, createPerms},
		{"page get", func(ctx context.Context, c *Client) (Permissions, error) {
			return c.GetPagePermissions(ctx, "home")
		}, http.MethodGet, "/pages/home/permissions", readPerms, nil},
		{"page update", func(ctx context.Context, c *Client) (Permissions, error) {
			return c.UpdatePagePermissions(ctx, "home", Permissions(readPerms))
		}, http.MethodPatch, "/pages/home/permissions", readPerms, readPerms},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []pageCall
			client := pageServer(t, http.StatusOK, map[string]interface{}{"ok": true, "permissions": tt.response}, &calls)

			got, err := tt.call(context.Background(), client)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			call := assertCall(t, calls, tt.method, tt.path, "")
			if tt.wantBody != nil && !reflect.DeepEqual(call.body, tt.wantBody) {
				t.Errorf("request body = %v, want %v", call.body, tt.wantBody)
			}
			if !reflect.DeepEqual(map[string]interface{}(got), tt.response) {
				t.Errorf("permissions = %v, want %v", got, tt.response)
			}
		})
	}
}

func TestCreatePageWidget(t *testing.T) {
	widget := map[string]interface{}{"type": "markdown", "title": "Notes"}

	for _, response := range []interface{}{nil, map[string]interface{}{"ok": true, "identifier": "w2"}} {
		var calls []pageCall
		client := pageServer(t, http.StatusCreated, response, &calls)

		if _, err := client.CreatePageWidget(context.Background(), "home", "dashboard-widget", widget); err != nil {
			t.Fatalf("CreatePageWidget with response %v: %v", response, err)
		}
		call := assertCall(t, calls, http.MethodPost, "/pages/home/widgets", "")
		want := map[string]interface{}{"parentWidgetId": "dashboard-widget", "widget": widget}
		if !reflect.DeepEqual(call.body, want) {
			t.Errorf("request body = %v, want %v", call.body, want)
		}
	}
}

func TestDeletePageAndWidget(t *testing.T) {
	var calls []pageCall
	client := pageServer(t, http.StatusOK, map[string]interface{}{"ok": true}, &calls)
	if err := client.DeletePageWidget(context.Background(), "home", "w1"); err != nil {
		t.Fatalf("DeletePageWidget: %v", err)
	}
	assertCall(t, calls, http.MethodDelete, "/pages/home/widgets/w1", "")

	calls = nil
	if err := client.DeletePage(context.Background(), "home"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	assertCall(t, calls, http.MethodDelete, "/pages/home", "")
}
