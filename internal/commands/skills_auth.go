package commands

import (
	"context"
	"fmt"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/spf13/cobra"
)

func newSkillsModuleWithFlags(ctx context.Context, flags GlobalFlags, orgName string) (*skills.Module, *config.ConfigManager, error) {
	configManager := config.NewConfigManager(flags.ConfigFile)
	cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, orgName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	useOrg := cfg.GetOrgOrDefault(orgName)
	orgConfig, err := cfg.GetOrgConfig(useOrg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get org config: %w", err)
	}

	token, err := configManager.GetOrRefreshToken(ctx, useOrg)
	if err != nil && !config.ShouldIgnoreGetOrRefreshTokenError(err) {
		return nil, nil, err
	}

	return skills.NewModule(token, orgConfig, configManager, useOrg), configManager, nil
}

func skillsOrgName(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if cmd.Parent() != nil {
		org, _ := cmd.Parent().PersistentFlags().GetString("org")
		return org
	}
	return ""
}

func newSkillsModule(flags GlobalFlags) (*skills.Module, *config.ConfigManager, error) {
	configManager := config.NewConfigManager(flags.ConfigFile)
	cfg, err := configManager.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	orgCfg := &config.OrganizationConfig{APIURL: "https://api.getport.io/v1"}
	orgName := cfg.DefaultOrg
	if orgName != "" {
		if oc, ocErr := cfg.GetOrgConfig(orgName); ocErr == nil {
			orgCfg = oc
		}
	}
	token, _ := configManager.GetToken(orgName)
	return skills.NewModule(token, orgCfg, configManager, orgName), configManager, nil
}
