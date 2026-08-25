package skills

import (
	"testing"
)

func TestBuildProjectTargets(t *testing.T) {
	tests := []struct {
		name          string
		globalTargets []string
		projectDirs   []string
		want          []string
	}{
		{
			name:          "cursor and copilot map to .cursor and .github",
			globalTargets: []string{"/home/user/.cursor", "/home/user/.copilot"},
			projectDirs:   []string{"/my/repo"},
			want:          []string{"/my/repo/.cursor", "/my/repo/.github"},
		},
		{
			name:          "cursor and repo-scoped copilot .github path",
			globalTargets: []string{"/home/user/.cursor", "/acme/app/.github"},
			projectDirs:   []string{"/other/repo"},
			want:          []string{"/other/repo/.cursor", "/other/repo/.github"},
		},
		{
			name:          "multiple project dirs",
			globalTargets: []string{"/home/user/.copilot"},
			projectDirs:   []string{"/repo/one", "/repo/two"},
			want:          []string{"/repo/one/.github", "/repo/two/.github"},
		},
		{
			name:          "deduplicates same tool dir across different home paths",
			globalTargets: []string{"/home/user/.copilot", "/home/other/.copilot"},
			projectDirs:   []string{"/repo"},
			want:          []string{"/repo/.github"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProjectTargets(tt.globalTargets, tt.projectDirs)
			if len(result) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, result)
			}
			for _, e := range tt.want {
				if !contains(result, e) {
					t.Errorf("missing %q in %v", e, result)
				}
			}
		})
	}
}

func TestExtractProjectDirs(t *testing.T) {
	tests := []struct {
		name          string
		globalTargets []string
		envVars       map[string]string
		wantContains  []string
		wantAbsent    []string
	}{
		{
			name:          "standard paths",
			globalTargets: []string{"/home/user/.cursor", "/home/user/.copilot", "/home/user/.claude"},
			wantContains:  []string{".cursor", ".github", ".claude"},
			wantAbsent:    []string{".copilot"},
		},
		{
			name:          "XDG cursor path maps to .cursor",
			globalTargets: []string{"/home/user/.config/cursor", "/home/user/.copilot"},
			envVars:       map[string]string{"CURSOR_CONFIG_DIR": ""},
			wantContains:  []string{".cursor", ".github"},
		},
		{
			name:          "env override cursor path maps to .cursor",
			globalTargets: []string{"/custom/cursor"},
			envVars:       map[string]string{"CURSOR_CONFIG_DIR": "/custom/cursor"},
			wantContains:  []string{".cursor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			dirs := extractProjectDirs(tt.globalTargets)
			for _, want := range tt.wantContains {
				if !contains(dirs, want) {
					t.Errorf("expected %q in %v", want, dirs)
				}
			}
			for _, absent := range tt.wantAbsent {
				if contains(dirs, absent) {
					t.Errorf("expected %q to be absent in %v", absent, dirs)
				}
			}
		})
	}
}

func TestMergeUnique(t *testing.T) {
	tests := []struct {
		name      string
		existing  []string
		additions []string
		want      []string
	}{
		{"merges new entries", []string{"a", "b"}, []string{"c", "d"}, []string{"a", "b", "c", "d"}},
		{"skips duplicates", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
		{"empty existing", nil, []string{"a"}, []string{"a"}},
		{"empty additions", []string{"a"}, nil, []string{"a"}},
		{"both empty", nil, nil, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeUnique(tt.existing, tt.additions)
			if len(got) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("[%d] want %q, got %q", i, w, got[i])
				}
			}
		})
	}
}

// TestReplaceManagedTargets_DropsDeselectedKeepsForeign locks in the fix for re-running
// 'init' and deselecting a tool: the deselected target must be dropped from the saved set
// (so 'sync' stops writing to it), while a different repository's repo-scoped target is kept.
func TestReplaceManagedTargets_DropsDeselectedKeepsForeign(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CURSOR_CONFIG_DIR", "")
	home := "/home/user"
	cwd := "/repo"

	managed := TargetPaths(DefaultHookTargets(), home, cwd)
	cursor := managed[1] // global, home-based
	claude := managed[2] // global, home-based
	foreign := "/otherrepo/.github"

	// Previously configured: Cursor + Claude + another repo's repo-scoped Copilot dir.
	saved := []string{cursor, claude, foreign}
	// Re-running init now selects only Claude.
	got := replaceManagedTargets(saved, []string{claude}, home, cwd)

	if contains(got, cursor) {
		t.Errorf("deselected target %q should be dropped, got %v", cursor, got)
	}
	if !contains(got, claude) {
		t.Errorf("selected target %q should be kept, got %v", claude, got)
	}
	if !contains(got, foreign) {
		t.Errorf("foreign repo-scoped target %q should be preserved, got %v", foreign, got)
	}
}
