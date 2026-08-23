package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/port-labs/port-cli/internal/auth"
)

func configTestJWT(t *testing.T, audience string, expiry time.Time) string {
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

func TestGetOrRefreshTokenReturnsValidToken(t *testing.T) {
	dir := t.TempDir()
	manager := NewConfigManager(filepath.Join(dir, "config.yaml"))

	token, err := auth.ParseToken(configTestJWT(t, "https://api.example.com", time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if err := manager.StoreToken("test-org", token); err != nil {
		t.Fatalf("StoreToken failed: %v", err)
	}

	got, err := manager.GetOrRefreshToken(context.Background(), "test-org")
	if err != nil {
		t.Fatalf("GetOrRefreshToken failed: %v", err)
	}
	if got == nil || got.Token != token.Token {
		t.Fatalf("expected unchanged valid token")
	}
}

func TestGetOrRefreshTokenReturnsNoStoredTokenError(t *testing.T) {
	dir := t.TempDir()
	manager := NewConfigManager(filepath.Join(dir, "config.yaml"))

	got, err := manager.GetOrRefreshToken(context.Background(), "test-org")
	if got != nil {
		t.Fatalf("expected no token, got %+v", got)
	}
	if !errors.Is(err, ErrGetOrRefreshToken) {
		t.Fatalf("expected ErrGetOrRefreshToken, got %v", err)
	}
	if !errors.Is(err, ErrNoStoredToken) {
		t.Fatalf("expected ErrNoStoredToken, got %v", err)
	}
}

func TestGetOrRefreshTokenReturnsRefreshErrorForLegacyToken(t *testing.T) {
	dir := t.TempDir()
	manager := NewConfigManager(filepath.Join(dir, "config.yaml"))

	token, err := auth.ParseToken(configTestJWT(t, "https://api.example.com", time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if err := manager.StoreToken("test-org", token); err != nil {
		t.Fatalf("StoreToken failed: %v", err)
	}

	got, err := manager.GetOrRefreshToken(context.Background(), "test-org")
	if got == nil || got.Token != token.Token {
		t.Fatalf("expected original token when refresh metadata is missing")
	}
	if !errors.Is(err, ErrGetOrRefreshToken) {
		t.Fatalf("expected ErrGetOrRefreshToken, got %v", err)
	}
	if !errors.Is(err, ErrRefreshToken) {
		t.Fatalf("expected ErrRefreshToken, got %v", err)
	}
}

func TestGetOrRefreshTokenRefreshesAndPersists(t *testing.T) {
	dir := t.TempDir()
	manager := NewConfigManager(filepath.Join(dir, "config.yaml"))

	audience := "https://api.example.com"
	newJWT := configTestJWT(t, audience, time.Now().Add(time.Hour))

	// Stub the refresh function to avoid a real HTTP call and the need to
	// register a test URL in auth's internal clientIds map.
	orig := auth.RefreshAccessToken
	t.Cleanup(func() { auth.RefreshAccessToken = orig })
	auth.RefreshAccessToken = func(_ context.Context, authBaseURL, _ string) (*auth.Token, error) {
		refreshed, err := auth.ParseToken(newJWT)
		if err != nil {
			return nil, err
		}
		refreshed.AuthBaseURL = authBaseURL
		refreshed.RefreshToken = "rotated-refresh-token"
		return refreshed, nil
	}

	token, err := auth.ParseToken(configTestJWT(t, audience, time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	token.RefreshToken = "old-refresh-token"
	token.AuthBaseURL = "https://auth.getport.io"
	if err := manager.StoreToken("test-org", token); err != nil {
		t.Fatalf("StoreToken failed: %v", err)
	}

	got, err := manager.GetOrRefreshToken(context.Background(), "test-org")
	if err != nil {
		t.Fatalf("GetOrRefreshToken failed: %v", err)
	}
	if got == nil || got.Token == token.Token {
		t.Fatalf("expected refreshed access token")
	}
	if got.RefreshToken != "rotated-refresh-token" {
		t.Fatalf("expected rotated refresh token, got %q", got.RefreshToken)
	}

	stored, err := manager.GetToken("test-org")
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	if stored.Token != got.Token {
		t.Fatalf("expected refreshed token to be persisted")
	}
}

func TestGetOrRefreshTokenReturnsRefreshErrorWhenExchangeFails(t *testing.T) {
	dir := t.TempDir()
	manager := NewConfigManager(filepath.Join(dir, "config.yaml"))

	orig := auth.RefreshAccessToken
	t.Cleanup(func() { auth.RefreshAccessToken = orig })
	auth.RefreshAccessToken = func(_ context.Context, _, _ string) (*auth.Token, error) {
		return nil, errors.New("refresh failed")
	}

	token, err := auth.ParseToken(configTestJWT(t, "https://api.example.com", time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	token.RefreshToken = "old-refresh-token"
	token.AuthBaseURL = "https://auth.getport.io"
	if err := manager.StoreToken("test-org", token); err != nil {
		t.Fatalf("StoreToken failed: %v", err)
	}

	got, err := manager.GetOrRefreshToken(context.Background(), "test-org")
	if got == nil || got.Token != token.Token {
		t.Fatalf("expected original token when refresh fails")
	}
	if !errors.Is(err, ErrGetOrRefreshToken) {
		t.Fatalf("expected ErrGetOrRefreshToken, got %v", err)
	}
	if !errors.Is(err, ErrRefreshToken) {
		t.Fatalf("expected ErrRefreshToken, got %v", err)
	}
}
