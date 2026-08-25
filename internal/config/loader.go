package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// ConfigManager manages configuration loading with precedence: CLI flags > env vars > config file.
type ConfigManager struct {
	configPath string
}

// ConfigPath returns the configuration file path.
func (cm *ConfigManager) ConfigPath() string {
	return cm.configPath
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(configPath string) *ConfigManager {
	if configPath == "" {
		configPath = DefaultConfigPath()
	}

	// Load .env files (doesn't override existing env vars)
	loadEnvFiles()

	return &ConfigManager{
		configPath: configPath,
	}
}

// loadEnvFiles loads .env files from current directory and ~/.port/.env.
func loadEnvFiles() {
	// Skip .env loading during tests or when explicitly disabled.
	if os.Getenv("TESTING") != "" || os.Getenv("PORT_NO_ENV_FILE") != "" {
		return
	}

	// Try current directory
	if _, err := os.Stat(".env"); err == nil {
		godotenv.Load(".env")
	}

	// Try ~/.port/.env
	home, err := os.UserHomeDir()
	if err == nil {
		envPath := filepath.Join(home, ".port", ".env")
		if _, err := os.Stat(envPath); err == nil {
			godotenv.Load(envPath)
		}
	}
}

// Exists returns whether the config exists or not
func (cm *ConfigManager) Exists() (bool, error) {
	if _, err := os.Stat(cm.configPath); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

// Load loads configuration with precedence: env vars > config file > defaults.
func (cm *ConfigManager) Load() (*Config, error) {
	// Start with defaults
	cfg := &Config{
		DefaultOrg:    "",
		Organizations: make(map[string]OrganizationConfig),
		Backend: BackendConfig{
			URL:     "http://localhost:8080",
			Timeout: 300,
		},
	}

	// Load from file if exists
	if _, err := os.Stat(cm.configPath); err == nil {
		if err := cm.loadFromFile(cfg); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	cm.loadFromEnv(cfg)

	return cfg, nil
}

// LoadWithDualOverrides loads configuration with dual org support.
// Returns config, base org config, and target org config.
// Precedence: CLI flags > env vars > config file > defaults.
func (cm *ConfigManager) LoadWithDualOverrides(
	baseClientID, baseClientSecret, baseAPIURL, baseOrg string,
	targetClientID, targetClientSecret, targetAPIURL, targetOrg string,
) (*Config, *OrganizationConfig, *OrganizationConfig, error) {
	// Load base config (file + env vars)
	cfg, err := cm.Load()
	if err != nil {
		return nil, nil, nil, err
	}

	// Resolve base org config
	baseOrgConfig, err := cm.resolveOrgConfig(cfg, baseClientID, baseClientSecret, baseAPIURL, baseOrg, "base")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve base org: %w", err)
	}

	// Resolve target org config
	targetOrgConfig, err := cm.resolveOrgConfig(cfg, targetClientID, targetClientSecret, targetAPIURL, targetOrg, "target")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve target org: %w", err)
	}

	// If either config is nil and we have an org name, try to get from config file
	if baseOrgConfig == nil && baseOrg != "" {
		baseOrgConfig, err = cfg.GetOrgConfig(baseOrg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get base org config: %w", err)
		}
	}

	if targetOrgConfig == nil && targetOrg != "" {
		targetOrgConfig, err = cfg.GetOrgConfig(targetOrg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get target org config: %w", err)
		}
	}
	if baseOrgConfig == nil {
		c := cfg.Organizations[cfg.DefaultOrg]
		baseOrgConfig = &c
	}

	return cfg, baseOrgConfig, targetOrgConfig, nil
}

// resolveOrgConfig resolves organization config with CLI/env overrides.
func (cm *ConfigManager) resolveOrgConfig(cfg *Config, clientID, clientSecret, apiURL, orgName, orgType string) (*OrganizationConfig, error) {
	// Check environment variables if CLI flags not provided
	if clientID == "" {
		if orgType == "target" {
			clientID = os.Getenv("PORT_TARGET_CLIENT_ID")
		} else {
			clientID = os.Getenv("PORT_CLIENT_ID")
		}
	}
	if clientSecret == "" {
		if orgType == "target" {
			clientSecret = os.Getenv("PORT_TARGET_CLIENT_SECRET")
		} else {
			clientSecret = os.Getenv("PORT_CLIENT_SECRET")
		}
	}
	if apiURL == "" {
		if orgType == "target" {
			apiURL = os.Getenv("PORT_TARGET_API_URL")
		} else {
			apiURL = os.Getenv("PORT_API_URL")
		}
	}

	// If no CLI/env overrides and no org name specified, return nil to allow fallback
	if clientID == "" && clientSecret == "" && apiURL == "" && orgName == "" {
		return nil, nil
	}

	// Apply CLI/env overrides if provided
	if clientID != "" || clientSecret != "" || apiURL != "" {
		overrideOrg := orgName
		if overrideOrg == "" {
			overrideOrg = cfg.DefaultOrg
		}
		if overrideOrg == "" {
			overrideOrg = fmt.Sprintf("cli-override-%s", orgType)
		}

		// Get existing org config if it exists
		existingOrg, exists := cfg.Organizations[overrideOrg]

		// Build override config
		overrideConfig := OrganizationConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			APIURL:       apiURL,
		}

		// Fill in missing values from existing config or defaults
		if overrideConfig.ClientID == "" {
			if exists {
				overrideConfig.ClientID = existingOrg.ClientID
			} else {
				return nil, fmt.Errorf("%s", MissingCredentialsForOrgMessage(orgType, cm.configPath))
			}
		}
		if overrideConfig.ClientSecret == "" {
			if exists {
				overrideConfig.ClientSecret = existingOrg.ClientSecret
			} else {
				return nil, fmt.Errorf("%s", MissingCredentialsForOrgMessage(orgType, cm.configPath))
			}
		}
		if overrideConfig.APIURL == "" {
			if exists {
				overrideConfig.APIURL = existingOrg.APIURL
			} else {
				overrideConfig.APIURL = "https://api.getport.io/v1"
			}
		}

		cfg.Organizations[overrideOrg] = overrideConfig
		if orgType == "base" {
			cfg.DefaultOrg = overrideOrg
		}
	}

	return cfg.GetOrgConfig(orgName)
}

