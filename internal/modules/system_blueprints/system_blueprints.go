package systemblueprints

import (
	"reflect"
	"sort"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
)

var managedFields = map[string]map[string]map[string]bool{
	"_rule_result": {
		"properties":            setOf("entity", "result", "result_last_change"),
		"relations":             setOf("rule"),
		"mirrorProperties":      setOf("blueprint", "level", "scorecard"),
		"calculationProperties": setOf("entity_link"),
	},
	"_scorecard": {
		"properties":            setOf("blueprint", "filter", "levels"),
		"aggregationProperties": setOf("rules_passed", "rules_tested"),
		"calculationProperties": setOf("of_rules_passed"),
	},
	"_rule": {
		"properties":            setOf("level", "query", "rule_description"),
		"relations":             setOf("scorecard"),
		"aggregationProperties": setOf("entities_passed", "entities_tested"),
		"calculationProperties": setOf("of_entities_passed"),
	},
	"_ai_invocations": {
		"properties":       setOf("asked_at", "context_usage_percent", "error", "execution_logs", "feedback_comment", "feedback_rating", "labels", "model", "prompt", "provider", "quota", "replied_at", "response", "source", "status", "summary"),
		"relations":        setOf("agent", "asked_by", "conversation", "parent"),
		"mirrorProperties": setOf("agent_title"),
	},
	"_ai_agent": {
		"properties":            setOf("conversation_starters", "description", "execution_mode", "labels", "model", "prompt", "provider", "status", "tools"),
		"relations":             setOf("mcp_servers"),
		"aggregationProperties": setOf("total_invocations"),
	},
	"_team": {
		"properties":            setOf("description"),
		"aggregationProperties": setOf("size"),
	},
	"_user": {
		"properties": setOf("moderated_blueprints", "port_role", "port_type", "status"),
	},
	"_ai_conversation": {
		"properties":       setOf("asked_at", "pinned", "source", "status"),
		"relations":        setOf("asked_by", "latest_invocation"),
		"mirrorProperties": setOf("latest_invocation_status"),
	},
	"_mcp_server": {
		"properties": setOf("allowed_tools", "description", "exposed", "headers", "oauth_config", "url"),
	},
}

var customSections = []string{
	"properties",
	"relations",
	"mirrorProperties",
	"calculationProperties",
	"aggregationProperties",
	"ownership",
}

var managedOwnershipBlueprints = map[string]bool{
	"_rule_result": true,
	"_user":        true,
}

type relationSkipRule struct {
	Type string
}

var managedRelationRules = map[string][]relationSkipRule{
	"_rule_result": {
		{Type: "rule_result_target"},
	},
}

var patchUpdateBlueprints = map[string]bool{
	"_rule_result": true,
}

