package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// OrganizationConfig represents configuration for a Port organization.
type OrganizationConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	APIURL       string `yaml:"api_url"`
}

// BackendConfig represents configuration for the backend server (legacy, may not be used).
type BackendConfig struct {
	URL     string `yaml:"url"`
	Timeout int    `yaml:"timeout"`
}

// SkillsConfig holds configuration for the port skills feature (hooks, selection, sync state).
type SkillsConfig struct {
	// Targets is the list of AI tool hook directories (e.g. ~/.cursor, ~/.claude).
	Targets []string `yaml:"targets"`
	// ProjectDirs is the accumulated list of project directories where
	// 'port skills init' has been run. Project-scoped skills are written
	// to every directory in this list on each sync.
	ProjectDirs        []string `yaml:"project_dirs,omitempty"`
	SelectAll          bool     `yaml:"select_all"`
	SelectAllGroups    bool     `yaml:"select_all_groups"`
	SelectAllUngrouped bool     `yaml:"select_all_ungrouped"`
	SelectedGroups     []string `yaml:"selected_groups"`
	SelectedSkills     []string `yaml:"selected_skills"`
	IncludeGroups      []string `yaml:"include_groups,omitempty"`
	ExcludeGroups      []string `yaml:"exclude_groups,omitempty"`
	TeamGroupDefaults  bool     `yaml:"team_group_defaults,omitempty"`
	LastSyncedAt       string   `yaml:"last_synced_at"`
}

// HasSelection reports whether any skill selection has been configured.
func (p *SkillsConfig) HasSelection() bool {
	return len(p.Targets) > 0 || p.HasSkillContentSelection()
}

// HasSkillContentSelection reports whether group/skill sync filters are configured.
func (p *SkillsConfig) HasSkillContentSelection() bool {
	return p.SelectAll || p.SelectAllGroups ||
		p.SelectAllUngrouped || len(p.SelectedGroups) > 0 || len(p.SelectedSkills) > 0 ||
		len(p.IncludeGroups) > 0 || len(p.ExcludeGroups) > 0 || p.TeamGroupDefaults
}

// UsesTeamGroupDefaults reports whether grouped skills are resolved via team defaults on GET /skills.
func (p *SkillsConfig) UsesTeamGroupDefaults() bool {
	return p.TeamGroupDefaults || len(p.IncludeGroups) > 0 || len(p.ExcludeGroups) > 0
}

// Config represents the main configuration structure.
type Config struct {
	DefaultOrg    string                        `yaml:"default_org"`
	Organizations map[string]OrganizationConfig `yaml:"organizations"`
	Backend       BackendConfig                 `yaml:"backend"`
	Skills        SkillsConfig                  `yaml:"skills,omitempty"`
}

// DefaultConfigPath returns the default path to the configuration file.
func DefaultConfigPath() string {
	if configFile := os.Getenv("PORT_CONFIG_FILE"); configFile != "" {
		return configFile
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".port/config.yaml"
	}
	return filepath.Join(home, ".port", "config.yaml")
}

func (c *Config) GetOrgOrDefault(orgName string) string {
	org := orgName
	if org == "" {
		org = c.DefaultOrg
	}
	return org
}

// GetOrgConfig returns the configuration for a specific organization.
func (c *Config) GetOrgConfig(orgName string) (*OrganizationConfig, error) {
	// Use default org if no name specified
	if orgName == "" {
		orgName = c.DefaultOrg
	}

	// If still no org name, try to use the first available org
	if orgName == "" {
		if len(c.Organizations) == 0 {
			return nil, fmt.Errorf("%s", MissingAuthCredentialsMessage(DefaultConfigPath()))
		}

		// Use first organization
		for name := range c.Organizations {
			orgName = name
			break
		}
	}

	org, exists := c.Organizations[orgName]
	if !exists {
		orgNames := make([]string, 0, len(c.Organizations))
		for name := range c.Organizations {
			orgNames = append(orgNames, name)
		}
		return nil, fmt.Errorf("organization '%s' not found in configuration. Available organizations: %v", orgName, orgNames)
	}

	return &org, nil
}

// Validate ensures the configuration is valid.
func (c *Config) Validate() error {
	if len(c.Organizations) == 0 {
		return fmt.Errorf("no organizations configured")
	}

	for name, org := range c.Organizations {
		if org.APIURL == "" {
			return fmt.Errorf("organization '%s' missing api_url", name)
		}
	}

	return nil
}
