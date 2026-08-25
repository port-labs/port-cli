package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/port-labs/port-cli/internal/auth"
)

// ErrOrgNotFound is returned by GetToken when credentials exist on disk but no
// entry is present for the requested org. Callers can distinguish this from
// real I/O or parse failures using errors.Is.
var ErrOrgNotFound = errors.New("org not found in credentials")

var (
	ErrGetOrRefreshToken = errors.New("failed to get new token or refresh existing one")
	ErrNoStoredToken     = errors.New("no stored oauth token")
	ErrRefreshToken      = errors.New("failed to refresh stored oauth token")
)

type orgsCreds map[string]auth.Token

func (cm *ConfigManager) StoreToken(org string, token *auth.Token) error {
	orgsContent := orgsCreds{}
	path := cm.credsPath()
	if f, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(f, &orgsContent); err != nil {
			return err
		}
	} else {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create creds directory (%w)", err)
		}
	}

	orgsContent[org] = *token

	content, err := json.Marshal(orgsContent)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0o600)
}

func (cm *ConfigManager) GetToken(org string) (*auth.Token, error) {
	orgsCreds := orgsCreds{}
	path := cm.credsPath()
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(f, &orgsCreds); err != nil {
		return nil, err
	}

	token, ok := orgsCreds[org]

	if !ok {
		return nil, fmt.Errorf("org %s: %w", org, ErrOrgNotFound)
	}

	return &token, nil
}

// GetOrRefreshToken returns the stored token for the org, silently refreshing it
// when it has expired and refresh metadata is available.
func (cm *ConfigManager) GetOrRefreshToken(ctx context.Context, org string) (*auth.Token, error) {
	token, err := cm.GetToken(org)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrOrgNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrGetOrRefreshToken, fmt.Errorf("%w: %w", ErrNoStoredToken, err))
		}
		return nil, fmt.Errorf("%w: failed reading stored credentials: %w", ErrGetOrRefreshToken, err)
	}

	if time.Now().Before(token.Claims.Expiry.Add(-5 * time.Minute)) {
		return token, nil
	}

	if token.RefreshToken == "" || token.AuthBaseURL == "" {
		return token, fmt.Errorf(
			"%w: %w",
			ErrGetOrRefreshToken,
			fmt.Errorf("%w: stored token is close to expiry but has no refresh metadata", ErrRefreshToken),
		)
	}

	refreshed, err := auth.RefreshAccessToken(ctx, token.AuthBaseURL, token.RefreshToken)
	if err != nil {
		return token, fmt.Errorf("%w: %w", ErrGetOrRefreshToken, fmt.Errorf("%w: %w", ErrRefreshToken, err))
	}

	if err := cm.StoreToken(org, refreshed); err != nil {
		return nil, fmt.Errorf("%w: failed storing refreshed credentials: %w", ErrGetOrRefreshToken, err)
	}

	return refreshed, nil
}

func ShouldIgnoreGetOrRefreshTokenError(err error) bool {
	return errors.Is(err, ErrNoStoredToken) || errors.Is(err, ErrRefreshToken)
}

func (cm *ConfigManager) DeleteToken(org string) error {
	orgsContent := orgsCreds{}
	path := cm.credsPath()
	f, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(f, &orgsContent); err != nil {
		return err
	}

	delete(orgsContent, org)
	return cm.saveToFile(path, orgsContent)
}

func (cm *ConfigManager) saveToFile(path string, content orgsCreds) error {
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func (cm *ConfigManager) credsPath() string {
	dir := filepath.Dir(cm.configPath)
	return filepath.Join(dir, "creds.json")
}
