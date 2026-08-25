package output

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const asciiLogo = ` ██████╗   ██████╗  ██████╗  ████████╗
 ██╔══██╗ ██╔═══██╗ ██╔══██╗ ╚══██╔══╝
 ██████╔╝ ██║   ██║ ██████╔╝    ██║
 ██╔═══╝  ██║   ██║ ██╔══██╗    ██║
 ██║      ╚██████╔╝ ██║  ██║    ██║
 ╚═╝       ╚═════╝  ╚═╝  ╚═╝    ╚═╝   `

// VersionInfo holds the fields displayed below the banner.
type VersionInfo struct {
	Version   string
	BuildDate string
	Commit    string
	GoVersion string
	Platform  string
}

// Banner renders a branded ASCII art banner with version information.
// It uses Lipgloss for styling and respects the existing NO_COLOR / TTY
// detection handled by the output package.
func Banner(info VersionInfo) string {
	// -- styles (only apply colors when color output is enabled) --
	logoStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()
	subtitleStyle := lipgloss.NewStyle()
	dimStyle := lipgloss.NewStyle()

	if Enabled() {
		logoStyle = logoStyle.Bold(true).Foreground(lipgloss.White)
		titleStyle = titleStyle.Bold(true).Foreground(lipgloss.White)
		subtitleStyle = subtitleStyle.Foreground(lipgloss.Color("33")) // blue
		dimStyle = dimStyle.Foreground(lipgloss.Color("245"))          // gray / dim
	}

	// -- logo --
	logo := logoStyle.Render(asciiLogo)

	// -- tagline --
	tagline := fmt.Sprintf("%s  %s  %s",
		titleStyle.Render("Port CLI"),
		dimStyle.Render("\u00b7"),
		subtitleStyle.Render("Agentic Engineering Platform"),
	)

	// -- overall width --
	// The tagline can be wider than the logo. Constraining the outer
	// container to the logo width alone would force the wider lines (and the
	// padding JoinVertical adds to match them) to wrap, shattering the ASCII
	// art. Use the widest piece of content so nothing wraps.
	contentWidth := lipgloss.Width(logo)
	if w := lipgloss.Width(tagline); w > contentWidth {
		contentWidth = w
	}

	// -- separator (thin line spanning the content width) --
	separator := dimStyle.Render(strings.Repeat("─", contentWidth))

	// -- version details --
	versionLines := []string{
		fmt.Sprintf("Version:    %s", info.Version),
		fmt.Sprintf("Build date: %s", info.BuildDate),
		fmt.Sprintf("Git commit: %s", info.Commit),
		fmt.Sprintf("Go version: %s", info.GoVersion),
		fmt.Sprintf("Platform:   %s", info.Platform),
	}
	versionBlock := dimStyle.Render(strings.Join(versionLines, "\n"))

	// -- assemble & center --
	inner := lipgloss.JoinVertical(lipgloss.Center,
		logo,
		"",
		tagline,
		separator,
		versionBlock,
	)

	centered := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(contentWidth).
		Render(inner)

	return centered
}
