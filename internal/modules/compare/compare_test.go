// Package compare provides functionality for comparing two Port organizations.
package compare

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
)

// TestFullComparisonWorkflow tests the complete comparison workflow from
// data input through diffing to all output formats.
func TestFullComparisonWorkflow(t *testing.T) {
	// Create test data
	sourceData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "service", "title": "Service", "schema": map[string]interface{}{}},
			{"identifier": "old-bp", "title": "Old Blueprint"},
		},
		Actions: []api.Action{
			{"identifier": "deploy", "title": "Deploy"},
		},
	}

	targetData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "service", "title": "Service Updated", "schema": map[string]interface{}{}},
			{"identifier": "new-bp", "title": "New Blueprint"},
		},
		Actions: []api.Action{
			{"identifier": "deploy", "title": "Deploy"},
		},
	}

	// Run diff
	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)
	result.Source = "source-org"
	result.Target = "target-org"
	result.Timestamp = "2026-02-05T19:30:00Z"

	// Verify results
	if result.Identical {
		t.Error("expected differences, got identical")
	}

	if result.Blueprints.Summary.Added != 1 {
		t.Errorf("expected 1 added blueprint, got %d", result.Blueprints.Summary.Added)
	}
	if result.Blueprints.Summary.Modified != 1 {
		t.Errorf("expected 1 modified blueprint, got %d", result.Blueprints.Summary.Modified)
	}
	if result.Blueprints.Summary.Removed != 1 {
		t.Errorf("expected 1 removed blueprint, got %d", result.Blueprints.Summary.Removed)
	}

	// Test text output
	var textBuf bytes.Buffer
	textFormatter := NewTextFormatter(&textBuf, false, false, nil)
	if err := textFormatter.Format(result); err != nil {
		t.Fatalf("text formatter error: %v", err)
	}
	if textBuf.Len() == 0 {
		t.Error("expected text output, got empty")
	}

	// Test JSON output
	var jsonBuf bytes.Buffer
	jsonFormatter := NewJSONFormatter(&jsonBuf)
	if err := jsonFormatter.Format(result); err != nil {
		t.Fatalf("json formatter error: %v", err)
	}
	if jsonBuf.Len() == 0 {
		t.Error("expected JSON output, got empty")
	}

	// Test HTML output
	var htmlBuf bytes.Buffer
	htmlFormatter := NewHTMLFormatter(&htmlBuf, false)
	if err := htmlFormatter.Format(result); err != nil {
		t.Fatalf("html formatter error: %v", err)
	}
	if htmlBuf.Len() == 0 {
		t.Error("expected HTML output, got empty")
	}
}

// TestIdenticalOrganizations tests comparing two identical organizations.
func TestIdenticalOrganizations(t *testing.T) {
	data := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "service", "title": "Service"},
		},
		Actions: []api.Action{
			{"identifier": "deploy", "title": "Deploy"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "quality", "title": "Quality"},
		},
	}

	differ := NewDiffer()
	result := differ.Diff(data, data, nil)

	if !result.Identical {
		t.Error("expected identical, got differences")
	}

	// All summaries should be zero
	if result.Blueprints.Summary.Added != 0 ||
		result.Blueprints.Summary.Modified != 0 ||
		result.Blueprints.Summary.Removed != 0 {
		t.Error("expected zero blueprint changes for identical data")
	}
	if result.Actions.Summary.Added != 0 ||
		result.Actions.Summary.Modified != 0 ||
		result.Actions.Summary.Removed != 0 {
		t.Error("expected zero action changes for identical data")
	}
}

// TestEmptyOrganizations tests comparing two empty organizations.
func TestEmptyOrganizations(t *testing.T) {
	sourceData := &export.Data{}
	targetData := &export.Data{}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if !result.Identical {
		t.Error("expected identical for empty organizations")
	}
}

