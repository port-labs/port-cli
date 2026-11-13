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
//
// Refresh is serialized with a cross-process file lock so concurrent CLI
// processes sharing the same credentials file do not race Auth0 rotating
// refresh tokens (which would revoke siblings with invalid_grant).
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

	var refreshed *auth.Token
	lockErr := withFileLock(cm.credsLockPath(), func() error {
		// Another process may have refreshed while we waited for the lock.
		latest, getErr := cm.GetToken(org)
		if getErr != nil {
			if errors.Is(getErr, os.ErrNotExist) || errors.Is(getErr, ErrOrgNotFound) {
				return fmt.Errorf("%w: %w", ErrNoStoredToken, getErr)
			}
			return fmt.Errorf("failed reading stored credentials: %w", getErr)
		}
		if time.Now().Before(latest.Claims.Expiry.Add(-5 * time.Minute)) {
			refreshed = latest
			return nil
		}
		if latest.RefreshToken == "" || latest.AuthBaseURL == "" {
			refreshed = latest
			return fmt.Errorf("%w: stored token is close to expiry but has no refresh metadata", ErrRefreshToken)
		}

		next, refreshErr := auth.RefreshAccessToken(ctx, latest.AuthBaseURL, latest.RefreshToken)
		if refreshErr != nil {
			if errors.Is(refreshErr, auth.ErrInvalidGrant) {
				_ = cm.DeleteToken(org)
				// Do not wrap ErrRefreshToken: callers ignore that for client-cred
				// fallback, but invalid_grant means the OAuth session is dead.
				return fmt.Errorf(
					"refresh token was rejected by Auth0 (invalid_grant); run `%s` to re-authenticate: %w",
					CmdAuthLogin,
					refreshErr,
				)
			}
			refreshed = latest
			return fmt.Errorf("%w: %w", ErrRefreshToken, refreshErr)
		}

		if storeErr := cm.StoreToken(org, next); storeErr != nil {
			return fmt.Errorf("failed storing refreshed credentials: %w", storeErr)
		}
		refreshed = next
		return nil
	})
	if lockErr != nil {
		if refreshed != nil {
			return refreshed, fmt.Errorf("%w: %w", ErrGetOrRefreshToken, lockErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrGetOrRefreshToken, lockErr)
	}

	return refreshed, nil
}

// ShouldIgnoreGetOrRefreshTokenError reports whether callers may fall back to
// client credentials (or a still-returned near-expiry token) after GetOrRefreshToken.
// invalid_grant must not be ignored: the OAuth refresh family is revoked and
// the user must re-authenticate.
func ShouldIgnoreGetOrRefreshTokenError(err error) bool {
	if errors.Is(err, auth.ErrInvalidGrant) {
		return false
	}
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

func (cm *ConfigManager) credsLockPath() string {
	return cm.credsPath() + ".lock"
}
