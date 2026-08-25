package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/port-labs/port-cli/internal/useragent"
)

const (
	cacheFile = ".port-cli-update-cache"
	cacheTTL  = 24 * time.Hour
)

var releasesURL = "https://api.github.com/repos/port-labs/port-cli/releases/latest"

// CheckResult represents the result of an update check.
type CheckResult struct {
	LatestVersion   string
	CurrentVersion  string
	UpdateAvailable bool
	DownloadURL     string
	Error           error
}

// Checker checks for updates.
type Checker struct {
	httpClient *http.Client
}

// NewChecker creates a new update checker.
func NewChecker() *Checker {
	return &Checker{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CheckLatestVersion checks for the latest version on GitHub.
func (c *Checker) CheckLatestVersion(ctx context.Context, currentVersion string) (*CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", releasesURL, nil)
	if err != nil {
		return &CheckResult{
			CurrentVersion: currentVersion,
			Error:          err,
		}, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", useragent.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &CheckResult{
			CurrentVersion: currentVersion,
			Error:          err,
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &CheckResult{
			CurrentVersion: currentVersion,
			Error:          fmt.Errorf("failed to fetch releases: %s", resp.Status),
		}, fmt.Errorf("failed to fetch releases: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &CheckResult{
			CurrentVersion: currentVersion,
			Error:          err,
		}, err
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}

	if err := json.Unmarshal(body, &release); err != nil {
		return &CheckResult{
			CurrentVersion: currentVersion,
			Error:          err,
		}, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	updateAvailable := compareVersions(currentVersion, latestVersion) < 0

	result := &CheckResult{
		LatestVersion:   latestVersion,
		CurrentVersion:  currentVersion,
		UpdateAvailable: updateAvailable,
		DownloadURL:     release.HTMLURL,
	}

	return result, nil
}

// compareVersions compares two version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 string) int {
	// Simple version comparison - remove 'v' prefix and compare
	v1 = strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2 = strings.TrimPrefix(strings.TrimSpace(v2), "v")

	// Handle dev versions
	if v1 == "dev" {
		return -1
	}
	if v2 == "dev" {
		return 1
	}

	// Simple string comparison for semantic versions
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}

	return 0
}
