package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillsInit_InstallHooksFlagDefaultsFalse(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	initCmd, _, err := root.Find([]string{"skills", "init"})
	if err != nil || initCmd == nil {
		t.Fatal("skills init command not found")
	}

	installHooks, err := initCmd.Flags().GetBool("install-hooks")
	if err != nil {
		t.Fatalf("install-hooks flag: %v", err)
	}
	if installHooks {
		t.Fatal("install-hooks should default to false")
	}
}

func TestSkillsInit_InstallHooksFlagParsesTrue(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	initCmd, _, err := root.Find([]string{"skills", "init"})
	if err != nil || initCmd == nil {
		t.Fatal("skills init command not found")
	}
	if err := initCmd.ParseFlags([]string{"--install-hooks", "--tool", "Cursor"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	installHooks, err := initCmd.Flags().GetBool("install-hooks")
	if err != nil {
		t.Fatalf("install-hooks flag: %v", err)
	}
	if !installHooks {
		t.Fatal("expected install-hooks true after --install-hooks")
	}
}
