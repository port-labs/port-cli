// Package compare provides functionality for comparing two Port organizations.
package compare

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
)

// createTestData creates test export data for testing.
func createTestData() *export.Data {
	return &export.Data{
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
}

func TestDiffResources_Added(t *testing.T) {
	source := []map[string]interface{}{}
	target := []map[string]interface{}{
		{"identifier": "new-bp", "title": "New Blueprint"},
	}

	diff := diffResources(source, target, "identifier")

	if diff.Summary.Added != 1 {
		t.Errorf("expected 1 added, got %d", diff.Summary.Added)
	}
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added item, got %d", len(diff.Added))
	}
	if diff.Added[0].Identifier != "new-bp" {
		t.Errorf("expected identifier 'new-bp', got %s", diff.Added[0].Identifier)
	}
}

func TestDiffResources_Removed(t *testing.T) {
	source := []map[string]interface{}{
		{"identifier": "old-bp", "title": "Old Blueprint"},
	}
	target := []map[string]interface{}{}

	diff := diffResources(source, target, "identifier")

	if diff.Summary.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", diff.Summary.Removed)
	}
}

func TestDiffResources_Modified(t *testing.T) {
	source := []map[string]interface{}{
		{"identifier": "bp1", "title": "Original Title"},
	}
	target := []map[string]interface{}{
		{"identifier": "bp1", "title": "Updated Title"},
	}

	diff := diffResources(source, target, "identifier")

	if diff.Summary.Modified != 1 {
		t.Errorf("expected 1 modified, got %d", diff.Summary.Modified)
	}
	if len(diff.Modified[0].FieldDiffs) == 0 {
		t.Error("expected field diffs for modified resource")
	}
}

func TestDiffResources_Identical(t *testing.T) {
	source := []map[string]interface{}{
		{"identifier": "bp1", "title": "Same Title"},
	}
	target := []map[string]interface{}{
		{"identifier": "bp1", "title": "Same Title"},
	}

	diff := diffResources(source, target, "identifier")

	if diff.Summary.Added != 0 || diff.Summary.Modified != 0 || diff.Summary.Removed != 0 {
		t.Errorf("expected no differences, got added=%d modified=%d removed=%d",
			diff.Summary.Added, diff.Summary.Modified, diff.Summary.Removed)
	}
}

func TestDiffResources_ExcludedFields(t *testing.T) {
	source := []map[string]interface{}{
		{"identifier": "bp1", "title": "Title", "createdAt": "2024-01-01", "updatedAt": "2024-01-01"},
	}
	target := []map[string]interface{}{
		{"identifier": "bp1", "title": "Title", "createdAt": "2024-06-01", "updatedAt": "2024-06-01"},
	}

	diff := diffResources(source, target, "identifier")

	// createdAt and updatedAt should be excluded, so no modification should be detected
	if diff.Summary.Modified != 0 {
		t.Errorf("expected 0 modified (excluded fields), got %d", diff.Summary.Modified)
	}
}

func TestDiffFields_NestedMaps(t *testing.T) {
	source := map[string]interface{}{
		"identifier": "bp1",
		"schema": map[string]interface{}{
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}
	target := map[string]interface{}{
		"identifier": "bp1",
		"schema": map[string]interface{}{
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "number",
				},
			},
		},
	}

	diffs := diffFields(source, target, "")

	if len(diffs) == 0 {
		t.Error("expected nested field diff")
	}

	// Should have a diff for schema.properties.name.type
	found := false
	for _, d := range diffs {
		if d.Path == "schema.properties.name.type" {
			found = true
			if d.SourceValue != "string" || d.TargetValue != "number" {
				t.Errorf("unexpected values: source=%v target=%v", d.SourceValue, d.TargetValue)
			}
			break
		}
	}
	if !found {
		t.Error("expected diff at path 'schema.properties.name.type'")
	}
}