// TestSourceOnlyData tests when only source has data.
func TestSourceOnlyData(t *testing.T) {
	sourceData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "bp1", "title": "Blueprint 1"},
			{"identifier": "bp2", "title": "Blueprint 2"},
		},
		Actions: []api.Action{
			{"identifier": "action1", "title": "Action 1"},
		},
	}
	targetData := &export.Data{}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if result.Identical {
		t.Error("expected differences")
	}
	// Source-only items appear as removed in target
	if result.Blueprints.Summary.Removed != 2 {
		t.Errorf("expected 2 removed blueprints, got %d", result.Blueprints.Summary.Removed)
	}
	if result.Actions.Summary.Removed != 1 {
		t.Errorf("expected 1 removed action, got %d", result.Actions.Summary.Removed)
	}
}

// TestTargetOnlyData tests when only target has data.
func TestTargetOnlyData(t *testing.T) {
	sourceData := &export.Data{}
	targetData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "bp1", "title": "Blueprint 1"},
		},
		Teams: []api.Team{
			{"name": "team1", "description": "Team 1"},
		},
	}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if result.Identical {
		t.Error("expected differences")
	}
	// Target-only items appear as added
	if result.Blueprints.Summary.Added != 1 {
		t.Errorf("expected 1 added blueprint, got %d", result.Blueprints.Summary.Added)
	}
	if result.Teams.Summary.Added != 1 {
		t.Errorf("expected 1 added team, got %d", result.Teams.Summary.Added)
	}
}

// TestAllResourceTypes tests comparison across all resource types.
func TestAllResourceTypes(t *testing.T) {
	sourceData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "bp1", "title": "Blueprint 1"},
		},
		Actions: []api.Action{
			{"identifier": "action1", "title": "Action 1"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "sc1", "title": "Scorecard 1"},
		},
		Pages: []api.Page{
			{"identifier": "page1", "title": "Page 1"},
		},
		Integrations: []api.Integration{
			{"installationId": "int1", "name": "Integration 1"},
		},
		Teams: []api.Team{
			{"name": "team1", "description": "Team 1"},
		},
		Users: []api.User{
			{"email": "user@example.com", "firstName": "Test"},
		},
	}

	targetData := &export.Data{
		Blueprints: []api.Blueprint{
			{"identifier": "bp1", "title": "Blueprint 1 Modified"},
		},
		Actions: []api.Action{
			{"identifier": "action2", "title": "Action 2"},
		},
		Scorecards: []api.Scorecard{
			{"identifier": "sc1", "title": "Scorecard 1"},
		},
		Pages: []api.Page{
			{"identifier": "page2", "title": "Page 2"},
		},
		Integrations: []api.Integration{
			{"installationId": "int1", "name": "Integration 1 Updated"},
		},
		Teams: []api.Team{
			{"name": "team2", "description": "Team 2"},
		},
		Users: []api.User{
			{"email": "user@example.com", "firstName": "Test"},
		},
	}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if result.Identical {
		t.Error("expected differences")
	}

	// Blueprints: modified
	if result.Blueprints.Summary.Modified != 1 {
		t.Errorf("expected 1 modified blueprint, got %d", result.Blueprints.Summary.Modified)
	}

	// Actions: 1 removed, 1 added
	if result.Actions.Summary.Added != 1 || result.Actions.Summary.Removed != 1 {
		t.Errorf("expected 1 added and 1 removed action, got added=%d removed=%d",
			result.Actions.Summary.Added, result.Actions.Summary.Removed)
	}

	// Scorecards: identical
	if result.Scorecards.Summary.Added != 0 ||
		result.Scorecards.Summary.Modified != 0 ||
		result.Scorecards.Summary.Removed != 0 {
		t.Error("expected scorecards to be identical")
	}

	// Pages: 1 removed, 1 added
	if result.Pages.Summary.Added != 1 || result.Pages.Summary.Removed != 1 {
		t.Errorf("expected 1 added and 1 removed page, got added=%d removed=%d",
			result.Pages.Summary.Added, result.Pages.Summary.Removed)
	}

	// Integrations: modified
	if result.Integrations.Summary.Modified != 1 {
		t.Errorf("expected 1 modified integration, got %d", result.Integrations.Summary.Modified)
	}

	// Teams: 1 removed, 1 added
	if result.Teams.Summary.Added != 1 || result.Teams.Summary.Removed != 1 {
		t.Errorf("expected 1 added and 1 removed team, got added=%d removed=%d",
			result.Teams.Summary.Added, result.Teams.Summary.Removed)
	}

	// Users: identical
	if result.Users.Summary.Added != 0 ||
		result.Users.Summary.Modified != 0 ||
		result.Users.Summary.Removed != 0 {
		t.Error("expected users to be identical")
	}
}

