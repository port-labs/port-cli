package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// jsonHookWriter handles the hooks.json format used by Cursor and OpenAI Codex
// ({ "command": "..." } session hook entries).
type jsonHookWriter struct{}

// hooksJSON models the hooks.json file structure.
// The Hooks map uses string keys (event names like "sessionStart") and
// each value is a slice of sessionHookEntry. Unknown event keys and their
// values are preserved during round-trips via the raw map.
type hooksJSON struct {
	Version int                    `json:"version"`
	Hooks   map[string]interface{} `json:"hooks"`
}

// sessionHookEntry represents a single hook entry with a command string.
type sessionHookEntry struct {
	Command string `json:"command"`
}

func (jsonHookWriter) Write(dir, command string) error {
	jsonPath := filepath.Join(dir, "hooks.json")
	existing := &hooksJSON{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, existing)
	}
	if existing.Hooks == nil {
		existing.Hooks = make(map[string]interface{})
	}
	existing.Version = 1
	existing.Hooks["sessionStart"] = []sessionHookEntry{
		{Command: command},
	}

	data, err := json.MarshalIndent(existing, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks.json: %w", err)
	}
	return os.WriteFile(jsonPath, data, 0o644)
}

func (jsonHookWriter) Remove(dir string) (bool, error) {
	jsonPath := filepath.Join(dir, "hooks.json")

	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", jsonPath, err)
	}

	existing := &hooksJSON{}
	if err := json.Unmarshal(data, existing); err != nil || existing.Hooks == nil {
		return false, nil
	}

	entries, _ := existing.Hooks["sessionStart"].([]interface{})
	kept := filterCommands(entries)

	changed := len(kept) != len(entries)
	if len(kept) == 0 {
		delete(existing.Hooks, "sessionStart")
	} else {
		existing.Hooks["sessionStart"] = kept
	}

	if len(existing.Hooks) == 0 {
		_ = os.Remove(jsonPath)
		return changed, nil
	}

	return changed, writeJSONFile(jsonPath, existing)
}

// copilotJSONHookWriter writes hooks.json using the schema expected by GitHub
// Copilot agents (type/command + bash + powershell). See:
// https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-hooks
type copilotJSONHookWriter struct{}

// copilotSessionHookTimeoutSec is the max time allowed for port skills sync
// (network) before Copilot treats the hook as failed.
const copilotSessionHookTimeoutSec = 120

func copilotPortHookEntry(command string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "command",
		"bash":       command,
		"powershell": command,
		"cwd":        ".",
		"timeoutSec": copilotSessionHookTimeoutSec,
	}
}

// isPortHookJSONMap reports whether m is a Port-installed hook entry, either
// GitHub Copilot shape ({type, bash, ...}) or legacy Cursor-style ({command}).
func isPortHookJSONMap(m map[string]interface{}) bool {
	if cmd, ok := m["command"].(string); ok {
		return isPortCommand(cmd)
	}
	if typ, _ := m["type"].(string); typ == "command" {
		for _, key := range []string{"bash", "powershell"} {
			if s, ok := m[key].(string); ok && isPortCommand(strings.TrimSpace(s)) {
				return true
			}
		}
	}
	return false
}

func filterSessionStartPortHooks(entries []interface{}) []interface{} {
	if len(entries) == 0 {
		return nil
	}
	kept := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]interface{})
		if !ok {
			kept = append(kept, entry)
			continue
		}
		if isPortHookJSONMap(m) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func (copilotJSONHookWriter) Write(dir, command string) error {
	jsonPath := filepath.Join(dir, "hooks.json")
	existing := &hooksJSON{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, existing)
	}
	if existing.Hooks == nil {
		existing.Hooks = make(map[string]interface{})
	}
	existing.Version = 1

	var ss []interface{}
	if raw, ok := existing.Hooks["sessionStart"]; ok {
		ss, _ = raw.([]interface{})
	}
	ss = filterSessionStartPortHooks(ss)
	ss = append(ss, copilotPortHookEntry(command))
	existing.Hooks["sessionStart"] = ss

	data, err := json.MarshalIndent(existing, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks.json: %w", err)
	}
	return os.WriteFile(jsonPath, data, 0o644)
}

func (copilotJSONHookWriter) Remove(dir string) (bool, error) {
	jsonPath := filepath.Join(dir, "hooks.json")

	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", jsonPath, err)
	}

	existing := &hooksJSON{}
	if err := json.Unmarshal(data, existing); err != nil || existing.Hooks == nil {
		return false, nil
	}

	entries, _ := existing.Hooks["sessionStart"].([]interface{})
	kept := filterSessionStartPortHooks(entries)

	changed := len(kept) != len(entries)
	if len(kept) == 0 {
		delete(existing.Hooks, "sessionStart")
	} else {
		existing.Hooks["sessionStart"] = kept
	}

	if len(existing.Hooks) == 0 {
		_ = os.Remove(jsonPath)
		return changed, nil
	}

	return changed, writeJSONFile(jsonPath, existing)
}
