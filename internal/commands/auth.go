package commands

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

// RegisterAuth registers the auth command and all subcommands.
func RegisterAuth(rootCmd *cobra.Command) {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate the CLI with Port",
		Long:  "Authenticate the CLI with Port using SSO",
	}

	authCmd.AddCommand(registerLogin())
	authCmd.AddCommand(registerToken())
	authCmd.AddCommand(registerStatus())
	authCmd.AddCommand(registerLogout())

	rootCmd.AddCommand(authCmd)
}

// registerLogin registers the login command.
func registerLogin() *cobra.Command {
	var org string
	var withToken bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Port",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, org, withToken)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVar(&withToken, "with-token", false, "Read token from standard input")

	return cmd
}

func runLogin(cmd *cobra.Command, org string, withToken bool) error {
	ctx := cmd.Context()
	flags := GetGlobalFlags(cmd.Context())
	configManager := config.NewConfigManager(flags.ConfigFile)
	createdDefaultCfg := false

	if exists, err := configManager.Exists(); err != nil {
		return fmt.Errorf("failed to check if config exists (%w)", err)
	} else if !exists {
		err := configManager.CreateDefaultConfig()
		if err != nil {
			return fmt.Errorf("failed creating default config (%w)", err)
		}
		createdDefaultCfg = true
	}

	if withToken {
		return loginWithStdinToken(configManager, org)
	}

	var region string
	cfg, err := configManager.LoadWithOverrides(
		flags.ClientID,
		flags.ClientSecret,
		flags.APIURL,
		org,
	)
	if err != nil {
		return fmt.Errorf("failed to load configuration (%w)", err)
	}

	useOrg := cfg.GetOrgOrDefault(org)
	orgConfig, err := cfg.GetOrgConfig(useOrg)
	if useOrg == "" || err != nil {
		lipgloss.Printf("%s No org provided or configured as default\n", styles.QuestionMark)
	}

	apiUrl := "https://api.getport.io/v1"
	if flags.APIURL == "" && (orgConfig == nil || orgConfig.APIURL == "") {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Choose your region").
					Options(
						huh.NewOption("Europe", "eu"),
						huh.NewOption("US", "us"),
					).
					Value(&region),
			)).WithTheme(&styles.FormTheme{})
		err = form.Run()
		if err != nil {
			return fmt.Errorf("unexpected error (%w)", err)
		}
		if region == "us" {
			apiUrl = "https://api.us.getport.io/v1"
		}
	} else if orgConfig != nil {
		apiUrl = orgConfig.APIURL
	} else {
		apiUrl = flags.APIURL
	}

	baseUrl := strings.Replace(apiUrl, "api", "auth", 1)
	if strings.Contains(apiUrl, "stg-01") || strings.Contains(apiUrl, "localhost") {
		baseUrl = "https://auth.staging.getport.io"
	}

	token, err := auth.TokenFromOAuth(ctx, auth.LoginOpts{
		Org:     useOrg,
		BaseURL: strings.TrimSuffix(baseUrl, "/v1"),
		APIURL:  strings.TrimSuffix(apiUrl, "/v1"),
	})
	if err != nil && errors.Is(err, auth.ErrInterrupted) {
		return err
	}
	if err != nil {
		return fmt.Errorf("unexpected error (%w)", err)
	}

	if err = configManager.StoreToken(useOrg, token); err != nil {
		return fmt.Errorf("unexpected error while storing the token (%w)", err)
	}

	tokenOrg := token.Claims.OrgName
	if cfg, err := configManager.WriteOrgIfMissing(tokenOrg, apiUrl); err != nil {
		lipgloss.Printf(
			"%s failed saving org %s\n",
			styles.Cross,
			styles.Bold.Render(tokenOrg),
		)
	} else if cfg.DefaultOrg == "" || createdDefaultCfg {
		cfg.DefaultOrg = useOrg
		err := configManager.Write(cfg)
		if err != nil {
			lipgloss.Printf(
				"%s failed setting default org as %s\n",
				styles.Cross,
				styles.Bold.Render(tokenOrg),
			)
		}
	}

	{
		lipgloss.Printf(
			"%s Set %s as the default org\n",
			styles.CheckMark,
			styles.Bold.Render(tokenOrg),
		)
	}

	lipgloss.Printf(
		"%s Successfully logged in as %s to %s (%s)\n",
		styles.CheckMark,
		styles.Bold.Render(token.Claims.Email),
		styles.Bold.Render(tokenOrg),
		token.Claims.Audience,
	)

	return nil
}