// TestOutputConsistency tests that all formatters produce consistent output.
func TestOutputConsistency(t *testing.T) {
	result := &CompareResult{
		Source:    "source-org",
		Target:    "target-org",
		Timestamp: "2026-02-05T19:30:00Z",
		Identical: false,
		Blueprints: ResourceDiff{
			Summary: DiffSummary{Added: 2, Modified: 1, Removed: 1},
			Added: []ResourceChange{
				{Identifier: "new-bp-1"},
				{Identifier: "new-bp-2"},
			},
			Modified: []ResourceChange{
				{
					Identifier: "mod-bp",
					FieldDiffs: []FieldDiff{
						{Path: "title", SourceValue: "Old", TargetValue: "New"},
					},
				},
			},
			Removed: []ResourceChange{
				{Identifier: "old-bp"},
			},
		},
	}

	// Text output
	var textBuf bytes.Buffer
	textFormatter := NewTextFormatter(&textBuf, true, true, nil)
	if err := textFormatter.Format(result); err != nil {
		t.Fatalf("text formatter error: %v", err)
	}
	textOutput := textBuf.String()

	// JSON output
	var jsonBuf bytes.Buffer
	jsonFormatter := NewJSONFormatter(&jsonBuf)
	if err := jsonFormatter.Format(result); err != nil {
		t.Fatalf("json formatter error: %v", err)
	}

	// HTML output
	var htmlBuf bytes.Buffer
	htmlFormatter := NewHTMLFormatter(&htmlBuf, false)
	if err := htmlFormatter.Format(result); err != nil {
		t.Fatalf("html formatter error: %v", err)
	}
	htmlOutput := htmlBuf.String()

	// Verify text contains key information
	if !strings.Contains(textOutput, "source-org") {
		t.Error("text output missing source org name")
	}
	if !strings.Contains(textOutput, "target-org") {
		t.Error("text output missing target org name")
	}
	if !strings.Contains(textOutput, "2 added") {
		t.Error("text output missing '2 added'")
	}
	if !strings.Contains(textOutput, "new-bp-1") {
		t.Error("verbose text output missing added identifier")
	}

	// Verify JSON is valid and contains key data
	var parsed JSONOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON output is invalid: %v", err)
	}
	if parsed.Source != "source-org" {
		t.Errorf("JSON source mismatch: got %s", parsed.Source)
	}
	if parsed.Target != "target-org" {
		t.Errorf("JSON target mismatch: got %s", parsed.Target)
	}
	if parsed.Summary.TotalAdded != 2 {
		t.Errorf("JSON TotalAdded mismatch: expected 2, got %d", parsed.Summary.TotalAdded)
	}

	// Verify HTML contains key information
	if !strings.Contains(htmlOutput, "<!DOCTYPE html>") {
		t.Error("HTML output missing doctype")
	}
	if !strings.Contains(htmlOutput, "source-org") {
		t.Error("HTML output missing source org name")
	}
	if !strings.Contains(htmlOutput, "target-org") {
		t.Error("HTML output missing target org name")
	}
}

