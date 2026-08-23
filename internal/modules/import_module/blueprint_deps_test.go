package import_module

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestStripDependentFields(t *testing.T) {
	bp := api.Blueprint{
		"identifier": "test",
		"title":      "Test Blueprint",
		"relations": map[string]interface{}{
			"service": map[string]interface{}{"target": "service"},
		},
		"mirrorProperties": map[string]interface{}{
			"name": map[string]interface{}{"path": "service.name"},
		},
		"calculationProperties": map[string]interface{}{
			"calc": map[string]interface{}{"calculation": "1+1"},
		},
		"aggregationProperties": map[string]interface{}{
			"count": map[string]interface{}{"target": "service"},
		},
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
	}

	stripped := StripDependentFields(bp)

	// Should keep identifier, title, properties, and relations
	// (relations must stay because the schema can't be added after creation)
	if stripped["identifier"] != "test" {
		t.Error("identifier should be preserved")
	}
	if stripped["title"] != "Test Blueprint" {
		t.Error("title should be preserved")
	}
	if stripped["properties"] == nil {
		t.Error("properties should be preserved")
	}
	if stripped["relations"] == nil {
		t.Error("relations should be preserved (schema must exist at creation)")
	}

	// Should remove dependent fields that reference other blueprints' properties
	if stripped["mirrorProperties"] != nil {
		t.Error("mirrorProperties should be stripped")
	}
	if stripped["calculationProperties"] != nil {
		t.Error("calculationProperties should be stripped")
	}
	if stripped["aggregationProperties"] != nil {
		t.Error("aggregationProperties should be stripped")
	}
}

func TestExtractDependentFields(t *testing.T) {
	bp := api.Blueprint{
		"identifier": "test",
		"relations": map[string]interface{}{
			"service": map[string]interface{}{"target": "service"},
		},
		"mirrorProperties": map[string]interface{}{
			"name": map[string]interface{}{"path": "service.name"},
		},
	}

	extracted := ExtractDependentFields(bp)

	// Relations are NOT in DependentFields anymore (must stay with blueprint)
	if extracted["relations"] != nil {
		t.Error("relations should not be extracted (stays with blueprint)")
	}
	if extracted["mirrorProperties"] == nil {
		t.Error("mirrorProperties should be extracted")
	}
	if extracted["calculationProperties"] != nil {
		t.Error("calculationProperties should not be extracted (not present)")
	}
}

func TestGetAllDependencies(t *testing.T) {
	bp := api.Blueprint{
		"identifier": "deployment",
		"relations": map[string]interface{}{
			"service": map[string]interface{}{"target": "service"},
			"cluster": map[string]interface{}{"target": "cluster"},
		},
		"mirrorProperties": map[string]interface{}{
			"serviceName": map[string]interface{}{"path": "service.name"},
		},
		"aggregationProperties": map[string]interface{}{
			"podCount": map[string]interface{}{"target": "pod"},
		},
	}

	deps := GetAllDependencies(bp)

	// Should find: service, cluster, pod (service is found twice but should be deduped)
	if len(deps) != 3 {
		t.Errorf("expected 3 dependencies, got %d: %v", len(deps), deps)
	}

	depsMap := make(map[string]bool)
	for _, d := range deps {
		depsMap[d] = true
	}

	if !depsMap["service"] {
		t.Error("should include service dependency")
	}
	if !depsMap["cluster"] {
		t.Error("should include cluster dependency")
	}
	if !depsMap["pod"] {
		t.Error("should include pod dependency")
	}
}

func TestIsSystemBlueprint(t *testing.T) {
	tests := []struct {
		id       string
		isSystem bool
	}{
		{"_user", true},
		{"_team", true},
		{"_rule", true},
		{"service", false},
		{"_", true},
		{"user", false},
		{"user_team", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if IsSystemBlueprint(tt.id) != tt.isSystem {
				t.Errorf("IsSystemBlueprint(%q) = %v, want %v", tt.id, !tt.isSystem, tt.isSystem)
			}
		})
	}
}

