package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// formatOutput formats and displays output data.
func formatOutput(data interface{}, format string) error {
	if err := validateStringEnum("--format", format, []string{"json", "yaml"}); err != nil {
		return err
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(data)
	case "yaml":
		encoder := yaml.NewEncoder(os.Stdout)
		defer encoder.Close()
		return encoder.Encode(data)
	default:
		// Print as-is
		fmt.Printf("%+v\n", data)
		return nil
	}
}

func getOrRefreshCommandToken(cmd *cobra.Command, configManager *config.ConfigManager, org string) (*auth.Token, error) {
	token, err := configManager.GetOrRefreshToken(cmd.Context(), org)
	if err != nil && !config.ShouldIgnoreGetOrRefreshTokenError(err) {
		return nil, err
	}

	return token, nil
}

// RegisterAPI registers the API command and all subcommands.
func RegisterAPI(rootCmd *cobra.Command) {
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Direct Port API operations",
		Long:  "Direct Port API operations for blueprints, entities, pages, etc.",
	}

	apiCmd.AddCommand(registerGenericAPICall())

	// Blueprint subcommands
	blueprintsCmd := &cobra.Command{
		Use:   "blueprints",
		Short: "Blueprint operations",
	}

	blueprintsCmd.AddCommand(registerBlueprintList())
	blueprintsCmd.AddCommand(registerBlueprintGet())
	blueprintsCmd.AddCommand(registerBlueprintCreate())
	blueprintsCmd.AddCommand(registerBlueprintUpdate())
	blueprintsCmd.AddCommand(registerBlueprintDelete())

	// Entity subcommands
	entitiesCmd := &cobra.Command{
		Use:   "entities",
		Short: "Entity operations",
	}

	entitiesCmd.AddCommand(registerEntityList())
	entitiesCmd.AddCommand(registerEntityGet())
	entitiesCmd.AddCommand(registerEntityCreate())
	entitiesCmd.AddCommand(registerEntityUpdate())
	entitiesCmd.AddCommand(registerEntityDelete())

	// Page subcommands
	pagesCmd := &cobra.Command{
		Use:   "pages",
		Short: "Page operations",
	}

	pagesCmd.AddCommand(registerPageList())
	pagesCmd.AddCommand(registerPageGet())
	pagesCmd.AddCommand(registerPageCreate())
	pagesCmd.AddCommand(registerPageUpdate())
	pagesCmd.AddCommand(registerPageDelete())

	// Team subcommands
	teamsCmd := &cobra.Command{
		Use:   "teams",
		Short: "Team operations",
	}

	teamsCmd.AddCommand(registerTeamList())
	teamsCmd.AddCommand(registerTeamCreate())
	teamsCmd.AddCommand(registerTeamUpdate())
	teamsCmd.AddCommand(registerTeamDelete())

	// User subcommands
	usersCmd := &cobra.Command{
		Use:   "users",
		Short: "User operations",
	}

	usersCmd.AddCommand(registerUserList())
	usersCmd.AddCommand(registerUserGet())

	// Scorecard subcommands
	scorecardsCmd := &cobra.Command{
		Use:   "scorecards",
		Short: "Scorecard operations",
	}

	scorecardsCmd.AddCommand(registerScorecardList())
	scorecardsCmd.AddCommand(registerScorecardCreate())
	scorecardsCmd.AddCommand(registerScorecardUpdate())
	scorecardsCmd.AddCommand(registerScorecardDelete())

	// Action subcommands
	actionsCmd := &cobra.Command{
		Use:   "actions",
		Short: "Action operations",
	}

	actionsCmd.AddCommand(registerActionList())
	actionsCmd.AddCommand(registerActionCreate())
	actionsCmd.AddCommand(registerActionUpdate())
	actionsCmd.AddCommand(registerActionDelete())

	// Permissions subcommands
	permissionsCmd := &cobra.Command{
		Use:   "permissions",
		Short: "Permission operations for blueprints, actions, and pages",
	}

	permissionsCmd.AddCommand(registerPermissionsResourceCmd(
		"blueprints",
		func(ctx context.Context, id string, c *api.Client) (api.Permissions, error) {
			return c.GetBlueprintPermissions(ctx, id)
		},
		func(ctx context.Context, id string, p api.Permissions, c *api.Client) (api.Permissions, error) {
			return c.UpdateBlueprintPermissions(ctx, id, p)
		},
	))
	permissionsCmd.AddCommand(registerPermissionsResourceCmd(
		"actions",
		func(ctx context.Context, id string, c *api.Client) (api.Permissions, error) {
			return c.GetActionPermissions(ctx, id)
		},
		func(ctx context.Context, id string, p api.Permissions, c *api.Client) (api.Permissions, error) {
			return c.UpdateActionPermissions(ctx, id, p)
		},
	))
	permissionsCmd.AddCommand(registerPermissionsResourceCmd(
		"pages",
		func(ctx context.Context, id string, c *api.Client) (api.Permissions, error) {
			return c.GetPagePermissions(ctx, id)
		},
		func(ctx context.Context, id string, p api.Permissions, c *api.Client) (api.Permissions, error) {
			return c.UpdatePagePermissions(ctx, id, p)
		},
	))

	// Agents subcommands
	agentsCmd := &cobra.Command{
		Use:   "agents",
		Short: "Agent operations",
	}
	agentsCmd.AddCommand(registerAgentInvoke())

	// AI subcommands
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Port AI operations",
	}
	aiCmd.AddCommand(registerAIInvoke())
	aiCmd.AddCommand(registerAIGet())

	// Action runs subcommands
	actionRunsCmd := &cobra.Command{
		Use:   "action-runs",
		Short: "Action run operations",
	}
	actionRunsCmd.AddCommand(registerActionRunList())
	actionRunsCmd.AddCommand(registerActionRunGet())
	actionRunsCmd.AddCommand(registerActionRunUpdate())
	actionRunsCmd.AddCommand(registerActionRunApprove())
	actionRunsCmd.AddCommand(registerActionRunExecute())

	// Webhooks subcommands
	webhooksCmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Webhook operations",
	}
	webhooksCmd.AddCommand(registerWebhookList())
	webhooksCmd.AddCommand(registerWebhookGet())
	webhooksCmd.AddCommand(registerWebhookCreate())
	webhooksCmd.AddCommand(registerWebhookUpdate())
	webhooksCmd.AddCommand(registerWebhookDelete())

	// Audit subcommands
	auditCmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit log operations",
	}
	auditCmd.AddCommand(registerAuditList())

	apiCmd.AddCommand(blueprintsCmd)
	apiCmd.AddCommand(entitiesCmd)
	apiCmd.AddCommand(pagesCmd)
	apiCmd.AddCommand(teamsCmd)
	apiCmd.AddCommand(usersCmd)
	apiCmd.AddCommand(scorecardsCmd)
	apiCmd.AddCommand(actionsCmd)
	apiCmd.AddCommand(permissionsCmd)
	apiCmd.AddCommand(agentsCmd)
	apiCmd.AddCommand(aiCmd)
	apiCmd.AddCommand(actionRunsCmd)
	apiCmd.AddCommand(webhooksCmd)
	apiCmd.AddCommand(auditCmd)

	rootCmd.AddCommand(apiCmd)
}

