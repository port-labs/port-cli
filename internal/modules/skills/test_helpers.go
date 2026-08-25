package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/config"
)

// ---------------------------------------------------------------------------
// Module / config helpers
// ---------------------------------------------------------------------------

func newTestModule(t *testing.T) (*Module, *config.ConfigManager, string) {
	t.Helper()
	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	orgCfg := &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       "https://api.getport.io/v1",
	}
	return NewModule(nil, orgCfg, cm, ""), cm, dir
}

func writeCfg(t *testing.T, cm *config.ConfigManager, cfg *config.SkillsConfig) {
	t.Helper()
	if err := cm.SaveSkillsConfig(cfg); err != nil {
		t.Fatalf("SaveSkillsConfig: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// assertFileExists fails if path does not exist.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}

// assertFileAbsent fails if path exists.
func assertFileAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be absent: %s", path)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(content) != want {
		t.Errorf("content mismatch for %s: want %q, got %q", path, want, string(content))
	}
}

// ---------------------------------------------------------------------------
// Skill / path helpers
// ---------------------------------------------------------------------------

// skillMDPath returns the expected SKILL.md path inside a target directory.
func skillMDPath(targetDir, groupID, skillID string) string {
	return filepath.Join(targetDir, "skills", skillID, "SKILL.md")
}

func identifiers(skills []Skill) []string {
	ids := make([]string, len(skills))
	for i, s := range skills {
		ids[i] = s.Identifier
	}
	return ids
}

// ---------------------------------------------------------------------------
// Generic string slice helpers
// ---------------------------------------------------------------------------

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func containsStr(body, substr string) bool {
	return strings.Contains(body, substr)
}