// registerToken registers the token command.
func registerToken() *cobra.Command {
	var org string
	var noBearer bool
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token Port uses for the given org",
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
				return fmt.Errorf("failed to load configuration (%w)", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)

			token, err := configManager.GetOrRefreshToken(cmd.Context(), useOrg)

			printedToken := ""
			if err == nil {
				if noBearer {
					printedToken = token.Token
				} else {
					printedToken = fmt.Sprintf("Bearer %s", token.Token)
				}
			} else {
				if config.ShouldIgnoreGetOrRefreshTokenError(err) && token != nil {
					fmt.Print(token.Token)
					return nil
				}
				return fmt.Errorf("failed fetching token (%w)", err)
			}

			fmt.Print(printedToken)
			return nil
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().BoolVar(&noBearer, "no-bearer", false, "Print the token without the Bearer prefix")
	return cmd
}

// registerStatus registers the status command.
func registerStatus() *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display active account and authentication state on each known org.",
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
				return fmt.Errorf("failed to load configuration (%w)", err)
			}

			now := time.Now()
			printOrg := func(cfgOrg string) {
				lipgloss.Printf("%s:\n", styles.Bold.Render(cfgOrg))
				token, err := configManager.GetToken(cfgOrg)
				if err != nil {
					lipgloss.Printf("  Failed fetching token (%v)\n\n", err)
					return
				}
				expiry := token.Claims.Expiry
				if expiry.Before(now) {
					lipgloss.Printf("  %s Auth token expired\n", styles.Cross)
					lipgloss.Printf("  - Expiry: %s (%s ago)\n", styles.Bold.Render(expiry.Format(time.DateTime)), time.Since(expiry).Truncate(time.Second))
				} else {
					lipgloss.Printf("  %s Logged in to %s account %s (%s) \n", styles.CheckMark, styles.Bold.Render(token.Claims.OrgName), styles.Bold.Render(token.Claims.Email), token.Claims.Audience)
					lipgloss.Printf("  - Expiry: %s (%s left)\n", styles.Bold.Render(expiry.Format(time.DateTime)), expiry.Sub(now).Truncate(time.Second))
				}
				for _, line := range printTokenRefreshStatus(token, expiry.Before(now)) {
					lipgloss.Printf("  - %s\n", line)
				}
				lipgloss.Printf("  - Token: %s\n", styles.Bold.Render(strings.Split(token.Token, ".")[0]+strings.Repeat("*", 25)))

				fmt.Println()
			}

			if org != "" {
				useOrg := cfg.GetOrgOrDefault(org)
				printOrg(useOrg)
			} else {
				for cfgOrg := range cfg.Organizations {
					printOrg(cfgOrg)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	return cmd
}

// registerLogout registers the logout command.
func registerLogout() *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout from Port",
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
				return fmt.Errorf("failed to load configuration (%w)", err)
			}

			useOrg := cfg.GetOrgOrDefault(org)
			if useOrg == "" {
				return fmt.Errorf("org not found. Configure a default org or pass the --org flag")
			}

			token, err := configManager.GetToken(useOrg)
			if err != nil {
				return fmt.Errorf("failed logging out (%w)", err)
			}
			if err = configManager.DeleteToken(useOrg); err != nil {
				return fmt.Errorf("failed deleting the token from cache (%w)", err)
			}

			lipgloss.Printf(
				"%s Successfully logged out %s from %s\n",
				styles.CheckMark,
				styles.Bold.Render(token.Claims.Email),
				styles.Bold.Render(token.Claims.OrgName),
			)

			return nil
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	return cmd
}

func printTokenRefreshStatus(token *auth.Token, expired bool) []string {
	if token.RefreshToken != "" && token.AuthBaseURL != "" {
		if expired {
			return []string{
				"Silent refresh: available (will refresh on next API call)",
				fmt.Sprintf("Auth base URL: %s", styles.Bold.Render(token.AuthBaseURL)),
			}
		}

		return []string{
			"Silent refresh: available",
			fmt.Sprintf("Auth base URL: %s", styles.Bold.Render(token.AuthBaseURL)),
		}
	}

	lines := []string{"Silent refresh: unavailable"}
	if expired {
		lines = append(lines, "Action: run 'port auth login' to renew the token")
	}
	return lines
}

func loginWithStdinToken(configManager *config.ConfigManager, org string) error {
	token, err := auth.ReadTokenFromStdin()
	if err != nil {
		return fmt.Errorf("failed reading token (%w)", err)
	}

	parsed, err := auth.ParseToken(token)
	if err != nil {
		return fmt.Errorf("failed parsing token (%w)", err)
	}

	err = configManager.StoreToken(org, parsed)
	if err != nil {
		return fmt.Errorf("failed storing token (%w)", err)
	}

	lipgloss.Printf("%s Using provided token\n", styles.CheckMark)

	return err
}
