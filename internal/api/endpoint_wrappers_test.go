package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/port-labs/port-cli/internal/auth"
)

func testClientWithHandler(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	token := &auth.Token{Token: "tok", Claims: auth.Claims{Expiry: time.Now().Add(time.Hour)}}
	return NewClient(ClientOpts{Token: token, APIURL: server.URL})
}

func TestBlueprintEndpointWrappers(t *testing.T) {
	tests := []struct {
		name   string
		call   func(context.Context, *Client) error
		method string
		path   string
	}{
		{name: "list", call: func(ctx context.Context, c *Client) error { _, err := c.GetBlueprints(ctx); return err }, method: http.MethodGet, path: "/blueprints"},
		{name: "get", call: func(ctx context.Context, c *Client) error { _, err := c.GetBlueprint(ctx, "service"); return err }, method: http.MethodGet, path: "/blueprints/service"},
		{name: "create", call: func(ctx context.Context, c *Client) error {
			_, err := c.CreateBlueprint(ctx, Blueprint{"identifier": "service"})
			return err
		}, method: http.MethodPost, path: "/blueprints"},
		{name: "update", call: func(ctx context.Context, c *Client) error {
			_, err := c.UpdateBlueprint(ctx, "service", Blueprint{"identifier": "service"})
			return err
		}, method: http.MethodPut, path: "/blueprints/service"},
		{name: "patch", call: func(ctx context.Context, c *Client) error {
			_, err := c.PatchBlueprint(ctx, "service", Blueprint{"identifier": "service"})
			return err
		}, method: http.MethodPatch, path: "/blueprints/service"},
		{name: "delete", call: func(ctx context.Context, c *Client) error { return c.DeleteBlueprint(ctx, "service") }, method: http.MethodDelete, path: "/blueprints/service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testClientWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					t.Fatalf("got %s %s, want %s %s", r.Method, r.URL.Path, tt.method, tt.path)
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"blueprints": []map[string]interface{}{{"identifier": "service"}}, "blueprint": map[string]interface{}{"identifier": "service"}})
			})
			if err := tt.call(context.Background(), client); err != nil {
				t.Fatalf("call failed: %v", err)
			}
		})
	}
}
