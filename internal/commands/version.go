package commands

import (
	"fmt"
	"runtime"

	"github.com/port-labs/port-cli/internal/output"
	"github.com/port-labs/port-cli/internal/update"
	"github.com/port-labs/port-cli/internal/useragent"
	"github.com/spf13/cobra"
)

// BuildInfo holds build-time information
type BuildInfo struct {
	Version   string
	BuildDate string
	Commit    string
	GoVersion string
	Platform  string
}

var buildInfo = BuildInfo{
	Version:   "dev",
	BuildDate: "unknown",
	Commit:    "unknown",
	GoVersion: runtime.Version(),
	Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
}

// SetBuildInfo sets build information (called from main.go)
func SetBuildInfo(info BuildInfo) {
	buildInfo = info
	useragent.SetVersion(info.Version)
}

// RegisterVersion registers the version command.
func RegisterVersion(rootCmd *cobra.Command) {
	var check bool

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show the CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			if GetGlobalFlags(cmd.Context()).Quiet {
				output.QuietPrint("%s\n", buildInfo.Version)
			} else {
				banner := output.Banner(output.VersionInfo{
					Version:   buildInfo.Version,
					BuildDate: buildInfo.BuildDate,
					Commit:    buildInfo.Commit,
					GoVersion: buildInfo.GoVersion,
					Platform:  buildInfo.Platform,
				})
				output.Println(banner)
			}

			if check {
				output.Printf("\nChecking for updates...\n")
				checker := update.NewChecker()
				result, err := checker.CheckLatestVersion(cmd.Context(), buildInfo.Version)
				if err != nil {
					output.WarningPrintf("Failed to check for updates: %v\n", err)
					return
				}

				if result.UpdateAvailable {
					output.Printf("\n%s A new version is available!\n", output.Warning("⚠"))
					output.Printf("Current version: %s\n", buildInfo.Version)
					output.Printf("Latest version: %s\n", output.Success(result.LatestVersion))
					output.Printf("Download: %s\n", result.DownloadURL)
				} else {
					output.Printf("\n%s You are running the latest version.\n", output.Success("✓"))
				}
			}
		},
	}

	versionCmd.Flags().BoolVar(&check, "check", false, "Check for updates")

	rootCmd.AddCommand(versionCmd)
}
