// Package compare provides functionality for comparing two Port organizations.
package compare

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/port-labs/port-cli/internal/config"
)

// Module handles organization comparison operations.
type Module struct {
	configManager *config.ConfigManager
}

// NewModule creates a new compare module.
func NewModule(configManager *config.ConfigManager) *Module {
	return &Module{
		configManager: configManager,
	}
}

// Execute runs the comparison and returns results.
func (m *Module) Execute(ctx context.Context, opts Options) (*CompareResult, error) {
	fetcher := NewFetcher(m.configManager)

	// Fetch source data
	sourceOpts := FetchOptions{
		OrgName:          opts.SourceOrg,
		FilePath:         opts.SourceFile,
		ClientID:         opts.SourceClientID,
		ClientSecret:     opts.SourceSecret,
		IncludeResources: opts.IncludeResources,
	}

	sourceData, err := fetcher.Fetch(ctx, sourceOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch source data: %w", err)
	}

	// Fetch target data
	targetOpts := FetchOptions{
		OrgName:          opts.TargetOrg,
		FilePath:         opts.TargetFile,
		ClientID:         opts.TargetClientID,
		ClientSecret:     opts.TargetSecret,
		IncludeResources: opts.IncludeResources,
	}

	targetData, err := fetcher.Fetch(ctx, targetOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch target data: %w", err)
	}

	// Compute differences
	differ := NewDiffer()
	result := differ.Diff(sourceData.Data, targetData.Data, opts.IncludeResources)
	result.Source = sourceData.Name
	result.Target = targetData.Name
	result.Timestamp = time.Now().UTC().Format(time.RFC3339)

	return result, nil
}

// FormatOutput formats the result based on options.
func (m *Module) FormatOutput(result *CompareResult, opts Options) error {
	var w io.Writer = os.Stdout

	switch opts.OutputFormat {
	case "json":
		formatter := NewJSONFormatter(w)
		return formatter.Format(result)

	case "html":
		// Write to file for HTML
		filePath := opts.HTMLFile
		if filePath == "" {
			filePath = "comparison-report.html"
		}
		file, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create HTML file: %w", err)
		}
		defer file.Close()

		formatter := NewHTMLFormatter(file, opts.HTMLSimple)
		if err := formatter.Format(result); err != nil {
			return err
		}
		fmt.Printf("HTML report written to %s\n", filePath)
		return nil

	default: // text
		formatter := NewTextFormatter(w, opts.Verbose, opts.Full, opts.IncludeResources)
		return formatter.Format(result)
	}
}
