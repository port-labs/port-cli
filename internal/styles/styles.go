package styles

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

var (
	Cross            = lipgloss.NewStyle().Foreground(lipgloss.Red).Render("✖︎")
	CheckMark        = lipgloss.NewStyle().Foreground(lipgloss.Green).Render("✔︎")
	QuestionMark     = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Render("?")
	ExclamationMark  = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Render("!")
	Circle           = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("○")
	Bold             = lipgloss.NewStyle().Bold(true)
	Faint            = lipgloss.NewStyle().Faint(true)
	GlobalLabel      = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true).Render("global")
	ProjectLabel     = lipgloss.NewStyle().Foreground(lipgloss.Magenta).Bold(true).Render("project")
	CopilotRepoLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render("GitHub Copilot (repo)")
)

// FormTheme implements huh.Theme using the base theme.
type FormTheme struct{}

func (t *FormTheme) Theme(isDark bool) *huh.Styles {
	return huh.ThemeBase(isDark)
}
