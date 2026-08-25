package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// windsurfHookWriter handles the .codeium/windsurf/hooks.json format.
type windsurfHookWriter struct{}

// windsurfHooksFile models the known structure of Windsurf's hooks.json.
// Unknown top-level keys are preserved via Extras.
type windsurfHooksFile struct {
	Hooks  windsurfHooks          `json:"hooks"`
	Extras map[string]interface{} `json:"-"`
}

type windsurfHooks struct {
	PreUserPrompt []hookEntry            `json:"pre_user_prompt,omitempty"`
	Extras        map[string]interface{} `json:"-"`
}

type hookEntry struct {
	Command string                 `json:"command"`
	Extras  map[string]interface{} `json:"-"`
}

func (windsurfHookWriter) Write(dir, command string) error {
	jsonPath := filepath.Join(dir, "hooks.json")

	wf := readWindsurfFile(jsonPath)

	filtered := filterHookEntries(wf.Hooks.PreUserPrompt)
	filtered = append(filtered, hookEntry{Command: command})
	wf.Hooks.PreUserPrompt = filtered

	return writeWindsurfFile(jsonPath, wf)
}

func (windsurfHookWriter) Remove(dir string) (bool, error) {
	jsonPath := filepath.Join(dir, "hooks.json")

	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	wf := parseWindsurfFile(data)
	original := len(wf.Hooks.PreUserPrompt)
	wf.Hooks.PreUserPrompt = filterHookEntries(wf.Hooks.PreUserPrompt)
	changed := len(wf.Hooks.PreUserPrompt) != original

	if isWindsurfFileEmpty(wf) {
		_ = os.Remove(jsonPath)
		return changed, nil
	}

	return changed, writeWindsurfFile(jsonPath, wf)
}

func filterHookEntries(entries []hookEntry) []hookEntry {
	kept := make([]hookEntry, 0, len(entries))
	for _, e := range entries {
		if !isPortCommand(e.Command) {
			kept = append(kept, e)
		}
	}
	return kept
}

func isWindsurfFileEmpty(wf *windsurfHooksFile) bool {
	return len(wf.Hooks.PreUserPrompt) == 0 && len(wf.Hooks.Extras) == 0 && len(wf.Extras) == 0
}

func readWindsurfFile(path string) *windsurfHooksFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return &windsurfHooksFile{}
	}
	return parseWindsurfFile(data)
}

func parseWindsurfFile(data []byte) *windsurfHooksFile {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return &windsurfHooksFile{}
	}

	wf := &windsurfHooksFile{Extras: make(map[string]interface{})}

	if hooksRaw, ok := raw["hooks"]; ok {
		var hooksMap map[string]json.RawMessage
		if err := json.Unmarshal(hooksRaw, &hooksMap); err == nil {
			wf.Hooks.Extras = make(map[string]interface{})
			if pupRaw, ok := hooksMap["pre_user_prompt"]; ok {
				var entries []hookEntry
				if err := json.Unmarshal(pupRaw, &entries); err == nil {
					wf.Hooks.PreUserPrompt = entries
				}
			}
			for k, v := range hooksMap {
				if k == "pre_user_prompt" {
					continue
				}
				var val interface{}
				_ = json.Unmarshal(v, &val)
				wf.Hooks.Extras[k] = val
			}
		}
	}

	for k, v := range raw {
		if k == "hooks" {
			continue
		}
		var val interface{}
		_ = json.Unmarshal(v, &val)
		wf.Extras[k] = val
	}

	return wf
}

func writeWindsurfFile(path string, wf *windsurfHooksFile) error {
	out := make(map[string]interface{})
	for k, v := range wf.Extras {
		out[k] = v
	}

	hooksOut := make(map[string]interface{})
	for k, v := range wf.Hooks.Extras {
		hooksOut[k] = v
	}
	if len(wf.Hooks.PreUserPrompt) > 0 {
		hooksOut["pre_user_prompt"] = wf.Hooks.PreUserPrompt
	}
	if len(hooksOut) > 0 {
		out["hooks"] = hooksOut
	}

	return writeJSONFile(path, out)
}