func TestTopologicalSort_Linear(t *testing.T) {
	// A -> B -> C (C depends on B, B depends on A)
	blueprints := []api.Blueprint{
		{"identifier": "C", "relations": map[string]interface{}{"b": map[string]interface{}{"target": "B"}}},
		{"identifier": "A"},
		{"identifier": "B", "relations": map[string]interface{}{"a": map[string]interface{}{"target": "A"}}},
	}

	levels, cyclic := TopologicalSort(blueprints, nil)

	if len(cyclic) != 0 {
		t.Errorf("expected no cyclic blueprints, got %d", len(cyclic))
	}

	if len(levels) != 3 {
		t.Errorf("expected 3 levels, got %d", len(levels))
	}

	// Level 0 should be A (no deps)
	if len(levels[0]) != 1 || levels[0][0]["identifier"] != "A" {
		t.Error("level 0 should contain only A")
	}

	// Level 1 should be B
	if len(levels[1]) != 1 || levels[1][0]["identifier"] != "B" {
		t.Error("level 1 should contain only B")
	}

	// Level 2 should be C
	if len(levels[2]) != 1 || levels[2][0]["identifier"] != "C" {
		t.Error("level 2 should contain only C")
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	blueprints := []api.Blueprint{
		{"identifier": "A"},
		{"identifier": "B", "relations": map[string]interface{}{"a": map[string]interface{}{"target": "A"}}},
		{"identifier": "C", "relations": map[string]interface{}{"a": map[string]interface{}{"target": "A"}}},
		{"identifier": "D", "relations": map[string]interface{}{
			"b": map[string]interface{}{"target": "B"},
			"c": map[string]interface{}{"target": "C"},
		}},
	}

	levels, cyclic := TopologicalSort(blueprints, nil)

	if len(cyclic) != 0 {
		t.Errorf("expected no cyclic blueprints, got %d", len(cyclic))
	}

	if len(levels) != 3 {
		t.Errorf("expected 3 levels, got %d", len(levels))
	}

	// Level 0 should be A
	if len(levels[0]) != 1 || levels[0][0]["identifier"] != "A" {
		t.Error("level 0 should contain only A")
	}

	// Level 1 should be B and C (order may vary)
	if len(levels[1]) != 2 {
		t.Error("level 1 should contain B and C")
	}

	// Level 2 should be D
	if len(levels[2]) != 1 || levels[2][0]["identifier"] != "D" {
		t.Error("level 2 should contain only D")
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	// A -> B -> A (cycle)
	blueprints := []api.Blueprint{
		{"identifier": "A", "relations": map[string]interface{}{"b": map[string]interface{}{"target": "B"}}},
		{"identifier": "B", "relations": map[string]interface{}{"a": map[string]interface{}{"target": "A"}}},
	}

	levels, cyclic := TopologicalSort(blueprints, nil)

	if len(levels) != 0 {
		t.Errorf("expected 0 levels for pure cycle, got %d", len(levels))
	}

	if len(cyclic) != 2 {
		t.Errorf("expected 2 cyclic blueprints, got %d", len(cyclic))
	}
}

func TestTopologicalSort_PartialCycle(t *testing.T) {
	// A (no deps), B <-> C (cycle)
	blueprints := []api.Blueprint{
		{"identifier": "A"},
		{"identifier": "B", "relations": map[string]interface{}{"c": map[string]interface{}{"target": "C"}}},
		{"identifier": "C", "relations": map[string]interface{}{"b": map[string]interface{}{"target": "B"}}},
	}

	levels, cyclic := TopologicalSort(blueprints, nil)

	// A should be sorted
	if len(levels) != 1 || levels[0][0]["identifier"] != "A" {
		t.Error("A should be sorted into level 0")
	}

	// B and C should be in cycle
	if len(cyclic) != 2 {
		t.Errorf("expected 2 cyclic blueprints, got %d", len(cyclic))
	}
}

func TestTopologicalSort_ExternalDependency(t *testing.T) {
	// A depends on "external" which is in existingBlueprints
	blueprints := []api.Blueprint{
		{"identifier": "A", "relations": map[string]interface{}{"ext": map[string]interface{}{"target": "external"}}},
		{"identifier": "B", "relations": map[string]interface{}{"a": map[string]interface{}{"target": "A"}}},
	}

	existing := map[string]bool{"external": true}
	levels, cyclic := TopologicalSort(blueprints, existing)

	if len(cyclic) != 0 {
		t.Errorf("expected no cyclic blueprints, got %d", len(cyclic))
	}

	// A should be in level 0 (external dependency doesn't count)
	// B should be in level 1
	if len(levels) != 2 {
		t.Errorf("expected 2 levels, got %d", len(levels))
	}
}

func TestTopologicalSortOwnership_DirectThenInherited(t *testing.T) {
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"ownership":  map[string]interface{}{"type": "Direct"},
		},
		{
			"identifier": "deployment",
			"relations": map[string]interface{}{
				"service": map[string]interface{}{"target": "service"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "service.$identifier",
			},
		},
		{
			"identifier": "pod",
			"relations": map[string]interface{}{
				"deployment": map[string]interface{}{"target": "deployment"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "deployment.$identifier",
			},
		},
	}

	levels, cyclic := TopologicalSortOwnership(blueprints)

	if len(cyclic) != 0 {
		t.Fatalf("expected no cyclic blueprints, got %d", len(cyclic))
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 ownership levels, got %d", len(levels))
	}
	if levels[0][0]["identifier"] != "service" {
		t.Fatalf("expected service in level 0, got %v", levels[0][0]["identifier"])
	}
	if levels[1][0]["identifier"] != "deployment" {
		t.Fatalf("expected deployment in level 1, got %v", levels[1][0]["identifier"])
	}
	if levels[2][0]["identifier"] != "pod" {
		t.Fatalf("expected pod in level 2, got %v", levels[2][0]["identifier"])
	}
}

func TestTopologicalSortOwnership_ExternalDependencyStartsAtLevelZero(t *testing.T) {
	blueprints := []api.Blueprint{
		{
			"identifier": "deployment",
			"relations": map[string]interface{}{
				"service": map[string]interface{}{"target": "service"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "service.$identifier",
			},
		},
	}

	levels, cyclic := TopologicalSortOwnership(blueprints)

	if len(cyclic) != 0 {
		t.Fatalf("expected no cyclic blueprints, got %d", len(cyclic))
	}
	if len(levels) != 1 || len(levels[0]) != 1 || levels[0][0]["identifier"] != "deployment" {
		t.Fatalf("expected deployment in level 0, got %+v", levels)
	}
}

func TestTopologicalSortOwnership_Cycle(t *testing.T) {
	blueprints := []api.Blueprint{
		{
			"identifier": "service",
			"relations": map[string]interface{}{
				"system": map[string]interface{}{"target": "system"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "system.$identifier",
			},
		},
		{
			"identifier": "system",
			"relations": map[string]interface{}{
				"service": map[string]interface{}{"target": "service"},
			},
			"ownership": map[string]interface{}{
				"type": "Inherited",
				"path": "service.$identifier",
			},
		},
	}

	levels, cyclic := TopologicalSortOwnership(blueprints)

	if len(levels) != 0 {
		t.Fatalf("expected no sortable ownership levels, got %d", len(levels))
	}
	if len(cyclic) != 2 {
		t.Fatalf("expected 2 cyclic blueprints, got %d", len(cyclic))
	}
}

func TestSeparateSystemBlueprints(t *testing.T) {
	blueprints := []api.Blueprint{
		{"identifier": "_user"},
		{"identifier": "service"},
		{"identifier": "_team"},
		{"identifier": "deployment"},
	}

	nonSystem, system := SeparateSystemBlueprints(blueprints)

	if len(nonSystem) != 2 {
		t.Errorf("expected 2 non-system blueprints, got %d", len(nonSystem))
	}
	if len(system) != 2 {
		t.Errorf("expected 2 system blueprints, got %d", len(system))
	}

	// Check correct separation
	for _, bp := range nonSystem {
		id := bp["identifier"].(string)
		if IsSystemBlueprint(id) {
			t.Errorf("non-system list contains system blueprint: %s", id)
		}
	}
	for _, bp := range system {
		id := bp["identifier"].(string)
		if !IsSystemBlueprint(id) {
			t.Errorf("system list contains non-system blueprint: %s", id)
		}
	}
}

func TestFlattenLevels(t *testing.T) {
	levels := [][]api.Blueprint{
		{{"identifier": "A"}, {"identifier": "B"}},
		{{"identifier": "C"}},
		{{"identifier": "D"}, {"identifier": "E"}, {"identifier": "F"}},
	}

	flat := FlattenLevels(levels)

	if len(flat) != 6 {
		t.Errorf("expected 6 blueprints, got %d", len(flat))
	}

	// Check order is preserved
	expected := []string{"A", "B", "C", "D", "E", "F"}
	for i, bp := range flat {
		if bp["identifier"] != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], bp["identifier"])
		}
	}
}

func TestTopologicalSortAggProps(t *testing.T) {
	// businessApplication.codeQualityBugs → component.codeQualityBugs
	// component.codeQualityBugs → sonarQubeProject.numberOfBugs (schema prop, not in storedAggProps)
	// Expected: component before businessApplication
	storedAggProps := map[string]map[string]interface{}{
		"businessApplication": {
			"codeQualityBugs": map[string]interface{}{
				"target": "component",
				"calculationSpec": map[string]interface{}{
					"calculationBy": "property",
					"property":      "codeQualityBugs",
				},
			},
		},
		"component": {
			"codeQualityBugs": map[string]interface{}{
				"target": "sonarQubeProject",
				"calculationSpec": map[string]interface{}{
					"calculationBy": "property",
					"property":      "numberOfBugs",
				},
			},
		},
		"snykTarget": {
			"scaCriticalOpenVulnerabilities": map[string]interface{}{
				"target": "snykProject",
				"calculationSpec": map[string]interface{}{
					"calculationBy": "property",
					"property":      "criticalOpenVulnerabilities",
				},
			},
		},
	}

	levels := TopologicalSortAggProps(storedAggProps)

	// component and snykTarget have no cross-agg-prop deps → level 1
	// businessApplication depends on component → level 2
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}

	level1 := make(map[string]bool)
	for _, id := range levels[0] {
		level1[id] = true
	}
	if !level1["component"] {
		t.Error("component should be in level 1 (no cross-agg-prop deps)")
	}
	if !level1["snykTarget"] {
		t.Error("snykTarget should be in level 1 (no cross-agg-prop deps)")
	}
	if level1["businessApplication"] {
		t.Error("businessApplication should NOT be in level 1 (depends on component)")
	}

	level2 := make(map[string]bool)
	for _, id := range levels[1] {
		level2[id] = true
	}
	if !level2["businessApplication"] {
		t.Error("businessApplication should be in level 2")
	}
}

