package compare

import (
	"context"
	"fmt"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/export"
	"github.com/port-labs/port-cli/internal/modules/import_module"
)

// detectInputType determines if input is an org name or file path.
func detectInputType(input string) string {
	if strings.HasSuffix(input, ".tar.gz") ||
		strings.HasSuffix(input, ".json") ||
		strings.HasPrefix(input, "/") ||
		strings.HasPrefix(input, "./") ||
		strings.HasPrefix(input, "../") {
		return "file"
	}
	return "org"
}

// Fetcher loads organization data from live orgs or export files.
type Fetcher struct {
	configManager *config.ConfigManager
}

// NewFetcher creates a new fetcher.
func NewFetcher(configManager *config.ConfigManager) *Fetcher {
	return &Fetcher{
		configManager: configManager,
	}
}

// FetchOptions contains options for fetching org data.
type FetchOptions struct {
	OrgName          string
	FilePath         string
	ClientID         string
	ClientSecret     string
	APIUrl           string
	IncludeResources []string
}

// Fetch loads organization data from either a live org or export file.
func (f *Fetcher) Fetch(ctx context.Context, opts FetchOptions) (*OrgData, error) {
	var input string
	if opts.FilePath != "" {
		input = opts.FilePath
	} else {
		input = opts.OrgName
	}

	inputType := detectInputType(input)

	if inputType == "file" {
		return f.fetchFromFile(ctx, input)
	}
	return f.fetchFromOrg(ctx, opts)
}

// fetchFromFile loads data from an export file.
func (f *Fetcher) fetchFromFile(ctx context.Context, filePath string) (*OrgData, error) {
	loader := import_module.NewLoader()
	data, err := loader.LoadData(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load export file %s: %w", filePath, err)
	}

	return &OrgData{
		Name: filePath,
		Data: data,
	}, nil
}

// fetchFromOrg loads data from a live Port organization.
func (f *Fetcher) fetchFromOrg(ctx context.Context, opts FetchOptions) (*OrgData, error) {
	// Load org config
	_, orgConfig, _, err := f.configManager.LoadWithDualOverrides(
		opts.ClientID,
		opts.ClientSecret,
		opts.APIUrl,
		opts.OrgName,
		"", "", "", "",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load config for org %s: %w", opts.OrgName, err)
	}

	if orgConfig == nil {
		return nil, fmt.Errorf("organization %s not found in config", opts.OrgName)
	}

	// Create API client
	token, err := f.configManager.GetOrRefreshToken(ctx, opts.OrgName)
	if err != nil {
		if !config.ShouldIgnoreGetOrRefreshTokenError(err) {
			return nil, err
		}
	}
	client := api.NewClient(api.ClientOpts{
		Token:        token,
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Timeout:      0,
	})
	defer client.Close()

	// Use export collector to fetch all data
	collector := export.NewCollector(client)
	includesEntities := false
	for _, r := range opts.IncludeResources {
		if r == "entities" {
			includesEntities = true
			break
		}
	}
	data, err := collector.Collect(ctx, export.Options{
		SkipEntities:       !includesEntities,
		IncludeRuleResults: true,
		IncludeResources:   opts.IncludeResources,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to collect data from org %s: %w", opts.OrgName, err)
	}

	return &OrgData{
		Name: opts.OrgName,
		Data: data,
	}, nil
}
