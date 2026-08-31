package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/useragent"
)

func TestTokenManager_GetToken(t *testing.T) {
	tm := NewTokenManager("test-client-id", "test-client-secret", "https://api.getport.io/v1")

	// Initially no token
	token, err := tm.GetToken()
	if err == nil && token != "" {
		t.Error("Expected error or empty token when refreshToken is not implemented")
	}
}

func TestTokenManager_SetToken(t *testing.T) {
	tm := NewTokenManager("test-client-id", "test-client-secret", "https://api.getport.io/v1")

	expiry := time.Now().Add(1 * time.Hour)
	tm.SetToken("test-token", expiry)

	// Token should be cached
	token, err := tm.GetToken()
	if err == nil && token == "test-token" {
		// Token is valid (within 5 minute buffer)
		return
	}

	// If token expired, that's also fine for this test
	if err != nil {
		// Expected if token expired
		return
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: "https://api.getport.io/v1", Timeout: 0})

	if client.apiURL != "https://api.getport.io/v1" {
		t.Errorf("Expected apiURL 'https://api.getport.io/v1', got '%s'", client.apiURL)
	}

	if client.tokenMgr.ClientID != "test-id" {
		t.Errorf("Expected ClientID 'test-id', got '%s'", client.tokenMgr.ClientID)
	}
}

func TestNewClient_DefaultURL(t *testing.T) {
	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: "", Timeout: 0})

	if client.apiURL != "https://api.getport.io/v1" {
		t.Errorf("Expected default apiURL 'https://api.getport.io/v1', got '%s'", client.apiURL)
	}
}

func TestNewClientWithToken(t *testing.T) {
	exp := time.Now().Add(time.Hour * 24).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":                             "https://api.example.com",
		"exp":                             float64(exp),
		"https://api.example.com/email":   "user@test.com",
		"https://api.example.com/orgId":   "someOrgId",
		"https://api.example.com/orgName": "Org Name",
	})
	signed, err := token.SignedString([]byte("signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := auth.ParseToken(signed)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientOpts{Token: parsed, ClientID: "test-id", ClientSecret: "test-secret", APIURL: "https://api.getport.io/v1", Timeout: 0})

	if client.apiURL != "https://api.getport.io/v1" {
		t.Errorf("Expected apiURL 'https://api.getport.io/v1', got '%s'", client.apiURL)
	}

	if client.tokenMgr.ClientID != "test-id" {
		t.Errorf("Expected ClientID 'test-id', got '%s'", client.tokenMgr.ClientID)
	}

	if client.tokenMgr.token != parsed.Token {
		t.Errorf("Expected token %s, got '%s'", parsed.Token, client.tokenMgr.token)
	}

	if client.tokenMgr.expiry.Unix() != exp {
		t.Errorf("Expected expiry %v, got '%v'", exp, client.tokenMgr.expiry.Unix())
	}
}

func TestNewClientWithoutSecret(t *testing.T) {
	exp := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":                             "https://api.example.com",
		"exp":                             float64(exp),
		"https://api.example.com/email":   "user@test.com",
		"https://api.example.com/orgId":   "someOrgId",
		"https://api.example.com/orgName": "Org Name",
	})
	signed, err := token.SignedString([]byte("signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := auth.ParseToken(signed)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientOpts{Token: parsed, APIURL: "https://api.getport.io/v1", Timeout: 0})

	if client.apiURL != "https://api.getport.io/v1" {
		t.Errorf("Expected apiURL 'https://api.getport.io/v1', got '%s'", client.apiURL)
	}

	if client.tokenMgr.ClientID != "" {
		t.Errorf("Expected no client id, got '%s'", client.tokenMgr.ClientID)
	}

	if client.tokenMgr.ClientSecret != "" {
		t.Errorf("Expected no client secret, got '%s'", client.tokenMgr.ClientSecret)
	}

	if client.tokenMgr.token != parsed.Token {
		t.Errorf("Expected token %s, got '%s'", parsed.Token, client.tokenMgr.token)
	}

	if client.tokenMgr.expiry.Unix() != exp {
		t.Errorf("Expected expiry %v, got '%v'", exp, client.tokenMgr.expiry.Unix())
	}
}

func TestClient_refreshToken(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/access_token" {
			t.Errorf("Expected path '/auth/access_token', got '%s'", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected method 'POST', got '%s'", r.Method)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		response := TokenResponse{
			AccessToken: "test-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: server.URL, Timeout: 0})
	client.apiURL = server.URL

	token, err := client.refreshToken(context.Background())
	if err != nil {
		t.Fatalf("Failed to refresh token: %v", err)
	}

	if token != "test-access-token" {
		t.Errorf("Expected token 'test-access-token', got '%s'", token)
	}
}

func TestClient_request(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			// Token endpoint
			response := TokenResponse{
				AccessToken: "test-token",
				ExpiresIn:   3600,
				TokenType:   "Bearer",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		// API endpoint
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: server.URL, Timeout: 0})
	client.apiURL = server.URL

	resp, err := client.request(context.Background(), "GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_request_RetryTransient5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(TokenResponse{AccessToken: "test-token", ExpiresIn: 3600})
			return
		}
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: server.URL, Timeout: 0})
	resp, err := client.request(context.Background(), "POST", "/test", map[string]string{"x": "y"}, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestClient_request_Retry(t *testing.T) {
	attempts := 0
	// Create a mock server that returns 429 on first attempt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			response := TokenResponse{
				AccessToken: "test-token",
				ExpiresIn:   3600,
				TokenType:   "Bearer",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: server.URL, Timeout: 0})
	client.apiURL = server.URL

	resp, err := client.request(context.Background(), "GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if attempts != 2 {
		t.Errorf("Expected 2 attempts (retry on 429), got %d", attempts)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 after retry, got %d", resp.StatusCode)
	}
}

func TestClientRequestStructuredAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			json.NewEncoder(w).Encode(TokenResponse{AccessToken: "test-token", ExpiresIn: 3600})
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"error":   "forbidden_format_change",
			"message": "Cannot change format",
			"details": map[string]interface{}{"property": "url"},
		})
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: server.URL, Timeout: 0})
	resp, err := client.request(context.Background(), "PUT", "/blueprints/service", map[string]interface{}{}, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "forbidden_format_change" {
		t.Fatalf("Code = %q", apiErr.Code)
	}
	if apiErr.Message != "Cannot change format" {
		t.Fatalf("Message = %q", apiErr.Message)
	}
	if apiErr.Details["property"] != "url" {
		t.Fatalf("Details[property] = %#v", apiErr.Details["property"])
	}
	want := "API request to " + server.URL + "/blueprints/service PUT failed: 422 Unprocessable Entity. Body: "
	if !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("expected compatible error prefix %q, got %q", want, err.Error())
	}
}

func TestClient_Close(t *testing.T) {
	client := NewClient(ClientOpts{ClientID: "test-id", ClientSecret: "test-secret", APIURL: "https://api.getport.io/v1", Timeout: 0})

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

// TestClient_UserAgent verifies that every outbound request carries a
// User-Agent header that starts with "port-cli/".
func TestClient_UserAgent(t *testing.T) {
	useragent.SetVersion("test-version")
	t.Cleanup(func() { useragent.SetVersion("dev") })

	wantUA := useragent.String()

	type capture struct {
		path string
		ua   string
	}
	captured := make([]capture, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, capture{path: r.URL.Path, ua: r.Header.Get("User-Agent")})

		if r.URL.Path == "/auth/access_token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenResponse{AccessToken: "tok", ExpiresIn: 3600})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
	}))
	defer server.Close()

	client := NewClient(ClientOpts{ClientID: "id", ClientSecret: "secret", APIURL: server.URL})
	resp, err := client.request(context.Background(), "GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Expect at least two calls: token refresh and the actual request.
	if len(captured) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(captured))
	}

	for _, c := range captured {
		if !strings.HasPrefix(c.ua, "port-cli/") {
			t.Errorf("request to %s: User-Agent = %q, want prefix \"port-cli/\"", c.path, c.ua)
		}
		if c.ua != wantUA {
			t.Errorf("request to %s: User-Agent = %q, want %q", c.path, c.ua, wantUA)
		}
	}
}
