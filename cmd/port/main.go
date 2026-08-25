package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/commands"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

var (
	version   = "0.2.0"
	buildDate = "unknown"
	commit    = "unknown"
)

func init() {
	fmt.Print("test 2")
	// Try to get build info from runtime
	if info, ok := debug.ReadBuildInfo(); ok {
		if version == "dev" {
			// Try to get version from build info
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" && commit == "unknown" {
					commit = setting.Value
					if len(commit) > 7 {
						commit = commit[:7]
					}
				}
				if setting.Key == "vcs.time" && buildDate == "unknown" {
					buildDate = setting.Value
				}
			}
		}
	}

	// Set build info in commands package
	commands.SetBuildInfo(commands.BuildInfo{
		Version:   version,
		BuildDate: buildDate,
		Commit:    commit,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	})
}

//go:embed logo.txt
var logo string

func main() {
	rootCmd := &cobra.Command{
		Use:   "port",
		Short: "Port CLI - Modular command-line interface for Port",
		Long: lipgloss.JoinHorizontal(
			lipgloss.Center,
			logo,
			lipgloss.NewStyle().PaddingLeft(2).Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					styles.Bold.Render("Port CLI\n"),
					lipgloss.NewStyle().Faint(true).Render("Modular command-line interface for Port")),
			),
		) + "\n\n" +
			`Manage your Port organization with import/export, migration, and API operations.

Credentials can be provided via:
  1. By calling port auth login
  2. CLI flags (--client-id, --client-secret) - highest priority
  3. Environment variables (PORT_CLIENT_ID, PORT_CLIENT_SECRET)
  4. Configuration file (~/.port/config.yaml)`,
		Version: version,
	}

	// Global flags
	var (
		configFile         string
		clientID           string
		clientSecret       string
		apiURL             string
		targetClientID     string
		targetClientSecret string
		targetAPIURL       string
		debug              bool
		noColor            bool
		jsonErrors         bool
		quiet              bool
		verbose            bool
		yes                bool
		noEnvFile          bool
	)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&clientID, "client-id", "", "Base org Port API client ID (overrides config/env)")
	rootCmd.PersistentFlags().StringVar(&clientSecret, "client-secret", "", "Base org Port API client secret (overrides config/env)")
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Base org Port API URL (overrides config/env)")
	rootCmd.PersistentFlags().StringVar(&targetClientID, "target-client-id", "", "Target org Port API client ID (overrides config/env)")
	rootCmd.PersistentFlags().StringVar(&targetClientSecret, "target-client-secret", "", "Target org Port API client secret (overrides config/env)")
	rootCmd.PersistentFlags().StringVar(&targetAPIURL, "target-api-url", "", "Target org Port API URL (overrides config/env)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug mode")
	rootCmd.PersistentFlags().MarkHidden("debug")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().BoolVar(&jsonErrors, "json-errors", false, "Print command errors as structured JSON")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.PersistentFlags().BoolVar(&noEnvFile, "no-env-file", false, "Do not load .env files from the current directory or ~/.port")
	rootCmd.PersistentFlags().Bool(commands.TreeFlagName, false, "Print the full command tree for this command and exit")

	// Store global flags in context and initialize color output
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Initialize color output early
		output.Init(noColor)

		// Initialize verbosity
		if noEnvFile {
			os.Setenv("PORT_NO_ENV_FILE", "1")
		}

		if quiet {
			output.SetVerbosity(output.QuietLevel)
		} else if verbose {
			output.SetVerbosity(output.VerboseLevel)
		} else {
			output.SetVerbosity(output.NormalLevel)
		}

		cmd.SetContext(commands.WithGlobalFlags(cmd.Context(), commands.GlobalFlags{
			ConfigFile:         configFile,
			ClientID:           clientID,
			ClientSecret:       clientSecret,
			APIURL:             apiURL,
			TargetClientID:     targetClientID,
			TargetClientSecret: targetClientSecret,
			TargetAPIURL:       targetAPIURL,
			Debug:              debug,
			NoColor:            noColor,
			Quiet:              quiet,
			Verbose:            verbose,
			Yes:                yes,
			NoEnvFile:          noEnvFile,
		}))
	}

	// Add subcommands
	commands.RegisterAuth(rootCmd)
	commands.RegisterExport(rootCmd)
	commands.RegisterImport(rootCmd)
	commands.RegisterClear(rootCmd)
	commands.RegisterMigrate(rootCmd)
	commands.RegisterCompare(rootCmd)
	commands.RegisterAPI(rootCmd)
	commands.RegisterVersion(rootCmd)
	commands.RegisterConfig(rootCmd)
	commands.RegisterCompletion(rootCmd)
	commands.RegisterDocs(rootCmd)
	commands.RegisterSkills(rootCmd)
	commands.RegisterCache(rootCmd)

	showTargetFlags := len(os.Args) > 1 && (os.Args[1] == "import" || os.Args[1] == "migrate" || os.Args[1] == "compare")
	for _, name := range []string{"target-client-id", "target-client-secret", "target-api-url"} {
		if flag := rootCmd.PersistentFlags().Lookup(name); flag != nil {
			flag.Hidden = !showTargetFlags
		}
	}

	if commands.HasTreeFlag(os.Args[1:]) {
		target := commands.ResolveTreeTarget(rootCmd, os.Args[1:])
		commands.PrintCommandTree(os.Stdout, target)
		return
	}

	themeFunc := fang.WithColorSchemeFunc(func(
		ld lipgloss.LightDarkFunc,
	) fang.ColorScheme {
		def := fang.DefaultColorScheme(ld)
		def.DimmedArgument = ld(lipgloss.Black, lipgloss.White)
		def.Codeblock = ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#1E1C25"))
		def.Title = lipgloss.Color("#3BB3F6")
		def.Command = lipgloss.Color("#3BB3F6")
		def.Program = ld(lipgloss.Color("#1E1C25"), lipgloss.Color("#FFFFFF"))
		def.Flag = ld(lipgloss.Color("#1E1C25"), lipgloss.Color("#FFFFFF"))
		return def
	})

	errorHandler := fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
		if jsonErrors {
			return
		}
		fang.DefaultErrorHandler(w, styles, err)
	})

	if err := fang.Execute(
		context.Background(),
		rootCmd,
		themeFunc,
		errorHandler,
		fang.WithVersion(version),
		fang.WithCommit(commit),
		fang.WithNotifySignal(os.Interrupt)); err != nil {
		output.Init(noColor)
		output.SetVerbosity(output.NormalLevel)
		if jsonErrors {
			_ = output.PrintJSONError(err)
			os.Exit(1)
		}
		formattedErr := output.FormatError(err)
		if formattedErr != "" {
			output.ErrorPrintf("%s\n", formattedErr)
		} else {
			output.ErrorPrintf("%s: %v\n", output.Error("Error"), err)
		}
		os.Exit(1)
	}
}
