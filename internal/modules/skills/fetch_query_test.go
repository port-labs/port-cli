package skills

import (
	"testing"

	"github.com/port-labs/port-cli/internal/config"
)

func TestBuildFetchSkillsQuery_ExcludesInternalByDefault(t *testing.T) {
	cfg := &config.SkillsConfig{SelectAllGroups: true}
	q := buildFetchSkillsQuery(cfg, &LoadSkillsOptions{})
	if len(q.Exclude) != 1 || q.Exclude[0] != "internal" {
		t.Fatalf("Exclude: %v", q.Exclude)
	}
}

func TestBuildFetchSkillsQuery_IncludeInternalOptIn(t *testing.T) {
	cfg := &config.SkillsConfig{SelectAllGroups: true}
	q := buildFetchSkillsQuery(cfg, &LoadSkillsOptions{IncludeInternalSkills: true})
	if len(q.Exclude) != 0 {
		t.Fatalf("expected no exclude when --include-internal, got %v", q.Exclude)
	}
}

func TestBuildFetchSkillsQuery_ExcludeLegacyFlag(t *testing.T) {
	cfg := &config.SkillsConfig{SelectAllGroups: true}
	q := buildFetchSkillsQuery(cfg, &LoadSkillsOptions{ExcludeLegacySkills: true})
	if len(q.Exclude) != 2 {
		t.Fatalf("Exclude: %v", q.Exclude)
	}
}

func TestBuildFetchSkillsQuery_SyncDoesNotExcludeFiles(t *testing.T) {
	cfg := &config.SkillsConfig{TeamGroupDefaults: true}
	q := buildFetchSkillsQuery(cfg, &LoadSkillsOptions{})
	if q.ExcludeFiles {
		t.Fatal("ExcludeFiles = true, want false for sync/write path")
	}
}