// LoadWithOverrides loads configuration with CLI flag overrides.
// Precedence: CLI flags > env vars > config file > defaults.
func (cm *ConfigManager) LoadWithOverrides(clientID, clientSecret, apiURL, orgName string) (*Config, error) {
	// Load base config (file + env vars)
	cfg, err := cm.Load()
	if err != nil {
		return nil, err
	}

	// Apply CLI overrides if provided
	if clientID != "" || clientSecret != "" || apiURL != "" {
		overrideOrg := orgName
		if overrideOrg == "" {
			overrideOrg = cfg.DefaultOrg
		}
		if overrideOrg == "" {
			overrideOrg = "cli-override"
		}

		// Get existing org config if it exists
		existingOrg, exists := cfg.Organizations[overrideOrg]

		// Build override config
		overrideConfig := OrganizationConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			APIURL:       apiURL,
		}

		// Fill in missing values from existing config or defaults
		if overrideConfig.ClientID == "" && exists {
			overrideConfig.ClientID = existingOrg.ClientID
		}
		if overrideConfig.ClientSecret == "" && exists {
			overrideConfig.ClientSecret = existingOrg.ClientSecret
		}
		if overrideConfig.APIURL == "" {
			if exists {
				overrideConfig.APIURL = existingOrg.APIURL
			} else {
				overrideConfig.APIURL = "https://api.getport.io/v1"
			}
		}

		cfg.Organizations[overrideOrg] = overrideConfig
		cfg.DefaultOrg = overrideOrg
	}

	return cfg, nil
}

// loadFromFile loads configuration from YAML file.
func (cm *ConfigManager) loadFromFile(cfg *Config) error {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return err
	}

	fileConfig := &configFileYAML{}
	if err := yaml.Unmarshal(data, fileConfig); err != nil {
		return err
	}

	// Merge file config into defaults
	if fileConfig.DefaultOrg != "" {
		cfg.DefaultOrg = fileConfig.DefaultOrg
	}
	if fileConfig.Organizations != nil {
		cfg.Organizations = fileConfig.Organizations
	}
	if fileConfig.Backend.URL != "" {
		cfg.Backend.URL = fileConfig.Backend.URL
	}
	if fileConfig.Backend.Timeout != 0 {
		cfg.Backend.Timeout = fileConfig.Backend.Timeout
	}
	cfg.Skills = mergeSkillsYAML(fileConfig.Skills, fileConfig.LegacyPlugin)

	return nil
}

// configFileYAML mirrors Config on disk, including the legacy `plugin` key for backward compatibility.
type configFileYAML struct {
	DefaultOrg    string                        `yaml:"default_org"`
	Organizations map[string]OrganizationConfig `yaml:"organizations"`
	Backend       BackendConfig                 `yaml:"backend"`
	Skills        SkillsConfig                  `yaml:"skills,omitempty"`
	LegacyPlugin  SkillsConfig                  `yaml:"plugin,omitempty"`
}

