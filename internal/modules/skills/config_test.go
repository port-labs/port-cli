package skills

import (
	"testing"
	"time"

	"github.com/port-labs/port-cli/internal/config"
)

func TestSaveAndLoadSkillsConfig(t *testing.T) {
	_, cm, _ := newTestModule(t)
	cfg := &config.SkillsConfig{
		Targets:            []string{"/home/user/.cursor", "/home/user/.claude"},
		SelectAllGroups:    true,
		SelectAllUngrouped: false,
		SelectedSkills:     []string{"skill-x"},
		LastSyncedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	writeCfg(t, cm, cfg)

	loaded, err := cm.LoadSkillsConfig()
	if err != nil {
		t.Fatalf("LoadSkillsConfig: %v", err)
	}
	if len(loaded.Targets) != 2 {
		t.Errorf("Targets: got %d", len(loaded.Targets))
	}
	if !loaded.SelectAllGroups {
		t.Error("SelectAllGroups should be true")
	}
	if loaded.SelectAllUngrouped {
		t.Error("SelectAllUngrouped should be false")
	}
	if len(loaded.SelectedSkills) != 1 || loaded.SelectedSkills[0] != "skill-x" {
		t.Errorf("SelectedSkills: got %v", loaded.SelectedSkills)
	}
}

func TestSaveSkillsConfig_PreservesOtherFields(t *testing.T) {
	_, cm, _ := newTestModule(t)
	if err := cm.SaveSkillsConfig(&config.SkillsConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := cm.SaveSkillsConfig(&config.SkillsConfig{SelectAll: true}); err != nil {
		t.Fatalf("second SaveSkillsConfig: %v", err)
	}
	loaded, err := cm.LoadSkillsConfig()
	if err != nil {
		t.Fatalf("LoadSkillsConfig: %v", err)
	}
	if !loaded.SelectAll {
		t.Error("expected SelectAll=true")
	}
}

func TestSkillsConfig_HasSelection(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.SkillsConfig
		want bool
	}{
		{"empty", config.SkillsConfig{}, false},
		{"targets set", config.SkillsConfig{Targets: []string{"/foo"}}, true},
		{"select all", config.SkillsConfig{SelectAll: true}, true},
		{"select all groups", config.SkillsConfig{SelectAllGroups: true}, true},
		{"selected skills", config.SkillsConfig{SelectedSkills: []string{"s"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.HasSelection(); got != tt.want {
				t.Errorf("HasSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}
