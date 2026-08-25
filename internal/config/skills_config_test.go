package config

import "testing"

func TestHasSkillContentSelection(t *testing.T) {
	if (&SkillsConfig{Targets: []string{"/tmp/.cursor"}}).HasSkillContentSelection() {
		t.Fatal("targets alone should not count as skill content selection")
	}
	if !(&SkillsConfig{TeamGroupDefaults: true}).HasSkillContentSelection() {
		t.Fatal("team_group_defaults should count")
	}
	if !(&SkillsConfig{Targets: []string{"/x"}, TeamGroupDefaults: true}).HasSelection() {
		t.Fatal("targets + team defaults should HasSelection")
	}
}