func TestDiffFields_FieldAdded(t *testing.T) {
	source := map[string]interface{}{
		"identifier": "bp1",
	}
	target := map[string]interface{}{
		"identifier":  "bp1",
		"description": "New description",
	}

	diffs := diffFields(source, target, "")

	if len(diffs) != 1 {
		t.Errorf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Path != "description" {
		t.Errorf("expected path 'description', got %s", diffs[0].Path)
	}
	if diffs[0].SourceValue != nil {
		t.Errorf("expected nil source value, got %v", diffs[0].SourceValue)
	}
}

func TestDiffFields_FieldRemoved(t *testing.T) {
	source := map[string]interface{}{
		"identifier":  "bp1",
		"description": "Old description",
	}
	target := map[string]interface{}{
		"identifier": "bp1",
	}

	diffs := diffFields(source, target, "")

	if len(diffs) != 1 {
		t.Errorf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Path != "description" {
		t.Errorf("expected path 'description', got %s", diffs[0].Path)
	}
	if diffs[0].TargetValue != nil {
		t.Errorf("expected nil target value, got %v", diffs[0].TargetValue)
	}
}

func TestDiffer_Diff(t *testing.T) {
	source := &OrgData{
		Name: "source-org",
		Data: createTestData(),
	}
	target := &OrgData{
		Name: "target-org",
		Data: createTestData(),
	}

	differ := NewDiffer()
	result := differ.Diff(source.Data, target.Data, nil)

	if !result.Identical {
		t.Error("expected identical result for same data")
	}
}

func TestDiffer_Diff_WithDifferences(t *testing.T) {
	source := &OrgData{
		Name: "source-org",
		Data: createTestData(),
	}

	targetData := createTestData()
	// Modify a blueprint in target
	targetData.Blueprints = append(targetData.Blueprints, map[string]interface{}{
		"identifier": "new-bp",
		"title":      "New Blueprint",
	})

	target := &OrgData{
		Name: "target-org",
		Data: targetData,
	}

	differ := NewDiffer()
	result := differ.Diff(source.Data, target.Data, nil)

	if result.Identical {
		t.Error("expected non-identical result")
	}
	if result.Blueprints.Summary.Added != 1 {
		t.Errorf("expected 1 added blueprint, got %d", result.Blueprints.Summary.Added)
	}
}

func TestDiffer_Diff_IncludeFilter_PagesOnly(t *testing.T) {
	sourceData := createTestData()
	targetData := createTestData()

	// Add a new page and a new blueprint in target
	targetData.Pages = append(targetData.Pages, map[string]interface{}{
		"identifier": "page2",
		"title":      "Page 2",
	})
	targetData.Blueprints = append(targetData.Blueprints, map[string]interface{}{
		"identifier": "bp2",
		"title":      "Blueprint 2",
	})

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, []string{"pages"})

	// Pages diff should be populated
	if result.Pages.Summary.Added != 1 {
		t.Errorf("expected 1 added page, got %d", result.Pages.Summary.Added)
	}
	// Blueprint diff should be empty (not included)
	if result.Blueprints.Summary.Added != 0 {
		t.Errorf("expected 0 added blueprints when not included, got %d", result.Blueprints.Summary.Added)
	}
	// Result should not be identical because pages differ
	if result.Identical {
		t.Error("expected non-identical result when pages differ")
	}
}

func TestDiffer_Diff_IncludeFilter_EmptyMeansAll(t *testing.T) {
	sourceData := createTestData()
	targetData := createTestData()
	targetData.Blueprints = append(targetData.Blueprints, map[string]interface{}{
		"identifier": "bp2",
		"title":      "Blueprint 2",
	})

	differ := NewDiffer()
	result := differ.Diff(sourceData, targetData, []string{})

	// Empty include list should compare all resources
	if result.Blueprints.Summary.Added != 1 {
		t.Errorf("expected 1 added blueprint with empty include list, got %d", result.Blueprints.Summary.Added)
	}
}

func TestShouldInclude(t *testing.T) {
	tests := []struct {
		resource string
		include  []string
		want     bool
	}{
		{"pages", nil, true},
		{"pages", []string{}, true},
		{"pages", []string{"pages"}, true},
		{"pages", []string{"blueprints", "pages"}, true},
		{"pages", []string{"blueprints"}, false},
		{"blueprints", []string{"pages"}, false},
	}

	for _, tt := range tests {
		got := shouldInclude(tt.resource, tt.include)
		if got != tt.want {
			t.Errorf("shouldInclude(%q, %v) = %v, want %v", tt.resource, tt.include, got, tt.want)
		}
	}
}

func TestDiffResources_MultipleChanges(t *testing.T) {
	source := []map[string]interface{}{
		{"identifier": "bp1", "title": "Blueprint 1"},
		{"identifier": "bp2", "title": "Blueprint 2"},
		{"identifier": "bp3", "title": "Blueprint 3"},
	}
	target := []map[string]interface{}{
		{"identifier": "bp1", "title": "Blueprint 1 Modified"},
		{"identifier": "bp3", "title": "Blueprint 3"},
		{"identifier": "bp4", "title": "Blueprint 4"},
	}

	diff := diffResources(source, target, "identifier")

	if diff.Summary.Added != 1 {
		t.Errorf("expected 1 added (bp4), got %d", diff.Summary.Added)
	}
	if diff.Summary.Removed != 1 {
		t.Errorf("expected 1 removed (bp2), got %d", diff.Summary.Removed)
	}
	if diff.Summary.Modified != 1 {
		t.Errorf("expected 1 modified (bp1), got %d", diff.Summary.Modified)
	}
}

func TestDiffResources_SortedOutput(t *testing.T) {
	source := []map[string]interface{}{}
	target := []map[string]interface{}{
		{"identifier": "c-bp"},
		{"identifier": "a-bp"},
		{"identifier": "b-bp"},
	}

	diff := diffResources(source, target, "identifier")

	// Verify sorted order
	if diff.Added[0].Identifier != "a-bp" {
		t.Errorf("expected first added to be 'a-bp', got %s", diff.Added[0].Identifier)
	}
	if diff.Added[1].Identifier != "b-bp" {
		t.Errorf("expected second added to be 'b-bp', got %s", diff.Added[1].Identifier)
	}
	if diff.Added[2].Identifier != "c-bp" {
		t.Errorf("expected third added to be 'c-bp', got %s", diff.Added[2].Identifier)
	}
}

func TestDiffer_Diff_BlueprintPermissions(t *testing.T) {
	source := &export.Data{
		BlueprintPermissions: map[string]api.Permissions{
			"service": {"entities": map[string]interface{}{"view": []string{"$team"}}},
		},
	}
	target := &export.Data{
		BlueprintPermissions: map[string]api.Permissions{
			"service": {"entities": map[string]interface{}{"view": []string{"$admin"}}},
		},
	}
	differ := NewDiffer()
	result := differ.Diff(source, target, nil)
	if result.BlueprintPermissions.Summary.Modified != 1 {
		t.Errorf("expected 1 modified blueprint permission, got %d", result.BlueprintPermissions.Summary.Modified)
	}
}

func TestDiffer_Diff_ActionPermissions(t *testing.T) {
	source := &export.Data{
		ActionPermissions: map[string]api.Permissions{
			"deploy": {"execute": map[string]interface{}{"users": []string{}}},
		},
	}
	target := &export.Data{
		ActionPermissions: map[string]api.Permissions{
			"deploy": {"execute": map[string]interface{}{"users": []string{"alice@example.com"}}},
		},
	}
	differ := NewDiffer()
	result := differ.Diff(source, target, nil)
	if result.ActionPermissions.Summary.Modified != 1 {
		t.Errorf("expected 1 modified action permission, got %d", result.ActionPermissions.Summary.Modified)
	}
}

func TestDiffEntities_Added(t *testing.T) {
	source := []api.Entity{}
	target := []api.Entity{
		{"identifier": "svc-a", "blueprint": "service", "title": "Service A"},
	}
	d := NewDiffer()
	result := d.diffEntities(source, target)
	if result.Summary.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Summary.Added)
	}
	if result.Added[0].Identifier != "service/svc-a" {
		t.Errorf("expected composite key 'service/svc-a', got %s", result.Added[0].Identifier)
	}
}

