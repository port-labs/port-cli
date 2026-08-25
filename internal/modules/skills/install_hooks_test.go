package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_WritesHookPerFormat(t *testing.T) {
	tests := []struct {
		name        string
		format      hookFormat
		subdir      string
		hookFile    string
		mustContain []string
	}{
		{
			name: "JSON (Cursor/Codex)", format: hookFormatJSON,
			subdir: "tooldir", hookFile: "hooks.json",
			mustContain: []string{"sessionStart", hookCommand},
		},
		{
			name: "Claude settings", format: hookFormatClaude,
			subdir: "claudedir", hookFile: "settings.json",
			mustContain: []string{"UserPromptSubmit", hookCommand},
		},
		{
			name: "Gemini settings", format: hookFormatGemini,
			subdir: "geminidir", hookFile: "settings.json",
			mustContain: []string{"SessionStart", hookCommand},
		},
		{
			name: "Windsurf hooks", format: hookFormatWindsurf,
			subdir: "wsdir", hookFile: "hooks.json",
			mustContain: []string{"pre_user_prompt", hookCommand},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			targets := []HookTarget{{Name: "Test", Dir: tt.subdir, Format: tt.format}}
			if err := InstallHooks(targets, dir, dir, ""); err != nil {
				t.Fatalf("InstallHooks: %v", err)
			}
			data, _ := os.ReadFile(filepath.Join(dir, tt.subdir, tt.hookFile))
			body := string(data)
			for _, want := range tt.mustContain {
				if !containsStr(body, want) {
					t.Errorf("missing %q in %s", want, tt.hookFile)
				}
			}
		})
	}
}

func TestInstallHooks_RepoScopedTarget(t *testing.T) {
	homeDir, repoDir := t.TempDir(), t.TempDir()
	targets := []HookTarget{{Name: "Copilot", Dir: ".github/hooks", Format: hookFormatJSON, RepoScoped: true}}

	if err := InstallHooks(targets, homeDir, repoDir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	assertFileExists(t, filepath.Join(repoDir, ".github", "hooks", "hooks.json"))
	assertFileAbsent(t, filepath.Join(homeDir, ".github", "hooks", "hooks.json"))
}

func TestInstallHooks_GitHubCopilotHooksUseAgentSchema(t *testing.T) {
	homeDir, repoDir := t.TempDir(), t.TempDir()
	copilot := HookTarget{
		Name: "GitHub Copilot", Dir: ".github", RepoScoped: true, HookSubDir: "hooks",
		Format: hookFormatCopilotJSON,
	}
	if err := InstallHooks([]HookTarget{copilot}, homeDir, repoDir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repoDir, ".github", "hooks", "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	body := string(data)
	for _, frag := range []string{
		`"version"`,
		`"sessionStart"`,
		`"type"`,
		`"command"`,
		`"bash"`,
		`"powershell"`,
		`"cwd"`,
		`"timeoutSec"`,
		hookCommand,
	} {
		if !containsStr(body, frag) {
			t.Errorf("hooks.json missing expected fragment %q in:\n%s", frag, body)
		}
	}
	if containsStr(body, `"command": "`+hookCommand) {
		t.Error("should not use Cursor-style {\"command\": ...} for the Port hook; use type/bash/powershell per GitHub Copilot docs")
	}
}

func TestInstallHooks_MergesExistingJSONHook(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"version":1,"hooks":{"preCommit":[{"command":"lint"}]}}`
	if err := os.WriteFile(filepath.Join(toolDir, "hooks.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := []HookTarget{{Name: "Tool", Dir: "tool", Format: hookFormatJSON}}
	if err := InstallHooks(targets, dir, dir, ""); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(toolDir, "hooks.json"))
	body := string(data)
	if !containsStr(body, "preCommit") {
		t.Error("existing preCommit hook lost after merge")
	}
	if !containsStr(body, "sessionStart") {
		t.Error("sessionStart hook not added")
	}
}

func TestInstallHooks_XDGAndEnvOverride(t *testing.T) {
	t.Run("env override", func(t *testing.T) {
		dir := t.TempDir()
		customDir := filepath.Join(dir, "custom-cursor")
		t.Setenv("CURSOR_CONFIG_DIR", customDir)
		targets := []HookTarget{{Name: "Cursor", Dir: ".cursor", Format: hookFormatJSON, EnvOverride: "CURSOR_CONFIG_DIR", XDGDir: "cursor"}}
		if err := InstallHooks(targets, dir, dir, ""); err != nil {
			t.Fatalf("InstallHooks: %v", err)
		}
		assertFileExists(t, filepath.Join(customDir, "hooks.json"))
		assertFileAbsent(t, filepath.Join(dir, ".cursor", "hooks.json"))
	})

	t.Run("XDG fallback", func(t *testing.T) {
		dir := t.TempDir()
		xdgDir := filepath.Join(dir, "xdg-config")
		t.Setenv("CURSOR_CONFIG_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)
		targets := []HookTarget{{Name: "Cursor", Dir: ".cursor", Format: hookFormatJSON, EnvOverride: "CURSOR_CONFIG_DIR", XDGDir: "cursor"}}
		if err := InstallHooks(targets, dir, dir, ""); err != nil {
			t.Fatalf("InstallHooks: %v", err)
		}
		assertFileExists(t, filepath.Join(xdgDir, "cursor", "hooks.json"))
	})
}
