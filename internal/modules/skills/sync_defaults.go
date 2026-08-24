package skills

import "github.com/port-labs/port-cli/internal/config"

// ApplySyncDefaults fills skill selection when the user has not run
// 'port skills init'. Targets must be configured by init or passed to sync.
func ApplySyncDefaults(cfg *config.SkillsConfig) {
	if cfg == nil {
		return
	}
	if !cfg.HasSkillContentSelection() {
		cfg.TeamGroupDefaults = true
	}
}
