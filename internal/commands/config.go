package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// RegisterConfig registers the config command.
func RegisterConfig(rootCmd *cobra.Command) {
	var show, init bool

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Port CLI configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			if init {
				if err := configManager.CreateDefaultConfig(); err != nil {
					return fmt.Errorf("failed to create configuration: %w", err)
				}
				fmt.Printf("✓ Configuration file created at %s\n", configManager.ConfigPath())
				fmt.Println("\nPlease edit the file and add your Port credentials.")
				return nil
			}

			if show {
				cfg, err := configManager.Load()
				if err != nil {
					return fmt.Errorf("failed to load configuration: %w", err)
				}

				fmt.Println("\nCurrent Configuration:")
				fmt.Printf("Config file: %s\n", configManager.ConfigPath())
				fmt.Printf("Default org: %s\n", cfg.DefaultOrg)
				fmt.Printf("Backend URL: %s\n", cfg.Backend.URL)
				fmt.Printf("Organizations: %d\n", len(cfg.Organizations))
				for orgName := range cfg.Organizations {
					fmt.Printf("  - %s\n", orgName)
				}
				return nil
			}

			fmt.Println("Use --show to display configuration")
			fmt.Println("Use --init to create a new configuration file")
			return nil
		},
	}

	configCmd.Flags().BoolVar(&show, "show", false, "Show current configuration")
	configCmd.Flags().BoolVar(&init, "init", false, "Initialize configuration file")

	configCmd.AddCommand(registerGet())
	configCmd.AddCommand(registerSet())
	configCmd.AddCommand(registerSources())

	rootCmd.AddCommand(configCmd)
}

func registerSources() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Show configuration and environment sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)
			cfg, err := configManager.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			fmt.Println("Configuration sources:")
			fmt.Printf("Config file: %s\n", configManager.ConfigPath())
			fmt.Printf("Default org: %s\n", cfg.DefaultOrg)
			fmt.Printf("Env file loading disabled: %t\n", config.EnvFileLoadingDisabled())
			fmt.Println("Env files checked:")
			for _, source := range config.EnvFileSources() {
				status := "missing"
				if source.Exists {
					status = "present"
				}
				fmt.Printf("  - %s (%s)\n", source.Path, status)
			}
			fmt.Println("Environment overrides:")
			for _, name := range []string{"PORT_CONFIG_FILE", "PORT_CLIENT_ID", "PORT_CLIENT_SECRET", "PORT_API_URL", "PORT_TARGET_CLIENT_ID", "PORT_TARGET_CLIENT_SECRET", "PORT_TARGET_API_URL", "PORT_DEFAULT_ORG", "PORT_NO_ENV_FILE"} {
				state := "unset"
				if os.Getenv(name) != "" {
					state = "set"
				}
				fmt.Printf("  - %s: %s\n", name, state)
			}
			return nil
		},
	}
	return cmd
}

// registerGet registers the get command.
func registerGet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the value of a given configuration key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.Load()
			if err != nil {
				return fmt.Errorf("failed loading config (%w)", err)
			}

			asMap, err := configManager.AsMap(cfg)
			if err != nil {
				return fmt.Errorf("failed loading config (%w)", err)
			}

			key := args[0]
			val, ok := asMap[key]
			if !ok {
				return fmt.Errorf("failed to find key %s in config", key)
			}

			if _, ok := val.(string); ok {
				fmt.Println(val)
				return nil
			}

			out, err := json.MarshalIndent(val, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to get key %s in config", key)
			}
			fmt.Println(string(out))
			return nil
		},
	}
	return cmd
}

// registerSet registers the set command.
func registerSet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update configuration with a value for the given key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.Load()
			if err != nil {
				return fmt.Errorf("failed loading config (%w)", err)
			}

			asMap, err := configManager.AsMap(cfg)
			if err != nil {
				return fmt.Errorf("failed loading config (%w)", err)
			}

			key := args[0]
			_, ok := asMap[key]
			if !ok {
				return fmt.Errorf("failed to find key %s in config", key)
			}

			val := args[1]

			asMap[key] = val
			out, err := yaml.Marshal(asMap)
			if err != nil {
				return fmt.Errorf("failed to write new config (%w)", err)
			}
			return configManager.WriteBytes(out)
		},
	}
	return cmd
}
