package systemblueprints

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestCustomPatch_StripsManagedFieldsAndKeepsCustomFields(t *testing.T) {
	patch := CustomPatch(api.Blueprint{
		"identifier": "_user",
		"properties": map[string]interface{}{
			"status":        map[string]interface{}{"type": "string"},
			"department":    map[string]interface{}{"type": "string"},
			"custom_number": map[string]interface{}{"type": "number"},
		},
		"relations": map[string]interface{}{
			"manager": map[string]interface{}{"target": "_user"},
		},
		"ownership": map[string]interface{}{
			"type": "Direct",
		},
	})

	if patch == nil {
		t.Fatal("expected custom system blueprint patch")
	}
	props := patch["properties"].(map[string]interface{})
	if _, ok := props["status"]; ok {
		t.Fatal("managed _user property status should be stripped")
	}
	if _, ok := props["department"]; !ok {
		t.Fatal("custom _user property department should be preserved")
	}
	if _, ok := props["custom_number"]; !ok {
		t.Fatal("custom _user property custom_number should be preserved")
	}
	if _, ok := patch["relations"].(map[string]interface{})["manager"]; !ok {
		t.Fatal("custom relation should be preserved")
	}
	if _, ok := patch["ownership"]; ok {
		t.Fatal("_user ownership should be treated as managed and omitted")
	}
}

func TestCustomPatch_PreservesOwnershipExceptManagedDefaults(t *testing.T) {
	teamPatch := CustomPatch(api.Blueprint{
		"identifier": "_team",
		"properties": map[string]interface{}{
			"cost_center": map[string]interface{}{"type": "string"},
		},
		"ownership": map[string]interface{}{
			"type": "Direct",
		},
	})
	if teamPatch == nil {
		t.Fatal("expected _team patch")
	}
	if _, ok := teamPatch["ownership"].(map[string]interface{})["type"]; !ok {
		t.Fatal("_team ownership should be preserved")
	}

	ruleResultPatch := CustomPatch(api.Blueprint{
		"identifier": "_rule_result",
		"relations": map[string]interface{}{
			"custom_target": map[string]interface{}{"target": "service"},
		},
		"ownership": map[string]interface{}{
			"type": "Direct",
		},
	})
	if ruleResultPatch == nil {
		t.Fatal("expected _rule_result patch")
	}
	if _, ok := ruleResultPatch["ownership"]; ok {
		t.Fatal("_rule_result ownership should be treated as managed and omitted")
	}
}

func TestCustomPatch_StripsManagedRuleResultRelationsByRule(t *testing.T) {
	patch := CustomPatch(api.Blueprint{
		"identifier": "_rule_result",
		"relations": map[string]interface{}{
			"rule": map[string]interface{}{"target": "_rule"},
			"_githubBranch": map[string]interface{}{
				"type": "rule_result_target", "target": "githubBranch",
			},
			"custom_target": map[string]interface{}{"target": "service"},
		},
	})

	if patch == nil {
		t.Fatal("expected _rule_result patch")
	}
	relations := patch["relations"].(map[string]interface{})
	if _, ok := relations["rule"]; ok {
		t.Fatal("managed _rule_result relation rule should be stripped")
	}
	if _, ok := relations["_githubBranch"]; ok {
		t.Fatal("rule_result_target relation should be stripped")
	}
	if _, ok := relations["custom_target"]; !ok {
		t.Fatal("custom _rule_result relation should be preserved")
	}
}

func TestFilterManagedRelations_RuleResultTarget(t *testing.T) {
	rels := map[string]interface{}{
		"rule": map[string]interface{}{"target": "_rule", "title": "Rule"},
		"_githubBranch": map[string]interface{}{
			"type": "rule_result_target", "target": "githubBranch",
		},
		"plain": map[string]interface{}{"target": "service"},
	}

	kept, ignored := FilterManagedRelations("_rule_result", rels)
	if len(ignored) != 1 || ignored[0] != "_githubBranch" {
		t.Fatalf("ignored: %v", ignored)
	}
	if len(kept) != 2 || kept["rule"] == nil || kept["plain"] == nil {
		t.Fatalf("kept: %#v", kept)
	}
}

func TestFilterManagedRelations_NoRulesReturnsOriginalRelations(t *testing.T) {
	rels := map[string]interface{}{
		"plain": map[string]interface{}{"target": "service"},
	}

	kept, ignored := FilterManagedRelations("service", rels)
	if len(ignored) != 0 {
		t.Fatalf("ignored: %v", ignored)
	}
	if len(kept) != 1 || kept["plain"] == nil {
		t.Fatalf("kept: %#v", kept)
	}
}

func TestCustomPatch_OmitsEmptyAndUnknownSystemBlueprints(t *testing.T) {
	if patch := CustomPatch(api.Blueprint{
		"identifier": "_team",
		"properties": map[string]interface{}{
			"description": map[string]interface{}{"type": "string"},
		},
	}); patch != nil {
		t.Fatalf("expected empty managed-only patch to be omitted, got %#v", patch)
	}

	if patch := CustomPatch(api.Blueprint{
		"identifier": "_unknown",
		"properties": map[string]interface{}{
			"custom": map[string]interface{}{"type": "string"},
		},
	}); patch != nil {
		t.Fatalf("expected unknown system blueprint patch to be omitted, got %#v", patch)
	}
}

func TestApplyExclusions(t *testing.T) {
	all := []api.Blueprint{
		{
			"identifier": "_user",
			"properties": map[string]interface{}{
				"status":     map[string]interface{}{"type": "string"},
				"department": map[string]interface{}{"type": "string"},
			},
		},
		{
			"identifier": "_unknown",
			"properties": map[string]interface{}{
				"custom": map[string]interface{}{"type": "string"},
			},
		},
		{"identifier": "service"},
	}

	iter, data := ApplyExclusions(all, nil, nil, true, false)
	if len(iter) != 3 {
		t.Fatalf("expected all non-deep-excluded blueprints in iter list, got %d", len(iter))
	}
	if len(data) != 2 {
		t.Fatalf("expected service and _user patch in data list, got %#v", data)
	}
	if data[0]["identifier"] != "_user" {
		t.Fatalf("expected _user patch first, got %#v", data[0]["identifier"])
	}
	if _, ok := data[0]["properties"].(map[string]interface{})["department"]; !ok {
		t.Fatalf("expected custom department property in _user patch: %#v", data[0])
	}
	if data[1]["identifier"] != "service" {
		t.Fatalf("expected service blueprint second, got %#v", data[1]["identifier"])
	}

	_, skippedData := ApplyExclusions(all, nil, nil, true, true)
	if len(skippedData) != 1 || skippedData[0]["identifier"] != "service" {
		t.Fatalf("expected only service when system blueprint properties are skipped, got %#v", skippedData)
	}
}

func TestCustomPatchEqual(t *testing.T) {
	patch := api.Blueprint{
		"identifier": "_rule",
		"properties": map[string]interface{}{
			"custom": map[string]interface{}{"type": "string"},
		},
	}
	current := api.Blueprint{
		"identifier": "_rule",
		"title":      "Rule",
		"properties": map[string]interface{}{
			"level":  map[string]interface{}{"type": "string"},
			"custom": map[string]interface{}{"type": "string"},
		},
	}
	if !CustomPatchEqual(patch, current) {
		t.Fatal("expected patch to equal matching custom fields on current blueprint")
	}

	current["properties"].(map[string]interface{})["custom"] = map[string]interface{}{"type": "number"}
	if CustomPatchEqual(patch, current) {
		t.Fatal("expected patch to differ when custom field differs")
	}
}
