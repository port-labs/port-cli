package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultHookTargets_ReturnsExpectedTools(t *testing.T) {
	targets := DefaultHookTargets()
	names := make([]string, len(targets))
	for i, tg := range targets {
		names[i] = tg.Name
	}
	for _, want := range []string{"Agents (cross-platform)", "Cursor", "Claude Code", "Gemini CLI", "OpenAI Codex", "Windsurf", "GitHub Copilot"} {
		if !contains(names, want) {
			t.Errorf("expected %q in default targets", want)
		}
	}
	for _, tg := range targets {
		switch tg.Name {
		case "Agents (cross-platform)":
			if tg.Dir != ".agents" {
				t.Errorf("Agents Dir: want .agents, got %s", tg.Dir)
			}
			if !tg.SkillsOnly {
				t.Error("Agents should be skills-only (no session hooks)")
			}
		case "GitHub Copilot":
			if tg.Dir != ".github" {
				t.Errorf("GitHub Copilot Dir: want .github, got %s", tg.Dir)
			}
			// ProjectDir must stay empty: Copilot is repo-scoped and Dir is already
			// ".github". extractProjectDirs uses Dir when ProjectDir is unset. The
			// previous ~/.copilot layout used ProjectDir=".github" only to map the
			// home skill target onto per-repo .github paths — that indirection is gone.
			if tg.ProjectDir != "" {
				t.Errorf("GitHub Copilot ProjectDir: want empty, got %s", tg.ProjectDir)
			}
			if !tg.RepoScoped {
				t.Error("GitHub Copilot should be repo-scoped")
			}
			if tg.HookSubDir != "hooks" {
				t.Errorf("GitHub Copilot HookSubDir: want hooks, got %s", tg.HookSubDir)
			}
			if len(tg.LegacyHookDirs) != 1 || tg.LegacyHookDirs[0] != ".copilot" {
				t.Errorf("GitHub Copilot LegacyHookDirs: want [.copilot], got %v", tg.LegacyHookDirs)
			}
			if tg.Format != hookFormatCopilotJSON {
				t.Errorf("GitHub Copilot Format: want hookFormatCopilotJSON, got %s", tg.Format)
			}
		case "Cursor":
			if tg.EnvOverride != "CURSOR_CONFIG_DIR" {
				t.Errorf("Cursor EnvOverride: want CURSOR_CONFIG_DIR, got %s", tg.EnvOverride)
			}
			if tg.XDGDir != "cursor" {
				t.Errorf("Cursor XDGDir: want cursor, got %s", tg.XDGDir)
			}
		}
	}
}

func TestTargetPaths_ResolvesPaths(t *testing.T) {
	targets := []HookTarget{
		{Name: "Cursor", Dir: ".cursor"},
		{Name: "Claude Code", Dir: ".claude"},
	}
	paths := TargetPaths(targets, "/home/user", "/home/user")
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if !strings.HasSuffix(paths[0], ".cursor") {
		t.Errorf("expected path ending in .cursor, got %s", paths[0])
	}
	if !strings.HasSuffix(paths[1], ".claude") {
		t.Errorf("expected path ending in .claude, got %s", paths[1])
	}
}

func TestTargetPaths_RepoScopedUsesRepoRoot(t *testing.T) {
	targets := []HookTarget{
		{Name: "Global", Dir: ".cursor"},
		{Name: "Repo", Dir: ".github/hooks", RepoScoped: true},
	}
	paths := TargetPaths(targets, "/home/user", "/repo/root")
	if paths[0] != "/home/user/.cursor" {
		t.Errorf("global: want /home/user/.cursor, got %s", paths[0])
	}
	if paths[1] != "/repo/root/.github/hooks" {
		t.Errorf("repo: want /repo/root/.github/hooks, got %s", paths[1])
	}
}