func TestDiffEntities_Removed(t *testing.T) {
	source := []api.Entity{
		{"identifier": "svc-a", "blueprint": "service"},
	}
	target := []api.Entity{}
	d := NewDiffer()
	result := d.diffEntities(source, target)
	if result.Summary.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Summary.Removed)
	}
}

func TestDiffEntities_SameIdentifierDifferentBlueprint(t *testing.T) {
	source := []api.Entity{
		{"identifier": "prod", "blueprint": "region"},
		{"identifier": "prod", "blueprint": "environment"},
	}
	target := []api.Entity{
		{"identifier": "prod", "blueprint": "region"},
	}
	d := NewDiffer()
	result := d.diffEntities(source, target)
	if result.Summary.Removed != 1 {
		t.Errorf("expected 1 removed (environment/prod), got %d", result.Summary.Removed)
	}
	if result.Removed[0].Identifier != "environment/prod" {
		t.Errorf("expected 'environment/prod', got %s", result.Removed[0].Identifier)
	}
}

func TestDiff_EntitiesOnlyWhenIncluded(t *testing.T) {
	source := &export.Data{
		Entities: []api.Entity{{"identifier": "svc-a", "blueprint": "service"}},
	}
	target := &export.Data{
		Entities: []api.Entity{},
	}
	d := NewDiffer()

	// Without entities in include list — Entities diff should be zero
	resultWithout := d.Diff(source, target, []string{"blueprints"})
	if resultWithout.Entities.Summary.Removed != 0 {
		t.Error("entities should not be diffed when not in include list")
	}

	// With entities in include list — Entities diff should show removal
	resultWith := d.Diff(source, target, []string{"entities"})
	if resultWith.Entities.Summary.Removed != 1 {
		t.Errorf("expected 1 removed entity, got %d", resultWith.Entities.Summary.Removed)
	}
}

