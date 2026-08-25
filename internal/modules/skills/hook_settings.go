package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// settingsHookWriter handles the settings.json format used by both Claude Code
// and Gemini CLI. The two formats differ in structure:
//
//   - Claude: top-level "hooks" is an array of {matcher, hooks:[{type,command}]}
//   - Gemini: top-level "hooks" is a map with event keys (e.g. "SessionStart")
//     each holding an array of {hooks:[{type,command}]}
//
// topLevelArray=true selects the Claude layout; false selects the Gemini layout.
type settingsHookWriter struct {
	eventKey      string
	topLevelArray bool
}

// settingsHookEntry represents a single hook action within a settings entry.
type settingsHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func (w settingsHookWriter) Write(dir, command string) error {
	path := filepath.Join(dir, "settings.json")

	raw, err := readJSONFileMap(path)
	if err != nil {
		return err
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}

	portHook := w.buildPortHook(command)

	if w.topLevelArray {
		w.mergeArrayLayout(raw, portHook)
	} else {
		w.mergeMapLayout(raw, portHook)
	}

	return writeJSONFile(path, raw)
}

func (w settingsHookWriter) Remove(dir string) (bool, error) {
	path := filepath.Join(dir, "settings.json")

	raw, err := readJSONFileMap(path)
	if raw == nil {
		return false, err
	}

	var changed bool
	if w.topLevelArray {
		changed = w.removeFromArrayLayout(raw)
	} else {
		changed = w.removeFromMapLayout(raw)
	}

	if len(raw) == 0 {
		_ = os.Remove(path)
		return changed, nil
	}

	return changed, writeJSONFile(path, raw)
}

func (w settingsHookWriter) buildPortHook(command string) map[string]interface{} {
	inner := []settingsHookEntry{
		{Type: "command", Command: command},
	}
	if w.topLevelArray {
		return map[string]interface{}{
			"matcher": w.eventKey,
			"hooks":   inner,
		}
	}
	return map[string]interface{}{"hooks": inner}
}

// mergeArrayLayout handles Claude's format: hooks is a top-level array.
// Preserves unrecognised entries; replaces only the entry matching eventKey.
func (w settingsHookWriter) mergeArrayLayout(raw map[string]interface{}, portHook map[string]interface{}) {
	existing, _ := raw["hooks"].([]interface{})
	merged := make([]interface{}, 0, len(existing)+1)
	for _, entry := range existing {
		m, ok := entry.(map[string]interface{})
		if !ok {
			merged = append(merged, entry)
			continue
		}
		if m["matcher"] == w.eventKey {
			continue
		}
		merged = append(merged, entry)
	}
	merged = append(merged, portHook)
	raw["hooks"] = merged
}

// mergeMapLayout handles the map-keyed-by-event format used by Claude and Gemini.
func (w settingsHookWriter) mergeMapLayout(raw map[string]interface{}, portHook map[string]interface{}) {
	hooksMap, ok := raw["hooks"].(map[string]interface{})
	if !ok {
		hooksMap = migrateArrayHooksToMap(raw["hooks"])
	}
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	entries, _ := hooksMap[w.eventKey].([]interface{})
	merged := filterNestedHookEntries(entries)
	merged = append(merged, portHook)
	hooksMap[w.eventKey] = merged
	raw["hooks"] = hooksMap
}

func (w settingsHookWriter) removeFromArrayLayout(raw map[string]interface{}) bool {
	hooks, _ := raw["hooks"].([]interface{})
	kept := make([]interface{}, 0, len(hooks))
	for _, entry := range hooks {
		m, ok := entry.(map[string]interface{})
		if !ok {
			kept = append(kept, entry)
			continue
		}
		if m["matcher"] != w.eventKey || !nestedHooksContainPort(m) {
			kept = append(kept, entry)
		}
	}
	changed := len(kept) != len(hooks)
	if len(kept) == 0 {
		delete(raw, "hooks")
	} else {
		raw["hooks"] = kept
	}
	return changed
}

func (w settingsHookWriter) removeFromMapLayout(raw map[string]interface{}) bool {
	hooksMap, ok := raw["hooks"].(map[string]interface{})
	if !ok {
		if raw["hooks"] == nil {
			return false
		}
		hooksMap = migrateArrayHooksToMap(raw["hooks"])
		raw["hooks"] = hooksMap
	}
	if len(hooksMap) == 0 {
		return false
	}

	entries, _ := hooksMap[w.eventKey].([]interface{})
	kept := filterNestedHookEntries(entries)
	changed := len(kept) != len(entries)

	if len(kept) == 0 {
		delete(hooksMap, w.eventKey)
	} else {
		hooksMap[w.eventKey] = kept
	}

	if len(hooksMap) == 0 {
		delete(raw, "hooks")
	}
	return changed
}

// filterNestedHookEntries removes entries whose nested hooks contain the Port command.
func filterNestedHookEntries(entries []interface{}) []interface{} {
	kept := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]interface{})
		if !ok {
			kept = append(kept, entry)
			continue
		}
		if !nestedHooksContainPort(m) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func nestedHooksContainPort(m map[string]interface{}) bool {
	innerHooks, _ := m["hooks"].([]interface{})
	for _, ih := range innerHooks {
		if h, ok := ih.(map[string]interface{}); ok {
			if cmd, _ := h["command"].(string); isPortCommand(cmd) {
				return true
			}
		}
	}
	return false
}

// migrateArrayHooksToMap converts the legacy Claude array-format hooks field to
// the current map format. Each entry {"matcher": X, "hooks": [...]} becomes
// {X: [{"hooks": [...]}]}. This handles files written by older versions of the
// CLI that incorrectly used an array for the top-level hooks field.
func migrateArrayHooksToMap(v interface{}) map[string]interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{})
	for _, entry := range arr {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := m["matcher"].(string)
		if key == "" {
			continue
		}
		newEntry := map[string]interface{}{}
		if innerHooks := m["hooks"]; innerHooks != nil {
			newEntry["hooks"] = innerHooks
		}
		existing, _ := result[key].([]interface{})
		result[key] = append(existing, newEntry)
	}
	return result
}

// readJSONFileMap loads a JSON object from disk. Returns (nil, nil) when the
// file does not exist, and (nil, err) on other I/O or parse errors.
func readJSONFileMap(path string) (map[string]interface{}, error) {
	raw, err := readJSONFile(path)
	if raw == nil {
		return nil, err
	}

	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected JSON object in %s", path)
	}
	return m, nil
}