func setOf(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// IsKnown returns true for system blueprints whose Port-managed fields are
// known well enough to preserve only custom schema additions.
func IsKnown(identifier string) bool {
	_, ok := managedFields[identifier]
	return ok
}

// CustomPatch returns a minimal blueprint containing only custom schema
// additions for known system blueprints.
func CustomPatch(bp api.Blueprint) api.Blueprint {
	id, _ := bp["identifier"].(string)
	if !IsKnown(id) {
		return nil
	}

	patch := api.Blueprint{"identifier": id}
	for _, section := range customSections {
		if section == "ownership" && managedOwnershipBlueprints[id] {
			continue
		}
		sectionMap, ok := bp[section].(map[string]interface{})
		if !ok || len(sectionMap) == 0 {
			continue
		}
		custom := customSectionFields(id, section, sectionMap)
		if section == "relations" {
			custom, _ = FilterManagedRelations(id, custom)
		}
		if len(custom) > 0 {
			patch[section] = custom
		}
	}

	if len(patch) == 1 {
		return nil
	}
	return patch
}

func customSectionFields(blueprintID, section string, values map[string]interface{}) map[string]interface{} {
	managed := managedFields[blueprintID][section]
	custom := make(map[string]interface{})
	for key, value := range values {
		if managed[key] {
			continue
		}
		custom[key] = value
	}
	return custom
}

// FilterManagedRelations removes relation definitions that are owned by Port
// for a known system blueprint. The ignored relation keys are sorted for
// stable CLI output and tests.
func FilterManagedRelations(blueprintID string, relations map[string]interface{}) (kept map[string]interface{}, ignoredKeys []string) {
	if len(relations) == 0 {
		return nil, nil
	}

	rules := managedRelationRules[blueprintID]
	if len(rules) == 0 {
		return relations, nil
	}

	kept = make(map[string]interface{}, len(relations))
	for key, relation := range relations {
		if shouldSkipRelation(relation, rules) {
			ignoredKeys = append(ignoredKeys, key)
			continue
		}
		kept[key] = relation
	}
	sort.Strings(ignoredKeys)
	if len(kept) == 0 {
		return nil, ignoredKeys
	}
	return kept, ignoredKeys
}

func shouldSkipRelation(relation interface{}, rules []relationSkipRule) bool {
	relationMap, ok := relation.(map[string]interface{})
	if !ok {
		return false
	}
	relationType, _ := relationMap["type"].(string)
	for _, rule := range rules {
		if rule.Type != "" && relationType == rule.Type {
			return true
		}
	}
	return false
}

// BlueprintWithRelations returns a shallow copy of bp with relations replaced
// by kept, or with relations removed when kept is nil or empty.
func BlueprintWithRelations(bp api.Blueprint, kept map[string]interface{}) api.Blueprint {
	out := make(api.Blueprint, len(bp))
	for k, v := range bp {
		out[k] = v
	}
	if len(kept) > 0 {
		out["relations"] = kept
	} else {
		delete(out, "relations")
	}
	return out
}

// PrefersPatchUpdate returns true when updates for this blueprint should be
// sent as partial patches instead of full replacement-style updates.
func PrefersPatchUpdate(identifier string) bool {
	return patchUpdateBlueprints[identifier]
}

// IsCustomPatch returns true when a blueprint is a minimal patch for a known
// system blueprint rather than a full blueprint schema.
func IsCustomPatch(bp api.Blueprint) bool {
	id, _ := bp["identifier"].(string)
	if !IsKnown(id) {
		return false
	}
	if len(bp) <= 1 {
		return false
	}
	sectionSet := make(map[string]bool, len(customSections))
	for _, section := range customSections {
		sectionSet[section] = true
	}
	for key := range bp {
		if key == "identifier" {
			continue
		}
		if !sectionSet[key] {
			return false
		}
	}
	return true
}

// CustomPatchEqual compares a minimal custom system blueprint patch with a
// full current blueprint by checking only fields present in the patch.
func CustomPatchEqual(patch, current api.Blueprint) bool {
	for _, section := range customSections {
		patchMap, ok := patch[section].(map[string]interface{})
		if !ok {
			continue
		}
		currentMap, _ := current[section].(map[string]interface{})
		for key, patchValue := range patchMap {
			if !reflect.DeepEqual(patchValue, currentMap[key]) {
				return false
			}
		}
	}
	return true
}

// ApplyExclusions filters blueprints for iteration and output. When
// skipSystemBlueprints is true and skipSystemBlueprintProperties is false,
// known system blueprint schemas are replaced in dataList by minimal custom
// property patches.
func ApplyExclusions(
	all []api.Blueprint,
	excludeDeep []string,
	excludeSchema []string,
	skipSystemBlueprints bool,
	skipSystemBlueprintProperties bool,
) (iterList, dataList []api.Blueprint) {
	deepSet := make(map[string]bool, len(excludeDeep))
	for _, id := range excludeDeep {
		deepSet[id] = true
	}
	schemaSet := make(map[string]bool, len(excludeSchema))
	for _, id := range excludeSchema {
		schemaSet[id] = true
	}

	for _, bp := range all {
		id, _ := bp["identifier"].(string)
		if deepSet[id] {
			continue
		}
		iterList = append(iterList, bp)
		if schemaSet[id] {
			continue
		}
		if skipSystemBlueprints && strings.HasPrefix(id, "_") {
			if skipSystemBlueprintProperties {
				continue
			}
			patch := CustomPatch(bp)
			if patch != nil {
				dataList = append(dataList, patch)
			}
			continue
		}
		dataList = append(dataList, bp)
	}

	return iterList, dataList
}