func TestTargetPaths_XDGAndEnvOverride(t *testing.T) {
	targets := []HookTarget{
		{Name: "Cursor", Dir: ".cursor", EnvOverride: "CURSOR_CONFIG_DIR", XDGDir: "cursor"},
		{Name: "Claude Code", Dir: ".claude"},
	}

	t.Run("env override", func(t *testing.T) {
		t.Setenv("CURSOR_CONFIG_DIR", "/custom/cursor")
		paths := TargetPaths(targets, "/home/user", "/repo")
		if paths[0] != "/custom/cursor" {
			t.Errorf("want /custom/cursor, got %s", paths[0])
		}
		if paths[1] != "/home/user/.claude" {
			t.Errorf("want /home/user/.claude, got %s", paths[1])
		}
	})

	t.Run("XDG fallback", func(t *testing.T) {
		t.Setenv("CURSOR_CONFIG_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "/home/user/.config")
		paths := TargetPaths(targets, "/home/user", "/repo")
		want := filepath.Join("/home/user/.config", "cursor")
		if paths[0] != want {
			t.Errorf("want %s, got %s", want, paths[0])
		}
		if paths[1] != "/home/user/.claude" {
			t.Errorf("want /home/user/.claude, got %s", paths[1])
		}
	})
}

func TestResolveTargetDir(t *testing.T) {
	cursorTarget := HookTarget{Name: "Cursor", Dir: ".cursor", EnvOverride: "CURSOR_CONFIG_DIR", XDGDir: "cursor"}
	claudeTarget := HookTarget{Name: "Claude Code", Dir: ".claude"}
	repoTarget := HookTarget{Name: "Repo", Dir: ".github/hooks", RepoScoped: true, EnvOverride: "SOME_VAR", XDGDir: "repo"}

	tests := []struct {
		name    string
		target  HookTarget
		envVars map[string]string
		home    string
		repo    string
		want    string
	}{
		{
			name:    "default path (no env set)",
			target:  cursorTarget,
			envVars: map[string]string{"CURSOR_CONFIG_DIR": "", "XDG_CONFIG_HOME": ""},
			home:    "/home/user", repo: "/repo",
			want: "/home/user/.cursor",
		},
		{
			name:    "env override takes priority over XDG",
			target:  cursorTarget,
			envVars: map[string]string{"CURSOR_CONFIG_DIR": "/custom/cursor-config", "XDG_CONFIG_HOME": "/home/user/.config"},
			home:    "/home/user", repo: "/repo",
			want: "/custom/cursor-config",
		},
		{
			name:    "XDG fallback when env unset",
			target:  cursorTarget,
			envVars: map[string]string{"CURSOR_CONFIG_DIR": "", "XDG_CONFIG_HOME": "/home/user/.config"},
			home:    "/home/user", repo: "/repo",
			want: "/home/user/.config/cursor",
		},
		{
			name:    "no XDGDir ignores XDG_CONFIG_HOME",
			target:  claudeTarget,
			envVars: map[string]string{"XDG_CONFIG_HOME": "/home/user/.config"},
			home:    "/home/user", repo: "/repo",
			want: "/home/user/.claude",
		},
		{
			name:    "repo-scoped ignores env and XDG",
			target:  repoTarget,
			envVars: map[string]string{"SOME_VAR": "/custom/path", "XDG_CONFIG_HOME": "/home/user/.config"},
			home:    "/home/user", repo: "/repo",
			want: "/repo/.github/hooks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			got := resolveTargetDir(tt.target, tt.home, tt.repo)
			if got != tt.want {
				t.Errorf("want %s, got %s", tt.want, got)
			}
		})
	}
}

func TestResolveTargetNames_AgentsPath(t *testing.T) {
	targets := DefaultHookTargets()
	names := ResolveTargetNames([]string{filepath.Join("/home/user", ".agents")}, targets)
	if len(names) != 1 || names[0] != "Agents (cross-platform)" {
		t.Fatalf("got %v", names)
	}
}

func TestResolveTargetNames(t *testing.T) {
	targets := []HookTarget{
		{Name: "Cursor", Dir: ".cursor", EnvOverride: "CURSOR_CONFIG_DIR", XDGDir: "cursor"},
		{Name: "GitHub Copilot", Dir: ".github", RepoScoped: true, HookSubDir: "hooks", LegacyHookDirs: []string{".copilot"}},
		{Name: "Claude Code", Dir: ".claude"},
	}

	tests := []struct {
		name       string
		savedPaths []string
		envVars    map[string]string
		wantNames  []string
	}{
		{
			name:       "resolves by Dir suffix",
			savedPaths: []string{"/home/user/.cursor", "/home/user/.copilot"},
			wantNames:  []string{"Cursor", "GitHub Copilot"},
		},
		{
			name:       "resolves repo-scoped .github path",
			savedPaths: []string{"/work/acme/.github"},
			wantNames:  []string{"GitHub Copilot"},
		},
		{
			name:       "no matches returns nil",
			savedPaths: []string{"/home/user/.unknown"},
			wantNames:  nil,
		},
		{
			name:      "empty paths returns nil",
			wantNames: nil,
		},
		{
			name:       "no duplicate names",
			savedPaths: []string{"/home/a/.cursor", "/home/b/.cursor"},
			wantNames:  []string{"Cursor"},
		},
		{
			name:       "matches env override path",
			savedPaths: []string{"/custom/cursor", "/home/user/.claude"},
			envVars:    map[string]string{"CURSOR_CONFIG_DIR": "/custom/cursor"},
			wantNames:  []string{"Cursor", "Claude Code"},
		},
		{
			name:       "matches XDG path",
			savedPaths: []string{"/home/user/.config/cursor"},
			envVars:    map[string]string{"CURSOR_CONFIG_DIR": ""},
			wantNames:  []string{"Cursor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			got := ResolveTargetNames(tt.savedPaths, targets)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("want %v, got %v", tt.wantNames, got)
			}
			for _, want := range tt.wantNames {
				if !contains(got, want) {
					t.Errorf("expected %q in result %v", want, got)
				}
			}
		})
	}
}
