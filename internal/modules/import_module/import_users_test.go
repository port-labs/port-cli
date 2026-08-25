package import_module

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestUserStatusForCreate(t *testing.T) {
	tests := []struct {
		name            string
		user            api.User
		usersAsDisabled bool
		want            string
	}{
		{"disabled=false admin", api.User{"type": "ADMIN"}, false, "STAGED"},
		{"disabled=false member", api.User{"type": "MEMBER"}, false, "STAGED"},
		{"disabled=false no type", api.User{}, false, "STAGED"},
		{"disabled=true admin", api.User{"type": "ADMIN"}, true, "STAGED"},
		{"disabled=true member", api.User{"type": "MEMBER"}, true, "DISABLED"},
		{"disabled=true no type", api.User{}, true, "DISABLED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := UserStatusForCreate(tc.user, tc.usersAsDisabled)
			if got != tc.want {
				t.Errorf("UserStatusForCreate(%v, %v) = %q; want %q", tc.user, tc.usersAsDisabled, got, tc.want)
			}
		})
	}
}

func TestUserToEntity(t *testing.T) {
	t.Run("identifier is email", func(t *testing.T) {
		u := api.User{"email": "alice@example.com", "firstName": "Alice", "lastName": "Smith"}
		e := UserToEntity(u, "STAGED")
		if e["identifier"] != "alice@example.com" {
			t.Errorf("identifier = %v; want alice@example.com", e["identifier"])
		}
	})

	t.Run("title from first+last name", func(t *testing.T) {
		u := api.User{"email": "a@b.com", "firstName": "Bob", "lastName": "Jones"}
		e := UserToEntity(u, "")
		if e["title"] != "Bob Jones" {
			t.Errorf("title = %v; want 'Bob Jones'", e["title"])
		}
	})

	t.Run("title falls back to email when names empty", func(t *testing.T) {
		u := api.User{"email": "only@email.com"}
		e := UserToEntity(u, "")
		if e["title"] != "only@email.com" {
			t.Errorf("title = %v; want 'only@email.com'", e["title"])
		}
	})

	t.Run("statusOverride sets status in properties", func(t *testing.T) {
		u := api.User{"email": "a@b.com"}
		e := UserToEntity(u, "STAGED")
		props, _ := e["properties"].(map[string]interface{})
		if props["status"] != "STAGED" {
			t.Errorf("properties.status = %v; want STAGED", props["status"])
		}
	})

	t.Run("empty statusOverride keeps source status", func(t *testing.T) {
		u := api.User{"email": "a@b.com", "status": "ACTIVE"}
		e := UserToEntity(u, "")
		props, _ := e["properties"].(map[string]interface{})
		if props["status"] != "ACTIVE" {
			t.Errorf("properties.status = %v; want ACTIVE", props["status"])
		}
	})

	t.Run("system fields stripped from properties", func(t *testing.T) {
		u := api.User{
			"email":     "a@b.com",
			"id":        "sys-id",
			"createdAt": "2025-01-01",
			"updatedAt": "2025-01-02",
			"createdBy": "admin",
			"updatedBy": "admin",
			"role":      "member",
		}
		e := UserToEntity(u, "")
		props, _ := e["properties"].(map[string]interface{})
		for _, sys := range []string{"id", "createdAt", "updatedAt", "createdBy", "updatedBy"} {
			if _, ok := props[sys]; ok {
				t.Errorf("system field %q should be stripped from properties", sys)
			}
		}
		if props["role"] != "member" {
			t.Errorf("non-system field 'role' should be kept")
		}
	})
}

func TestImportUsers_ConflictTriggersUpsert(t *testing.T) {
	var callCount int32

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/blueprints/_user/entities/bulk" {
			n := atomic.AddInt32(&callCount, 1)
			upsert := r.URL.Query().Get("upsert")

			if n == 1 {
				// First call: upsert=false — alice conflicts, bob succeeds
				if upsert != "false" {
					t.Errorf("first call: expected upsert=false, got %q", upsert)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errors": []map[string]interface{}{
						{"identifier": "alice@example.com", "statusCode": float64(409), "error": "Conflict", "message": "already exists"},
					},
				})
			} else {
				// Second call: upsert=true — update alice
				if upsert != "true" {
					t.Errorf("second call: expected upsert=true, got %q", upsert)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			}
			return
		}
		http.NotFound(w, r)
	})

	users := []api.User{
		{"email": "alice@example.com", "type": "MEMBER"},
		{"email": "bob@example.com", "type": "ADMIN"},
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importUsers(context.Background(), users, result, false)

	if result.UsersCreated != 1 {
		t.Errorf("UsersCreated = %d; want 1", result.UsersCreated)
	}
	if result.UsersUpdated != 1 {
		t.Errorf("UsersUpdated = %d; want 1", result.UsersUpdated)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("API calls = %d; want 2", callCount)
	}
}

func TestImportUsers_TransportError(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/blueprints/_user/entities/bulk" {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	})

	users := []api.User{
		{"email": "alice@example.com"},
		{"email": "bob@example.com"},
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importUsers(context.Background(), users, result, false)

	if result.UsersCreated != 0 {
		t.Errorf("UsersCreated = %d; want 0 on transport error", result.UsersCreated)
	}
	// Flush errors from collector into a slice to assert
	errs := importer.errors.ToStringSlice()
	if len(errs) == 0 {
		t.Error("expected errors to be recorded on transport failure")
	}
}

func TestImportUsers_SkipsEmptyEmail(t *testing.T) {
	var callCount int32

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/blueprints/_user/entities/bulk" {
			atomic.AddInt32(&callCount, 1)
			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			entities, _ := payload["entities"].([]interface{})
			// Only valid@example.com should appear
			if len(entities) != 1 {
				t.Errorf("expected 1 entity in bulk request, got %d", len(entities))
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			return
		}
		http.NotFound(w, r)
	})

	users := []api.User{
		{"email": ""},                  // empty email — skip
		{"firstName": "NoEmail"},       // missing email key — skip
		{"email": "valid@example.com"}, // valid
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importUsers(context.Background(), users, result, false)

	if result.UsersCreated != 1 {
		t.Errorf("UsersCreated = %d; want 1", result.UsersCreated)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
}

func TestImportUsers_BatchBoundary(t *testing.T) {
	var callCount int32

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if authHandler(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/blueprints/_user/entities/bulk" {
			atomic.AddInt32(&callCount, 1)
			json.NewEncoder(w).Encode(map[string]interface{}{"errors": []interface{}{}})
			return
		}
		http.NotFound(w, r)
	})

	// 21 users → 2 batches (20 + 1)
	users := make([]api.User, 21)
	for i := range users {
		users[i] = api.User{"email": fmt.Sprintf("user%d@example.com", i)}
	}

	importer := NewImporter(client)
	result := &Result{}
	importer.importUsers(context.Background(), users, result, false)

	if result.UsersCreated != 21 {
		t.Errorf("UsersCreated = %d; want 21", result.UsersCreated)
	}
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 API calls for 21 users, got %d", callCount)
	}
}
