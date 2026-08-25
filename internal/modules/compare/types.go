// Package compare provides functionality for comparing two Port organizations.
package compare

import (
	"github.com/port-labs/port-cli/internal/modules/export"
)

// Options represents compare options.
type Options struct {
	SourceOrg        string   // Source org name from config
	TargetOrg        string   // Target org name from config
	SourceFile       string   // Source export file path (alternative to org)
	TargetFile       string   // Target export file path (alternative to org)
	SourceClientID   string   // Override source client ID
	SourceSecret     string   // Override source client secret
	TargetClientID   string   // Override target client ID
	TargetSecret     string   // Override target client secret
	OutputFormat     string   // text, json, html
	HTMLFile         string   // Output path for HTML report
	HTMLSimple       bool     // Use simple HTML template
	Verbose          bool     // Show identifiers
	Full             bool     // Show full diff
	IncludeResources []string // Filter resource types
	FailOnDiff       bool     // Exit 1 if differences found
}

// DiffSummary represents the summary of differences for a resource type.
type DiffSummary struct {
	Added    int
	Modified int
	Removed  int
}

// ResourceDiff represents differences for a single resource type.
type ResourceDiff struct {
	Summary  DiffSummary
	Added    []ResourceChange
	Modified []ResourceChange
	Removed  []ResourceChange
}

// ResourceChange represents a single changed resource.
type ResourceChange struct {
	Identifier string
	SourceData map[string]interface{}
	TargetData map[string]interface{}
	FieldDiffs []FieldDiff
}

// FieldDiff represents a single field-level difference.
type FieldDiff struct {
	Path        string
	SourceValue interface{}
	TargetValue interface{}
}

// CompareResult represents the full comparison result.
type CompareResult struct {
	Source               string
	Target               string
	Timestamp            string
	Identical            bool
	Blueprints           ResourceDiff
	Actions              ResourceDiff
	Scorecards           ResourceDiff
	Pages                ResourceDiff
	Integrations         ResourceDiff
	Teams                ResourceDiff
	Users                ResourceDiff
	Automations          ResourceDiff
	BlueprintPermissions ResourceDiff
	ActionPermissions    ResourceDiff
	Entities             ResourceDiff
}

// OrgData wraps export.Data for comparison.
type OrgData struct {
	Name string
	Data *export.Data
}
