package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/port-labs/port-cli/internal/styles"
)

func printLoadResult(result *skills.LoadSkillsResult) {
	fmt.Fprintf(os.Stderr,
		"%s %d skill group(s), %d skill(s) synced\n",
		styles.CheckMark,
		result.GroupCount,
		result.SkillCount,
	)

	if len(result.TargetResults) == 0 {
		printLoadWarnings(result.Warnings)
		return
	}

	var globalTargets, projectTargets, copilotRepoTargets []skills.TargetResult
	for _, t := range result.TargetResults {
		switch {
		case t.GitHubCopilotRepo:
			copilotRepoTargets = append(copilotRepoTargets, t)
		case t.IsProject:
			projectTargets = append(projectTargets, t)
		default:
			globalTargets = append(globalTargets, t)
		}
	}

	if len(globalTargets) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, t := range globalTargets {
			fmt.Fprintf(os.Stderr, "  %s %s/skills/  %s  %s\n",
				styles.Circle,
				t.Path,
				styles.GlobalLabel,
				styles.Faint.Render(fmt.Sprintf("%d skill group(s), %d skill(s)", t.GroupCount, t.SkillCount)),
			)
		}
	}

	if len(projectTargets) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, t := range projectTargets {
			fmt.Fprintf(os.Stderr, "  %s %s/skills/  %s  %s\n",
				styles.Circle,
				t.Path,
				styles.ProjectLabel,
				styles.Faint.Render(fmt.Sprintf("%d skill group(s), %d skill(s)", t.GroupCount, t.SkillCount)),
			)
		}
	}

	if len(copilotRepoTargets) > 0 {
		fmt.Fprintln(os.Stderr)
		for _, t := range copilotRepoTargets {
			fmt.Fprintf(os.Stderr, "  %s %s/skills/  %s  %s\n",
				styles.Circle,
				t.Path,
				styles.CopilotRepoLabel,
				styles.Faint.Render(fmt.Sprintf("%d skill group(s), %d skill(s) · not synced to a global directory", t.GroupCount, t.SkillCount)),
			)
		}
	}
	fmt.Fprintln(os.Stderr)
	printLoadWarnings(result.Warnings)
}

func printLoadWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "%s Warning: %s\n", styles.ExclamationMark, warning)
	}
}

func printSkillsStatus(status *skills.StatusResult) {
	fmt.Println("\nPort Skills Status")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("Last synced:     %s\n", valueOrNone(status.LastSyncedAt))
	fmt.Printf("\nHook targets (%d):\n", len(status.Targets))
	for _, t := range status.Targets {
		fmt.Printf("  - %s/skills/\n", t)
	}
	fmt.Printf("\nProject directories (%d):\n", len(status.ProjectDirs))
	if len(status.ProjectDirs) == 0 {
		fmt.Println("  (none)")
	}
	for _, d := range status.ProjectDirs {
		fmt.Printf("  - %s\n", d)
	}
	fmt.Printf("\nSkill selection:\n")
	if status.SelectAll {
		fmt.Println("  Groups:           all")
		fmt.Println("  Ungrouped skills: all")
	} else {
		if status.SelectAllGroups {
			fmt.Println("  Groups:           all")
		} else if status.TeamGroupDefaults {
			fmt.Println("  Groups:           team defaults (groups owned by your teams)")
			if len(status.IncludeGroups) > 0 {
				fmt.Printf("    extra includes (%d):\n", len(status.IncludeGroups))
				for _, g := range status.IncludeGroups {
					fmt.Printf("      + %s\n", g)
				}
			}
			if len(status.ExcludeGroups) > 0 {
				fmt.Printf("    excluded (%d):\n", len(status.ExcludeGroups))
				for _, g := range status.ExcludeGroups {
					fmt.Printf("      - %s\n", g)
				}
			}
		} else {
			fmt.Printf("  Groups (%d):\n", len(status.SelectedGroups))
			if len(status.SelectedGroups) == 0 {
				fmt.Println("    (none)")
			}
			for _, g := range status.SelectedGroups {
				fmt.Printf("    - %s\n", g)
			}
		}
		if status.SelectAllUngrouped {
			fmt.Println("  Ungrouped skills: all")
		} else {
			fmt.Printf("  Ungrouped skills (%d):\n", len(status.SelectedSkills))
			if len(status.SelectedSkills) == 0 {
				fmt.Println("    (none)")
			}
			for _, s := range status.SelectedSkills {
				fmt.Printf("    - %s\n", s)
			}
		}
	}
}

func printSkillsSearchResults(entries []api.SkillCatalogEntry, query string) {
	fmt.Printf("%s %d skill(s) matching %q:\n\n", styles.CheckMark, len(entries), query)
	for _, entry := range entries {
		s := entry.Skill
		title := strings.TrimSpace(s.Title)
		if title == "" || title == s.Identifier {
			fmt.Printf("  %s\n", styles.Bold.Render(s.Identifier))
		} else {
			fmt.Printf("  %s  %s\n", styles.Bold.Render(s.Identifier), title)
		}
		if loc := catalogPropString(s.Properties, "location"); loc != "" {
			fmt.Printf("    %s\n", styles.Faint.Render("location: "+loc))
		}
		if entry.Version != nil {
			if v := catalogPropString(entry.Version.Properties, "version"); v != "" {
				fmt.Printf("    %s\n", styles.Faint.Render("version: "+v))
			}
		}
	}
}

func catalogPropString(props map[string]interface{}, key string) string {
	if props == nil {
		return ""
	}
	raw, ok := props[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func printSkillsCatalogJSON(resp *api.SkillsSummaryResponse) error {
	if resp == nil {
		resp = &api.SkillsSummaryResponse{OK: true}
	}
	payload := *resp
	payload.OK = true
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printGroupedSkillsPreview(resp *api.GroupedSkillsResponse) {
	printGroupSkills := func(skills []api.SkillAtLatestVersion) {
		for _, s := range skills {
			title := strings.TrimSpace(s.Title)
			if title != "" && title != s.Identifier {
				fmt.Printf("  %s  %s\n", styles.Bold.Render(s.Identifier), title)
			} else {
				fmt.Printf("  %s\n", styles.Bold.Render(s.Identifier))
			}
			if s.Location != "" {
				fmt.Printf("    %s\n", styles.Faint.Render("location: "+s.Location))
			}
			version := strings.TrimSpace(s.Version)
			if version == "" {
				fmt.Printf("    %s\n", styles.Faint.Render("version: (none)"))
			} else {
				fmt.Printf("    %s\n", styles.Faint.Render("version: "+version))
			}
		}
	}

	for _, g := range resp.Groups {
		groupLabel := g.Identifier
		if t := strings.TrimSpace(g.Title); t != "" && t != g.Identifier {
			groupLabel = t
		}
		fmt.Printf("\n%s\n", styles.Bold.Render(groupLabel))
		printGroupSkills(g.Skills)
	}
	if len(resp.UngroupedSkills) > 0 {
		fmt.Printf("\n%s\n", styles.Bold.Render("Ungrouped"))
		printGroupSkills(resp.UngroupedSkills)
	}
}

func printGroupedSkillsPreviewJSON(resp *api.GroupedSkillsResponse) error {
	if resp == nil {
		resp = &api.GroupedSkillsResponse{OK: true}
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
