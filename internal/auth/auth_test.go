package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseToken(t *testing.T) {
	exp := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":                             "https://api.example.com",
		"exp":                             float64(exp),
		"https://api.example.com/email":   "user@test.com",
		"https://api.example.com/orgId":   "someOrgId",
		"https://api.example.com/orgName": "Org Name",
	})
	ss, err := token.SignedString([]byte("signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseToken(ss)
	if err != nil {
		t.Fatal(err)
	}

	if aud := parsed.Claims.Audience; aud != "https://api.example.com" {
		t.Errorf("expected audience https://api.example.com but got %v", aud)
	}
	if email := parsed.Claims.Email; email != "user@test.com" {
		t.Errorf("expected email user@test.com but got %v", email)
	}
	if orgId := parsed.Claims.OrgId; orgId != "someOrgId" {
		t.Errorf("expected orgId someOrgId but got %v", orgId)
	}
	if orgName := parsed.Claims.OrgName; orgName != "Org Name" {
		t.Errorf("expected orgName Org Name but got %v", orgName)
	}
	if exp != parsed.Claims.Expiry.Unix() {
		t.Errorf("expected expiry %v, got '%v'", exp, parsed.Claims.Expiry.Unix())
	}
}

func makeTestJWT(t *testing.T, audience string, expiry time.Time) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":                 audience,
		"exp":                 float64(expiry.Unix()),
		audience + "/email":   "user@test.com",
		audience + "/orgId":   "someOrgId",
		audience + "/orgName": "Org Name",
	})
	ss, err := token.SignedString([]byte("signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	return ss
}

func TestRefreshAccessToken(t *testing.T) {
	audience := "https://api.example.com"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("expected /oauth/token, got %s", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if payload["grant_type"] != "refresh_token" {
			t.Fatalf("expected refresh_token grant, got %q", payload["grant_type"])
		}
		if payload["refresh_token"] != "old-refresh-token" {
			t.Fatalf("expected old refresh token, got %q", payload["refresh_token"])
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token":  makeTestJWT(t, audience, time.Now().Add(time.Hour)),
			"refresh_token": "new-refresh-token",
		})
	}))
	defer server.Close()

	registerClientID(server.URL, "test-client-id")
	defer unregisterClientID(server.URL)

	token, err := RefreshAccessToken(context.Background(), server.URL, "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken returned error: %v", err)
	}
	if token.RefreshToken != "new-refresh-token" {
		t.Fatalf("expected rotated refresh token, got %q", token.RefreshToken)
	}
	if token.AuthBaseURL != server.URL {
		t.Fatalf("expected auth base URL %q, got %q", server.URL, token.AuthBaseURL)
	}
	if token.Claims.Audience != audience {
		t.Fatalf("expected audience %q, got %q", audience, token.Claims.Audience)
	}
}

func TestRefreshAccessTokenPreservesOldRefreshToken(t *testing.T) {
	audience := "https://api.example.com"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": makeTestJWT(t, audience, time.Now().Add(time.Hour)),
		})
	}))
	defer server.Close()

	registerClientID(server.URL, "test-client-id")
	defer unregisterClientID(server.URL)

	token, err := RefreshAccessToken(context.Background(), server.URL, "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken returned error: %v", err)
	}
	if token.RefreshToken != "old-refresh-token" {
		t.Fatalf("expected original refresh token, got %q", token.RefreshToken)
	}
}

func TestRefreshAccessTokenUnsupportedBaseURL(t *testing.T) {
	_, err := RefreshAccessToken(context.Background(), "https://unsupported.example.com", "refresh-token")
	if err == nil {
		t.Fatal("expected error for unsupported base URL")
	}
}

func TestRefreshAccessTokenInvalidGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Unknown or invalid refresh token."}`))
	}))
	defer server.Close()

	registerClientID(server.URL, "test-client-id")
	defer unregisterClientID(server.URL)

	_, err := RefreshAccessToken(context.Background(), server.URL, "old-refresh-token")
	if err == nil {
		t.Fatal("expected invalid_grant error")
	}
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant, got %v", err)
	}
}
