package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveHooks_RemovesHookPerFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   hookFormat
		subdir   string
		hookFile string
	}{
		{name: "JSON", format: hookFormatJSON, subdir: "tooldir", hookFile: "hooks.json"},
		{name: "Claude", format: hookFormatClaude, subdir: "claudedir", hookFile: "settings.json"},
		{name: "Gemini", format: hookFormatGemini, subdir: "geminidir", hookFile: "settings.json"},
		{name: "Windsurf", format: hookFormatWindsurf, subdir: "wsdir", hookFile: "hooks.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			targets := []HookTarget{{Name: "Test", Dir: tt.subdir, Format: tt.format}}
			if err := InstallHooks(targets, dir, dir, ""); err != nil {
				t.Fatalf("InstallHooks: %v", err)
			}
			result, err := RemoveHooks(targets, dir, dir, nil)
			if err != nil {
				t.Fatalf("RemoveHooks: %v", err)
			}
			if len(result.RemovedFrom) != 1 {
				t.Errorf("expected 1 removal, got %d", len(result.RemovedFrom))
			}
			assertFileAbsent(t, filepath.Join(dir, tt.subdir, tt.hookFile))
		})
	}
}

func TestRemoveHooks_PreservesOtherJSONHooks(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tooldir")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := []HookTarget{{Name: "Tool", Dir: "tooldir", Format: hookFormatJSON}}
	if err := InstallHooks(targets, dir, dir, ""); err != nil {
		t.Fatal(err)
	}

	hookFile := filepath.Join(toolDir, "hooks.json")
	raw := map[string]interface{}{}
	data, _ := os.ReadFile(hookFile)
	_ = json.Unmarshal(data, &raw)
	hooks := raw["hooks"].(map[string]interface{})
	hooks["preCommit"] = []map[string]string{{"command": "./lint.sh"}}
	hooks["sessionStart"] = append(hooks["sessionStart"].([]interface{}), map[string]string{"command": "./other.sh"})
	out, _ := json.Marshal(raw)
	_ = os.WriteFile(hookFile, out, 0o644)

	if _, err := RemoveHooks(targets, dir, dir, nil); err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	body, _ := os.ReadFile(hookFile)
	if !containsStr(string(body), "preCommit") {
		t.Error("preCommit hook should be preserved")
	}
	if !containsStr(string(body), "other.sh") {
		t.Error("other sessionStart entry should be preserved")
	}
	if containsStr(string(body), hookCommand) {
		t.Error("Port hook command should be removed")
	}
}

func TestRemoveHooks_PreservesOtherClaudeHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claudedir")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"hooks": [{"matcher": "PreToolUse", "hooks": [{"type": "command", "command": "./lint.sh"}]}]}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := []HookTarget{{Name: "Claude", Dir: "claudedir", Format: hookFormatClaude}}
	if err := InstallHooks(targets, dir, dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveHooks(targets, dir, dir, nil); err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	body := string(data)
	if !containsStr(body, "PreToolUse") {
		t.Error("PreToolUse hook should be preserved")
	}
	if containsStr(body, hookCommand) {
		t.Error("Port hook command should be removed")
	}
}

func TestRemoveHooks_SkipsWhenNoHookFile(t *testing.T) {
	dir := t.TempDir()
	targets := []HookTarget{{Name: "Tool", Dir: "nonexistent", Format: hookFormatJSON}}
	result, err := RemoveHooks(targets, dir, dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(result.Skipped))
	}
}

func TestRemoveHooks_CopilotFindsHooksViaSavedTargetsNotCwd(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()
	wrongCwd := t.TempDir()

	skillRoot := filepath.Join(repoDir, ".github")
	if err := os.MkdirAll(filepath.Join(skillRoot, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyJSON := `{"version":1,"hooks":{"sessionStart":[{"command":"` + hookCommand + `"}]}}`
	hookPath := filepath.Join(skillRoot, "hooks", "hooks.json")
	if err := os.WriteFile(hookPath, []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	copilot := HookTarget{
		Name: "GitHub Copilot", Dir: ".github", RepoScoped: true, HookSubDir: "hooks",
		Format: hookFormatCopilotJSON, LegacyHookDirs: []string{".copilot"},
	}
	result, err := RemoveHooks([]HookTarget{copilot}, homeDir, wrongCwd, []string{skillRoot})
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	wantHookDir := filepath.Join(skillRoot, "hooks")
	found := false
	for _, p := range result.RemovedFrom {
		if p == wantHookDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected removal from saved repo hooks dir %q, RemovedFrom=%v", wantHookDir, result.RemovedFrom)
	}
	assertFileAbsent(t, hookPath)
}

func TestRemoveHooks_GitHubCopilotAlsoCleansLegacyDotCopilot(t *testing.T) {
	homeDir, repoDir := t.TempDir(), t.TempDir()
	legacyDir := filepath.Join(homeDir, ".copilot")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyJSON := `{"version":1,"hooks":{"sessionStart":[{"command":"` + hookCommand + `"}]}}`
	if err := os.WriteFile(filepath.Join(legacyDir, "hooks.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	copilot := HookTarget{
		Name: "GitHub Copilot", Dir: ".github", RepoScoped: true, HookSubDir: "hooks",
		Format: hookFormatCopilotJSON, LegacyHookDirs: []string{".copilot"},
	}
	if err := InstallHooks([]HookTarget{copilot}, homeDir, repoDir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	result, err := RemoveHooks([]HookTarget{copilot}, homeDir, repoDir, nil)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if len(result.RemovedFrom) < 2 {
		t.Fatalf("expected removals from primary + legacy dirs, got %v", result.RemovedFrom)
	}
	assertFileAbsent(t, filepath.Join(legacyDir, "hooks.json"))
	assertFileAbsent(t, filepath.Join(repoDir, ".github", "hooks", "hooks.json"))
}