func TestDiff_EntitiesNotIncludedByDefault(t *testing.T) {
	source := &export.Data{
		Entities: []api.Entity{{"identifier": "svc-a", "blueprint": "service"}},
	}
	target := &export.Data{
		Entities: []api.Entity{},
	}
	d := NewDiffer()

	// Empty include list = compare all resources EXCEPT entities
	result := d.Diff(source, target, []string{})
	if result.Entities.Summary.Removed != 0 {
		t.Error("entities should not be diffed with empty include list (opt-in only)")
	}

	// nil include list — same behavior
	result2 := d.Diff(source, target, nil)
	if result2.Entities.Summary.Removed != 0 {
		t.Error("entities should not be diffed with nil include list (opt-in only)")
	}
}

func TestDiffer_Diff_PermissionsIncludeFilter(t *testing.T) {
	source := &export.Data{
		BlueprintPermissions: map[string]api.Permissions{
			"service": {"view": []string{"$team"}},
		},
		ActionPermissions: map[string]api.Permissions{
			"deploy": {"execute": []string{"$team"}},
		},
	}
	target := &export.Data{
		BlueprintPermissions: map[string]api.Permissions{
			"service": {"view": []string{"$admin"}},
		},
		ActionPermissions: map[string]api.Permissions{
			"deploy": {"execute": []string{"$admin"}},
		},
	}
	differ := NewDiffer()
	// Only include blueprint-permissions
	result := differ.Diff(source, target, []string{"blueprint-permissions"})
	if result.BlueprintPermissions.Summary.Modified != 1 {
		t.Errorf("expected 1 modified blueprint permission")
	}
	// Action permissions should be empty (not included)
	if result.ActionPermissions.Summary.Modified != 0 {
		t.Errorf("expected 0 modified action permissions when not included, got %d", result.ActionPermissions.Summary.Modified)
	}
}