// mergeSkillsYAML prefers the `skills` section; if it has no selection, falls back to legacy `plugin`.
func mergeSkillsYAML(skills, legacyPlugin SkillsConfig) SkillsConfig {
	if skills.HasSelection() {
		return skills
	}
	return legacyPlugin
}

func (cm *ConfigManager) AsMap(cfg *Config) (map[string]any, error) {
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return nil, err
	}

	fileConfig := map[string]any{}
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return nil, err
	}

	return fileConfig, nil
}

// loadFromEnv loads configuration from environment variables.
func (cm *ConfigManager) loadFromEnv(cfg *Config) {
	// Backend URL
	if backendURL := os.Getenv("PORT_CLI_BACKEND_URL"); backendURL != "" {
		cfg.Backend.URL = backendURL
	}

	// Default org from environment
	if defaultOrg := os.Getenv("PORT_DEFAULT_ORG"); defaultOrg != "" {
		cfg.DefaultOrg = defaultOrg
	}

	// Single organization from environment variables
	clientID := os.Getenv("PORT_CLIENT_ID")
	clientSecret := os.Getenv("PORT_CLIENT_SECRET")
	apiURL := os.Getenv("PORT_API_URL")
	if apiURL == "" {
		apiURL = "https://api.getport.io/v1"
	}

	if clientID != "" && clientSecret != "" {
		// Create or override the "default" organization
		orgName := cfg.DefaultOrg
		if orgName == "" {
			orgName = "default"
		}

		if cfg.Organizations == nil {
			cfg.Organizations = make(map[string]OrganizationConfig)
		}

		cfg.Organizations[orgName] = OrganizationConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			APIURL:       apiURL,
		}

		// Set as default org if not set
		if cfg.DefaultOrg == "" {
			cfg.DefaultOrg = orgName
		}
	}
}

// CreateDefaultConfig creates a default configuration file.
func (cm *ConfigManager) CreateDefaultConfig() error {
	// Create default config
	defaultConfig := &Config{
		DefaultOrg: "production",
		Organizations: map[string]OrganizationConfig{
			"production": {
				ClientID:     "your-client-id",
				ClientSecret: "your-client-secret",
				APIURL:       "https://api.getport.io/v1",
			},
			"staging": {
				ClientID:     "your-staging-client-id",
				ClientSecret: "your-staging-client-secret",
				APIURL:       "https://api.getport.io/v1",
			},
		},
		Backend: BackendConfig{
			URL:     "http://localhost:8080",
			Timeout: 300,
		},
	}

	return cm.Write(defaultConfig)
}

func (cm *ConfigManager) Write(cfg *Config) error {
	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write to file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (cm *ConfigManager) WriteBytes(data []byte) error {
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// WriteOrgIfMissing adds the org to the config if its missing.
func (cm *ConfigManager) WriteOrgIfMissing(org string, apiUrl string) (*Config, error) {
	cfg, err := cm.Load()
	if err != nil {
		return nil, err
	}

	newDefault := strings.ToLower(org)
	newDefault = strings.ReplaceAll(newDefault, " ", "_")

	if existingOrg, ok := cfg.Organizations[newDefault]; !ok {
		cfg.Organizations[newDefault] = OrganizationConfig{APIURL: apiUrl}
	} else {
		existingOrg.APIURL = apiUrl
		cfg.Organizations[newDefault] = existingOrg
	}

	err = cm.Write(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed saving default org as %s (%w)", org, err)
	}
	return cfg, nil
}

// LoadSkillsConfig loads the skills section from the config file.
func (cm *ConfigManager) LoadSkillsConfig() (*SkillsConfig, error) {
	cfg, err := cm.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return &cfg.Skills, nil
}

// SaveSkillsConfig persists the skills section into the config file, preserving all other fields.
func (cm *ConfigManager) SaveSkillsConfig(skills *SkillsConfig) error {
	cfg, err := cm.Load()
	if err != nil {
		// Config file doesn't exist yet -- start from an empty config.
		// Load() wraps os errors, so check the file directly.
		if _, statErr := os.Stat(cm.configPath); errors.Is(statErr, os.ErrNotExist) {
			cfg = &Config{Organizations: make(map[string]OrganizationConfig)}
		} else {
			// File exists but couldn't be loaded (parse error, permissions, etc.)
			return fmt.Errorf("failed to load existing config: %w", err)
		}
	}

	cfg.Skills = *skills

	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