func TestTopologicalSortAggProps_NoDeps(t *testing.T) {
	storedAggProps := map[string]map[string]interface{}{
		"a": {"count": map[string]interface{}{"target": "x", "calculationSpec": map[string]interface{}{"property": "p"}}},
		"b": {"count": map[string]interface{}{"target": "y", "calculationSpec": map[string]interface{}{"property": "q"}}},
	}
	levels := TopologicalSortAggProps(storedAggProps)
	if len(levels) != 1 {
		t.Fatalf("expected 1 level (no deps), got %d", len(levels))
	}
	if len(levels[0]) != 2 {
		t.Fatalf("expected both blueprints in level 1, got %v", levels[0])
	}
}

func TestValidateAllDependencies(t *testing.T) {
	bp := api.Blueprint{
		"identifier": "deployment",
		"relations": map[string]interface{}{
			"service": map[string]interface{}{"target": "service"},
			"cluster": map[string]interface{}{"target": "cluster"},
		},
	}

	// Only service exists
	existing := map[string]bool{"service": true}
	missing := ValidateAllDependencies(bp, existing)

	if len(missing) != 1 {
		t.Errorf("expected 1 missing dependency, got %d", len(missing))
	}
	if missing[0] != "cluster" {
		t.Errorf("expected 'cluster' to be missing, got %s", missing[0])
	}
}