// TestFieldLevelDiffs tests detailed field-level difference detection.
func TestFieldLevelDiffs(t *testing.T) {
	sourceData := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier":  "bp1",
				"title":       "Original Title",
				"description": "Original Description",
				"schema": map[string]interface{}{
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
	}

	targetData := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier":  "bp1",
				"title":       "Updated Title",
				"description": "Original Description",
				"schema": map[string]interface{}{
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "number",
						},
					},
				},
			},
		},
	}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if result.Blueprints.Summary.Modified != 1 {
		t.Fatalf("expected 1 modified blueprint, got %d", result.Blueprints.Summary.Modified)
	}

	modified := result.Blueprints.Modified[0]
	if modified.Identifier != "bp1" {
		t.Errorf("expected modified identifier 'bp1', got %s", modified.Identifier)
	}

	// Should have field diffs for title and nested schema property
	if len(modified.FieldDiffs) < 2 {
		t.Errorf("expected at least 2 field diffs, got %d", len(modified.FieldDiffs))
	}

	// Check for title diff
	foundTitle := false
	foundSchema := false
	for _, fd := range modified.FieldDiffs {
		if fd.Path == "title" {
			foundTitle = true
			if fd.SourceValue != "Original Title" || fd.TargetValue != "Updated Title" {
				t.Errorf("title diff values incorrect: source=%v target=%v",
					fd.SourceValue, fd.TargetValue)
			}
		}
		if strings.Contains(fd.Path, "schema.properties.name.type") {
			foundSchema = true
			if fd.SourceValue != "string" || fd.TargetValue != "number" {
				t.Errorf("schema diff values incorrect: source=%v target=%v",
					fd.SourceValue, fd.TargetValue)
			}
		}
	}

	if !foundTitle {
		t.Error("missing title field diff")
	}
	if !foundSchema {
		t.Error("missing schema field diff")
	}
}

// TestExcludedFields tests that certain fields are excluded from comparison.
func TestExcludedFields(t *testing.T) {
	sourceData := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "bp1",
				"title":      "Title",
				"createdAt":  "2024-01-01T00:00:00Z",
				"updatedAt":  "2024-01-01T00:00:00Z",
				"createdBy":  "user1",
				"updatedBy":  "user1",
			},
		},
	}

	targetData := &export.Data{
		Blueprints: []api.Blueprint{
			{
				"identifier": "bp1",
				"title":      "Title",
				"createdAt":  "2024-06-01T00:00:00Z",
				"updatedAt":  "2024-06-01T00:00:00Z",
				"createdBy":  "user2",
				"updatedBy":  "user2",
			},
		},
	}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	// Should be identical since only excluded fields differ
	if !result.Identical {
		t.Error("expected identical result when only excluded fields differ")
	}
	if result.Blueprints.Summary.Modified != 0 {
		t.Errorf("expected 0 modified blueprints, got %d", result.Blueprints.Summary.Modified)
	}
}

// TestLargeDataset tests comparison with larger datasets.
func TestLargeDataset(t *testing.T) {
	// Create source with 100 blueprints
	var sourceBlueprints []api.Blueprint
	for i := 0; i < 100; i++ {
		sourceBlueprints = append(sourceBlueprints, api.Blueprint{
			"identifier": string(rune('a'+i/26)) + string(rune('a'+i%26)),
			"title":      "Blueprint " + string(rune('A'+i/26)) + string(rune('A'+i%26)),
		})
	}

	// Create target with 100 blueprints, 50 same, 25 modified, 25 new (25 removed from source)
	var targetBlueprints []api.Blueprint
	for i := 0; i < 50; i++ {
		// Same as source
		targetBlueprints = append(targetBlueprints, api.Blueprint{
			"identifier": string(rune('a'+i/26)) + string(rune('a'+i%26)),
			"title":      "Blueprint " + string(rune('A'+i/26)) + string(rune('A'+i%26)),
		})
	}
	for i := 50; i < 75; i++ {
		// Modified
		targetBlueprints = append(targetBlueprints, api.Blueprint{
			"identifier": string(rune('a'+i/26)) + string(rune('a'+i%26)),
			"title":      "Modified Blueprint " + string(rune('A'+i/26)) + string(rune('A'+i%26)),
		})
	}
	for i := 100; i < 125; i++ {
		// New
		targetBlueprints = append(targetBlueprints, api.Blueprint{
			"identifier": "new-" + string(rune('a'+i/26)) + string(rune('a'+i%26)),
			"title":      "New Blueprint " + string(rune('A'+i/26)) + string(rune('A'+i%26)),
		})
	}

	sourceData := &export.Data{Blueprints: sourceBlueprints}
	targetData := &export.Data{Blueprints: targetBlueprints}

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, nil)

	if result.Identical {
		t.Error("expected differences")
	}

	// 25 removed (indices 75-99 from source)
	if result.Blueprints.Summary.Removed != 25 {
		t.Errorf("expected 25 removed, got %d", result.Blueprints.Summary.Removed)
	}

	// 25 modified
	if result.Blueprints.Summary.Modified != 25 {
		t.Errorf("expected 25 modified, got %d", result.Blueprints.Summary.Modified)
	}

	// 25 added
	if result.Blueprints.Summary.Added != 25 {
		t.Errorf("expected 25 added, got %d", result.Blueprints.Summary.Added)
	}
}