// registerBlueprintList registers the blueprint list command.
func registerBlueprintList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all blueprints",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetBlueprints(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list blueprints: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerBlueprintGet registers the blueprint get command.
func registerBlueprintGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [blueprint-id]",
		Short: "Get a specific blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetBlueprint(cmd.Context(), blueprintID)
			if err != nil {
				return fmt.Errorf("failed to get blueprint: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerBlueprintCreate registers the blueprint create command.
func registerBlueprintCreate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new blueprint",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}

			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			// Load data file
			defer client.Close()

			result, err := client.CreateBlueprint(cmd.Context(), api.Blueprint(data))
			if err != nil {
				return fmt.Errorf("failed to create blueprint: %w", err)
			}

			cmd.Printf("✓ Blueprint created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with blueprint data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerBlueprintUpdate registers the blueprint update command.
func registerBlueprintUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [blueprint-id]",
		Short: "Update an existing blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			// Load data file
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}

			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateBlueprint(cmd.Context(), blueprintID, api.Blueprint(data))
			if err != nil {
				return fmt.Errorf("failed to update blueprint: %w", err)
			}

			cmd.Printf("✓ Blueprint updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with blueprint data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerBlueprintDelete registers the blueprint delete command.
func registerBlueprintDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [blueprint-id]",
		Short: "Delete a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]

			if !force {
				confirm, err := cmd.Flags().GetBool("yes")
				if err != nil || !confirm {
					cmd.Printf("Are you sure you want to delete blueprint '%s'? [y/N]: ", blueprintID)
					var response string
					fmt.Scanln(&response)
					if response != "y" && response != "Y" {
						cmd.Println("Operation cancelled")
						return nil
					}
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteBlueprint(cmd.Context(), blueprintID); err != nil {
				return fmt.Errorf("failed to delete blueprint: %w", err)
			}

			cmd.Printf("✓ Blueprint '%s' deleted successfully!\n", blueprintID)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerEntityList registers the entity list command.
func registerEntityList() *cobra.Command {
	var org, format, blueprint string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			var result []api.Entity
			if blueprint != "" {
				entities, err := client.GetEntities(cmd.Context(), blueprint, nil)
				if err != nil {
					return fmt.Errorf("failed to list entities: %w", err)
				}
				result = entities
			} else {
				// Get all blueprints and then all entities
				blueprints, err := client.GetBlueprints(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to get blueprints: %w", err)
				}

				for _, bp := range blueprints {
					if identifier, ok := bp["identifier"].(string); ok {
						entities, err := client.GetEntities(cmd.Context(), identifier, nil)
						if err != nil {
							continue // Skip blueprints without entities
						}
						result = append(result, entities...)
					}
				}
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Filter by blueprint ID")

	return cmd
}

// registerEntityGet registers the entity get command.
func registerEntityGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [blueprint-id] [entity-id]",
		Short: "Get a specific entity",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			entityID := args[1]

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetEntity(cmd.Context(), blueprintID, entityID)
			if err != nil {
				return fmt.Errorf("failed to get entity: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerEntityCreate registers the entity create command.
func registerEntityCreate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "create [blueprint-id]",
		Short: "Create a new entity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}

			// Load data file
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}

			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreateEntity(cmd.Context(), blueprintID, api.Entity(data))
			if err != nil {
				return fmt.Errorf("failed to create entity: %w", err)
			}

			cmd.Printf("✓ Entity created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with entity data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerEntityUpdate registers the entity update command.
func registerEntityUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [blueprint-id] [entity-id]",
		Short: "Update an existing entity",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			entityID := args[1]

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}

			// Load data file
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}

			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateEntity(cmd.Context(), blueprintID, entityID, api.Entity(data))
			if err != nil {
				return fmt.Errorf("failed to update entity: %w", err)
			}

			cmd.Printf("✓ Entity updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with entity data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerEntityDelete registers the entity delete command.
func registerEntityDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [blueprint-id] [entity-id]",
		Short: "Delete an entity",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			entityID := args[1]

			if !force {
				cmd.Printf("Are you sure you want to delete entity '%s' from blueprint '%s'? [y/N]: ", entityID, blueprintID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteEntity(cmd.Context(), blueprintID, entityID); err != nil {
				return fmt.Errorf("failed to delete entity: %w", err)
			}

			cmd.Printf("✓ Entity '%s' deleted successfully!\n", entityID)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerPageGet registers the page get command.
func registerPageGet() *cobra.Command {
	var org, format string
	var compact bool

	cmd := &cobra.Command{
		Use:   "get [page-id]",
		Short: "Get a specific page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID := args[0]

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetPage(cmd.Context(), pageID)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			if compact {
				compacted := make(api.Page, len(result))
				for key, value := range result {
					if key == "widgets" {
						continue
					}
					compacted[key] = value
				}
				result = compacted
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().BoolVar(&compact, "compact", true, "Remove the widgets key from the printed payload")

	return cmd
}

// registerPageDelete registers the page delete command.
func registerPageDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [page-id]",
		Short: "Delete a page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID := args[0]

			if !force {
				cmd.Printf("Are you sure you want to delete page '%s'? [y/N]: ", pageID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeletePage(cmd.Context(), pageID); err != nil {
				return fmt.Errorf("failed to delete page: %w", err)
			}

			cmd.Printf("✓ Page '%s' deleted successfully!\n", pageID)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerPageList registers the page list command.
func registerPageList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all pages",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetPages(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list pages: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerPageCreate registers the page create command.
func registerPageCreate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new page",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreatePage(cmd.Context(), api.Page(data))
			if err != nil {
				return fmt.Errorf("failed to create page: %w", err)
			}

			cmd.Printf("✓ Page created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with page data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerPageUpdate registers the page update command.
func registerPageUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [page-id]",
		Short: "Update an existing page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdatePage(cmd.Context(), pageID, api.Page(data))
			if err != nil {
				return fmt.Errorf("failed to update page: %w", err)
			}

			cmd.Printf("✓ Page updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with page data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerTeamList registers the team list command.
func registerTeamList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all teams",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetTeams(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list teams: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerTeamCreate registers the team create command.
func registerTeamCreate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new team",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreateTeam(cmd.Context(), api.Team(data))
			if err != nil {
				return fmt.Errorf("failed to create team: %w", err)
			}

			cmd.Printf("✓ Team created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with team data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerTeamUpdate registers the team update command.
func registerTeamUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [team-name]",
		Short: "Update an existing team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			teamName := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateTeam(cmd.Context(), teamName, api.Team(data))
			if err != nil {
				return fmt.Errorf("failed to update team: %w", err)
			}

			cmd.Printf("✓ Team updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with team data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerTeamDelete registers the team delete command.
func registerTeamDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [team-name]",
		Short: "Delete a team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			teamName := args[0]

			if !force {
				cmd.Printf("Are you sure you want to delete team '%s'? [y/N]: ", teamName)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteTeam(cmd.Context(), teamName); err != nil {
				return fmt.Errorf("failed to delete team: %w", err)
			}

			cmd.Printf("✓ Team '%s' deleted successfully!\n", teamName)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerAgentInvoke registers the agent invoke command.
func registerAgentInvoke() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "invoke [agent-id]",
		Short: "Invoke an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.Request(cmd.Context(), api.RequestParams{
				Method:   "POST",
				Endpoint: fmt.Sprintf("/agent/%s/invoke", agentID),
				Data:     data,
			})
			if err != nil {
				return fmt.Errorf("failed to invoke agent: %w", err)
			}

			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with invocation body")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerAIInvoke registers the AI invoke command.
func registerAIInvoke() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Invoke Port AI",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.Request(cmd.Context(), api.RequestParams{
				Method:   "POST",
				Endpoint: "/ai/invoke",
				Data:     data,
			})
			if err != nil {
				return fmt.Errorf("failed to invoke AI: %w", err)
			}

			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with AI invocation body")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerAIGet registers the AI get invocation command.
func registerAIGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [invocation-id]",
		Short: "Get an AI invocation result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			invocationID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.Request(cmd.Context(), api.RequestParams{
				Method:   "GET",
				Endpoint: fmt.Sprintf("/ai/invoke/%s", invocationID),
			})
			if err != nil {
				return fmt.Errorf("failed to get AI invocation: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerActionRunList registers the action run list command.
func registerActionRunList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all action runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetActionRuns(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list action runs: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerActionRunGet registers the action run get command.
func registerActionRunGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [run-id]",
		Short: "Get a specific action run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetActionRun(cmd.Context(), runID)
			if err != nil {
				return fmt.Errorf("failed to get action run: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerActionRunUpdate registers the action run update command.
func registerActionRunUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [run-id]",
		Short: "Update an action run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateActionRun(cmd.Context(), runID, data)
			if err != nil {
				return fmt.Errorf("failed to update action run: %w", err)
			}

			cmd.Printf("✓ Action run updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with action run update data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerActionRunApprove registers the action run approve command.
func registerActionRunApprove() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "approve [run-id]",
		Short: "Approve or decline an action run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.ApproveActionRun(cmd.Context(), runID, data)
			if err != nil {
				return fmt.Errorf("failed to approve action run: %w", err)
			}

			cmd.Printf("✓ Action run approval submitted!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with approval data (e.g. {\"status\":\"APPROVED\",\"description\":\"...\"})")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerActionRunExecute registers the action execute command.
func registerActionRunExecute() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "execute [action-id]",
		Short: "Execute an action (create a new action run)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.ExecuteAction(cmd.Context(), actionID, data)
			if err != nil {
				return fmt.Errorf("failed to execute action: %w", err)
			}

			cmd.Printf("✓ Action executed successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with action run body")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerWebhookList registers the webhook list command.
func registerWebhookList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all webhooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetWebhooks(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list webhooks: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerWebhookGet registers the webhook get command.
func registerWebhookGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [webhook-id]",
		Short: "Get a specific webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetWebhook(cmd.Context(), id)
			if err != nil {
				return fmt.Errorf("failed to get webhook: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerWebhookCreate registers the webhook create command.
func registerWebhookCreate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new webhook",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreateWebhook(cmd.Context(), data)
			if err != nil {
				return fmt.Errorf("failed to create webhook: %w", err)
			}

			cmd.Printf("✓ Webhook created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with webhook data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerWebhookUpdate registers the webhook update command.
func registerWebhookUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [webhook-id]",
		Short: "Update an existing webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateWebhook(cmd.Context(), id, data)
			if err != nil {
				return fmt.Errorf("failed to update webhook: %w", err)
			}

			cmd.Printf("✓ Webhook updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with webhook data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerWebhookDelete registers the webhook delete command.
func registerWebhookDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [webhook-id]",
		Short: "Delete a webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if !force {
				cmd.Printf("Are you sure you want to delete webhook '%s'? [y/N]: ", id)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteWebhook(cmd.Context(), id); err != nil {
				return fmt.Errorf("failed to delete webhook: %w", err)
			}

			cmd.Printf("✓ Webhook '%s' deleted successfully!\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// registerAuditList registers the audit log list command.
func registerAuditList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetAuditLogs(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list audit logs: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// loadJSONFile loads a JSON file and returns its contents as a map.
func loadJSONFile(filePath string) (map[string]interface{}, error) {
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("data file not found: %s", filePath)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, nil
}

func registerGenericAPICall() *cobra.Command {
	var method, org, format, data, unwrap string

	cmd := &cobra.Command{
		Use:   "call",
		Short: "Generic API operations",
		Long:  "Generic API operations. Prints the raw Port API response envelope by default; use --unwrap to print a top-level response field such as blueprints.",
		Example: ` # get raw blueprints response envelope
port api call /blueprints

# print only the blueprints array from the raw response
port api call /blueprints --unwrap blueprints

# trigger an action
port api call /actions/my-action/runs --data '{"properties": {}}'

# get action runs for org
port api call /actions/runs --org my-org`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			endpoint := args[0]

			if method == "" {
				if data == "" {
					method = "GET"
				} else {
					method = "POST"
				}
			}

			var parsedData map[string]any
			if data == "" {
				parsedData = nil
			} else {
				err := json.Unmarshal([]byte(data), &parsedData)
				if err != nil {
					return fmt.Errorf("failed encoding body (%w)", err)
				}
			}
			result, err := client.Request(cmd.Context(), api.RequestParams{Method: method, Endpoint: endpoint, Data: parsedData})
			if err != nil {
				return fmt.Errorf("failed to perform request %s to %s (%w)", method, endpoint, err)
			}
			if unwrap != "" {
				responseMap, ok := result.(map[string]interface{})
				if !ok {
					return fmt.Errorf("failed to unwrap %q: response is not a JSON object", unwrap)
				}
				value, ok := responseMap[unwrap]
				if !ok {
					return fmt.Errorf("failed to unwrap %q from response", unwrap)
				}
				return formatOutput(value, format)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&method, "method", "X", "", `The HTTP method for the request (default "GET")`)
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().StringVar(&data, "data", "", "Data passed in the request body")
	cmd.Flags().StringVar(&unwrap, "unwrap", "", "Print only this top-level field from the raw API response envelope")

	return cmd
}

// registerUserList registers the user list command.
func registerUserList() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetUsers(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list users: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerUserGet registers the user get command.
func registerUserGet() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "get [email]",
		Short: "Get a specific user by email",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.GetUser(cmd.Context(), email)
			if err != nil {
				return fmt.Errorf("failed to get user: %w", err)
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerScorecardList registers the scorecard list command.
func registerScorecardList() *cobra.Command {
	var org, format, blueprint string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List scorecards",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			var result []api.Scorecard
			if blueprint != "" {
				result, err = client.GetScorecards(cmd.Context(), blueprint)
				if err != nil {
					return fmt.Errorf("failed to list scorecards: %w", err)
				}
			} else {
				result, err = client.GetAllScorecards(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to list scorecards: %w", err)
				}
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Filter by blueprint ID")

	return cmd
}

// registerScorecardCreate registers the scorecard create command.
func registerScorecardCreate() *cobra.Command {
	var org, dataFile, blueprint string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new scorecard",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreateScorecard(cmd.Context(), blueprint, api.Scorecard(data))
			if err != nil {
				return fmt.Errorf("failed to create scorecard: %w", err)
			}

			cmd.Printf("✓ Scorecard created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with scorecard data")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Blueprint ID")
	cmd.MarkFlagRequired("data")
	cmd.MarkFlagRequired("blueprint")

	return cmd
}

// registerScorecardUpdate registers the scorecard update command.
func registerScorecardUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [blueprint-id] [scorecard-id]",
		Short: "Update an existing scorecard",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			scorecardID := args[1]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateScorecard(cmd.Context(), blueprintID, scorecardID, api.Scorecard(data))
			if err != nil {
				return fmt.Errorf("failed to update scorecard: %w", err)
			}

			cmd.Printf("✓ Scorecard updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with scorecard data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerScorecardDelete registers the scorecard delete command.
func registerScorecardDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [blueprint-id] [scorecard-id]",
		Short: "Delete a scorecard",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			scorecardID := args[1]

			if !force {
				cmd.Printf("Are you sure you want to delete scorecard '%s' from blueprint '%s'? [y/N]: ", scorecardID, blueprintID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteScorecard(cmd.Context(), blueprintID, scorecardID); err != nil {
				return fmt.Errorf("failed to delete scorecard: %w", err)
			}

			cmd.Printf("✓ Scorecard '%s' deleted successfully!\n", scorecardID)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerActionList registers the action list command.
func registerActionList() *cobra.Command {
	var org, format, blueprint string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List actions",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			var result []api.Action
			if blueprint != "" {
				result, err = client.GetActions(cmd.Context(), blueprint)
				if err != nil {
					return fmt.Errorf("failed to list actions: %w", err)
				}
			} else {
				result, err = client.GetAllActions(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to list actions: %w", err)
				}
			}

			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Filter by blueprint ID")

	return cmd
}

// registerActionCreate registers the action create command.
func registerActionCreate() *cobra.Command {
	var org, dataFile, blueprint string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new action",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.CreateAction(cmd.Context(), blueprint, api.Action(data))
			if err != nil {
				return fmt.Errorf("failed to create action: %w", err)
			}

			cmd.Printf("✓ Action created successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with action data")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Blueprint ID")
	cmd.MarkFlagRequired("data")
	cmd.MarkFlagRequired("blueprint")

	return cmd
}

// registerActionUpdate registers the action update command.
func registerActionUpdate() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "update [blueprint-id] [action-id]",
		Short: "Update an existing action",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			actionID := args[1]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := client.UpdateAction(cmd.Context(), blueprintID, actionID, api.Action(data))
			if err != nil {
				return fmt.Errorf("failed to update action: %w", err)
			}

			cmd.Printf("✓ Action updated successfully!\n")
			return formatOutput(result, "json")
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with action data")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerActionDelete registers the action delete command.
func registerActionDelete() *cobra.Command {
	var org string
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [blueprint-id] [action-id]",
		Short: "Delete an action",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]
			actionID := args[1]

			if !force {
				cmd.Printf("Are you sure you want to delete action '%s' from blueprint '%s'? [y/N]: ", actionID, blueprintID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			if err := client.DeleteAction(cmd.Context(), blueprintID, actionID); err != nil {
				return fmt.Errorf("failed to delete action: %w", err)
			}

			cmd.Printf("✓ Action '%s' deleted successfully!\n", actionID)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

// registerPermissionsResourceCmd creates get/update subcommands for a permissions resource.
func registerPermissionsResourceCmd(
	resourceName string,
	getFunc func(ctx context.Context, id string, client *api.Client) (api.Permissions, error),
	updateFunc func(ctx context.Context, id string, perms api.Permissions, client *api.Client) (api.Permissions, error),
) *cobra.Command {
	singular := resourceName[:len(resourceName)-1]

	resourceCmd := &cobra.Command{
		Use:   resourceName,
		Short: resourceName + " permission operations",
	}

	// get subcommand
	var getOrg, getFormat string
	getCmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get permissions for a " + singular,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, getOrg)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(getOrg)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := getFunc(cmd.Context(), id, client)
			if err != nil {
				return fmt.Errorf("failed to get permissions: %w", err)
			}

			return formatOutput(result, getFormat)
		},
	}
	getCmd.Flags().StringVar(&getOrg, "org", "", "Organization name (uses default if not specified)")
	getCmd.Flags().StringVarP(&getFormat, "format", "f", "json", "Output format: json, yaml")

	// update subcommand
	var updateOrg, updateDataFile string
	updateCmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Update permissions for a " + singular,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, updateOrg)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			useOrg := cfg.GetOrgOrDefault(updateOrg)
			orgConfig, err := cfg.GetOrgConfig(useOrg)
			if err != nil {
				return err
			}
			data, err := loadJSONFile(updateDataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
			if err != nil {
				return err
			}
			client := api.NewClient(api.ClientOpts{
				Token:        token,
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
				Timeout:      0,
			})
			defer client.Close()

			result, err := updateFunc(cmd.Context(), id, api.Permissions(data), client)
			if err != nil {
				return fmt.Errorf("failed to update permissions: %w", err)
			}

			cmd.Printf("✓ Permissions updated successfully!\n")
			return formatOutput(result, "json")
		},
	}
	updateCmd.Flags().StringVar(&updateOrg, "org", "", "Organization name (uses default if not specified)")
	updateCmd.Flags().StringVar(&updateDataFile, "data", "", "JSON file with permissions data")
	updateCmd.MarkFlagRequired("data")

	resourceCmd.AddCommand(getCmd)
	resourceCmd.AddCommand(updateCmd)

	return resourceCmd
}
