package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// isPortCommand
// ---------------------------------------------------------------------------

func TestIsPortCommand_CurrentCommand(t *testing.T) {
	if !isPortCommand(hookCommand) {
		t.Errorf("current hookCommand %q should be recognised", hookCommand)
	}
}

func TestIsPortCommand_LegacyCommands(t *testing.T) {
	for _, cmd := range legacyHookCommands {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			if !isPortCommand(cmd) {
				t.Errorf("legacy command %q should be recognised by isPortCommand", cmd)
			}
		})
	}
}

func TestIsPortCommand_UnrelatedCommands(t *testing.T) {
	unrelated := []string{
		"",
		"echo hello",
		"./lint.sh",
		"port",
		"port sync",
		"port skills",
	}
	for _, cmd := range unrelated {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			if isPortCommand(cmd) {
				t.Errorf("unrelated command %q should NOT be recognised by isPortCommand", cmd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RemoveHooks removes legacy "port plugin sync" entries
// ---------------------------------------------------------------------------

func TestRemoveHooks_RemovesLegacyPluginSync_JSON(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tooldir")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a hooks.json that contains the old "port plugin sync" command.
	legacy := `{
		"version": 1,
		"hooks": {
			"sessionStart": [
				{"command": "port plugin sync"},
				{"command": "./other.sh"}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(toolDir, "hooks.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := []HookTarget{{Name: "Tool", Dir: "tooldir", Format: hookFormatJSON}}
	result, err := RemoveHooks(targets, dir, dir, nil)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if len(result.RemovedFrom) != 1 {
		t.Errorf("expected 1 removal, got %d", len(result.RemovedFrom))
	}

	data, _ := os.ReadFile(filepath.Join(toolDir, "hooks.json"))
	body := string(data)
	if containsStr(body, "port plugin sync") {
		t.Error("legacy 'port plugin sync' should have been removed")
	}
	if !containsStr(body, "other.sh") {
		t.Error("unrelated 'other.sh' hook should be preserved")
	}
}

func TestRemoveHooks_RemovesLegacyPluginSync_Claude(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claudedir")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a settings.json with the old "port plugin sync" command and an
	// unrelated hook that must survive removal.
	legacy := `{
		"hooks": {
			"UserPromptSubmit": [
				{
					"hooks": [{"type": "command", "command": "port plugin sync"}]
				},
				{
					"hooks": [{"type": "command", "command": "./other.sh"}]
				}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := []HookTarget{{Name: "Claude", Dir: "claudedir", Format: hookFormatClaude}}
	result, err := RemoveHooks(targets, dir, dir, nil)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if len(result.RemovedFrom) != 1 {
		t.Errorf("expected 1 removal, got %d", len(result.RemovedFrom))
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	body := string(data)
	if containsStr(body, "port plugin sync") {
		t.Error("legacy 'port plugin sync' should have been removed")
	}
	if !containsStr(body, "other.sh") {
		t.Error("unrelated 'other.sh' hook should be preserved")
	}
}

func TestInstallHooks_ReplacesLegacyPluginSync_JSON(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tooldir")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate with the legacy command.
	legacy := `{"version":1,"hooks":{"sessionStart":[{"command":"port plugin sync"}]}}`
	if err := os.WriteFile(filepath.Join(toolDir, "hooks.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := []HookTarget{{Name: "Tool", Dir: "tooldir", Format: hookFormatJSON}}
	if err := InstallHooks(targets, dir, dir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(toolDir, "hooks.json"))
	body := string(data)
	if containsStr(body, "port plugin sync") {
		t.Error("legacy 'port plugin sync' should have been replaced")
	}
	if !containsStr(body, hookCommand) {
		t.Errorf("current hookCommand %q should be present after install", hookCommand)
	}
}

func TestInstallHooks_ReplacesLegacySkillsSync_Claude(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "claudedir")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate with the old "port skills sync" (without --quiet).
	legacy := `{
		"hooks": {
			"UserPromptSubmit": [
				{"hooks": [{"type": "command", "command": "port skills sync"}]}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := []HookTarget{{Name: "Claude", Dir: "claudedir", Format: hookFormatClaude}}
	if err := InstallHooks(targets, dir, dir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	body := string(data)
	if containsStr(body, `"port skills sync"`) {
		t.Error("old 'port skills sync' (without --quiet) should have been replaced")
	}
	if !containsStr(body, hookCommand) {
		t.Errorf("current hookCommand %q should be present after install", hookCommand)
	}
}