// TestHTMLReportModes tests both simple and interactive HTML modes.
func TestHTMLReportModes(t *testing.T) {
	result := &CompareResult{
		Source:    "staging",
		Target:    "production",
		Timestamp: "2026-02-05T19:30:00Z",
		Identical: false,
		Blueprints: ResourceDiff{
			Summary: DiffSummary{Added: 1, Modified: 1, Removed: 1},
			Added:   []ResourceChange{{Identifier: "new-bp"}},
			Modified: []ResourceChange{{
				Identifier: "mod-bp",
				FieldDiffs: []FieldDiff{{Path: "title", SourceValue: "Old", TargetValue: "New"}},
			}},
			Removed: []ResourceChange{{Identifier: "old-bp"}},
		},
	}

	// Test interactive mode
	var interactiveBuf bytes.Buffer
	interactiveFormatter := NewHTMLFormatter(&interactiveBuf, false)
	if err := interactiveFormatter.Format(result); err != nil {
		t.Fatalf("interactive HTML formatter error: %v", err)
	}
	interactiveOutput := interactiveBuf.String()

	// Test simple mode
	var simpleBuf bytes.Buffer
	simpleFormatter := NewHTMLFormatter(&simpleBuf, true)
	if err := simpleFormatter.Format(result); err != nil {
		t.Fatalf("simple HTML formatter error: %v", err)
	}
	simpleOutput := simpleBuf.String()

	// Both should be valid HTML
	if !strings.Contains(interactiveOutput, "<!DOCTYPE html>") {
		t.Error("interactive HTML missing doctype")
	}
	if !strings.Contains(simpleOutput, "<!DOCTYPE html>") {
		t.Error("simple HTML missing doctype")
	}

	// Interactive mode should have more content (JavaScript for interactivity)
	if len(interactiveOutput) <= len(simpleOutput) {
		t.Error("expected interactive HTML to be larger than simple HTML")
	}
}

// TestTextOutputModes tests text output in summary, verbose, and full modes.
func TestTextOutputModes(t *testing.T) {
	result := &CompareResult{
		Source:    "staging",
		Target:    "production",
		Timestamp: "2026-02-05T19:30:00Z",
		Identical: false,
		Blueprints: ResourceDiff{
			Summary: DiffSummary{Added: 1, Modified: 1, Removed: 1},
			Added:   []ResourceChange{{Identifier: "new-bp", TargetData: map[string]interface{}{"title": "New"}}},
			Modified: []ResourceChange{{
				Identifier: "mod-bp",
				SourceData: map[string]interface{}{"title": "Old Title"},
				TargetData: map[string]interface{}{"title": "New Title"},
				FieldDiffs: []FieldDiff{{Path: "title", SourceValue: "Old Title", TargetValue: "New Title"}},
			}},
			Removed: []ResourceChange{{Identifier: "old-bp", SourceData: map[string]interface{}{"title": "Old"}}},
		},
	}

	// Summary mode (default)
	var summaryBuf bytes.Buffer
	summaryFormatter := NewTextFormatter(&summaryBuf, false, false, nil)
	if err := summaryFormatter.Format(result); err != nil {
		t.Fatalf("summary formatter error: %v", err)
	}
	summaryOutput := summaryBuf.String()

	// Verbose mode
	var verboseBuf bytes.Buffer
	verboseFormatter := NewTextFormatter(&verboseBuf, true, false, nil)
	if err := verboseFormatter.Format(result); err != nil {
		t.Fatalf("verbose formatter error: %v", err)
	}
	verboseOutput := verboseBuf.String()

	// Full mode
	var fullBuf bytes.Buffer
	fullFormatter := NewTextFormatter(&fullBuf, false, true, nil)
	if err := fullFormatter.Format(result); err != nil {
		t.Fatalf("full formatter error: %v", err)
	}
	fullOutput := fullBuf.String()

	// Summary should have count but not identifiers
	if !strings.Contains(summaryOutput, "1 added") {
		t.Error("summary missing count")
	}

	// Verbose should include identifiers
	if !strings.Contains(verboseOutput, "new-bp") {
		t.Error("verbose missing identifier")
	}

	// Full should include field values
	if !strings.Contains(fullOutput, "Old Title") || !strings.Contains(fullOutput, "New Title") {
		t.Error("full mode missing field values")
	}

	// Full mode should be the longest
	if len(fullOutput) <= len(verboseOutput) || len(verboseOutput) <= len(summaryOutput) {
		t.Error("expected full > verbose > summary in output length")
	}
}

