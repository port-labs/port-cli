package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillsRemove_CommandRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	removeCmd, _, err := root.Find([]string{"skills", "remove"})
	if err != nil || removeCmd == nil {
		t.Fatal("skills remove command not found")
	}
}

func TestSkillsRemove_FlagsRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	removeCmd, _, err := root.Find([]string{"skills", "remove"})
	if err != nil || removeCmd == nil {
		t.Fatal("skills remove command not found")
	}

	if err := removeCmd.ParseFlags([]string{"--group", "my-group", "--skill", "my-skill", "--tool", "Cursor"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	groups, _ := removeCmd.Flags().GetStringArray("group")
	if len(groups) != 1 || groups[0] != "my-group" {
		t.Errorf("group flag: got %v", groups)
	}
	skills, _ := removeCmd.Flags().GetStringArray("skill")
	if len(skills) != 1 || skills[0] != "my-skill" {
		t.Errorf("skill flag: got %v", skills)
	}
	tools, _ := removeCmd.Flags().GetStringArray("tool")
	if len(tools) != 1 || tools[0] != "Cursor" {
		t.Errorf("tool flag: got %v", tools)
	}
}