// TestJSONOutputStructure tests the complete JSON output structure.
func TestJSONOutputStructure(t *testing.T) {
	result := &CompareResult{
		Source:    "source-org",
		Target:    "target-org",
		Timestamp: "2026-02-05T19:30:00Z",
		Identical: false,
		Blueprints: ResourceDiff{
			Summary: DiffSummary{Added: 1, Modified: 1, Removed: 1},
			Added:   []ResourceChange{{Identifier: "new-bp", TargetData: map[string]interface{}{"title": "New"}}},
			Modified: []ResourceChange{{
				Identifier: "mod-bp",
				SourceData: map[string]interface{}{"title": "Old"},
				TargetData: map[string]interface{}{"title": "New"},
				FieldDiffs: []FieldDiff{{Path: "title", SourceValue: "Old", TargetValue: "New"}},
			}},
			Removed: []ResourceChange{{Identifier: "old-bp", SourceData: map[string]interface{}{"title": "Old"}}},
		},
		Actions: ResourceDiff{
			Summary: DiffSummary{Added: 2},
			Added: []ResourceChange{
				{Identifier: "action1"},
				{Identifier: "action2"},
			},
		},
	}

	var buf bytes.Buffer
	formatter := NewJSONFormatter(&buf)
	if err := formatter.Format(result); err != nil {
		t.Fatalf("JSON formatter error: %v", err)
	}

	var parsed JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	// Check top-level fields
	if parsed.Source != "source-org" {
		t.Errorf("source mismatch: got %s", parsed.Source)
	}
	if parsed.Target != "target-org" {
		t.Errorf("target mismatch: got %s", parsed.Target)
	}
	if parsed.Timestamp != "2026-02-05T19:30:00Z" {
		t.Errorf("timestamp mismatch: got %s", parsed.Timestamp)
	}
	if parsed.Identical {
		t.Error("expected identical=false")
	}

	// Check summary totals (1+2 added, 1 modified, 1 removed)
	if parsed.Summary.TotalAdded != 3 {
		t.Errorf("expected TotalAdded=3, got %d", parsed.Summary.TotalAdded)
	}
	if parsed.Summary.TotalModified != 1 {
		t.Errorf("expected TotalModified=1, got %d", parsed.Summary.TotalModified)
	}
	if parsed.Summary.TotalRemoved != 1 {
		t.Errorf("expected TotalRemoved=1, got %d", parsed.Summary.TotalRemoved)
	}

	// Check blueprints diff structure
	bps, ok := parsed.Diffs["blueprints"]
	if !ok {
		t.Fatal("missing blueprints in diffs")
	}
	if len(bps.Added) != 1 {
		t.Errorf("expected 1 added blueprint, got %d", len(bps.Added))
	}
	if len(bps.Modified) != 1 {
		t.Errorf("expected 1 modified blueprint, got %d", len(bps.Modified))
	}
	if len(bps.Removed) != 1 {
		t.Errorf("expected 1 removed blueprint, got %d", len(bps.Removed))
	}

	// Check modified blueprint has changes
	if len(bps.Modified[0].Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(bps.Modified[0].Changes))
	}
	if bps.Modified[0].Changes[0].Path != "title" {
		t.Errorf("expected change path 'title', got %s", bps.Modified[0].Changes[0].Path)
	}
}
