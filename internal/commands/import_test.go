package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestImportExcludeFlags(t *testing.T) {
	// Build a root command and register import on it
	rootCmd := &cobra.Command{Use: "port"}
	RegisterImport(rootCmd)

	// Verify that the flags exist on the import subcommand
	importCmd, _, err := rootCmd.Find([]string{"import"})
	if err != nil || importCmd == nil {
		t.Fatal("import command not found")
	}

	tests := []struct {
		name string
		flag string
	}{
		{"exclude-blueprints flag exists", "exclude-blueprints"},
		{"exclude-blueprint-schema flag exists", "exclude-blueprint-schema"},
		{"skip-system-blueprint-properties flag exists", "skip-system-blueprint-properties"},
		{"max-errors flag exists", "max-errors"},
		{"on-error flag exists", "on-error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := importCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Errorf("flag --%s not registered on import command", tt.flag)
			}
		})
	}
}

func TestImportOnErrorFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterImport(rootCmd)

	importCmd, _, _ := rootCmd.Find([]string{"import"})
	if importCmd == nil {
		t.Fatal("import command not found")
	}

	err := importCmd.ParseFlags([]string{
		"--on-error", "forbidden_format_change=ignore-property",
		"--on-error", "forbidden_format_change=recreate-property",
		"--input", "dummy.tar.gz",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}
	values, err := importCmd.Flags().GetStringArray("on-error")
	if err != nil {
		t.Fatalf("could not get --on-error: %v", err)
	}
	if len(values) != 2 || values[0] != "forbidden_format_change=ignore-property" || values[1] != "forbidden_format_change=recreate-property" {
		t.Fatalf("unexpected --on-error values: %#v", values)
	}
}

func TestOnErrorPolicyValidationRejectsInvalidAction(t *testing.T) {
	_, err := buildErrorHandlingOptions(&cobra.Command{Use: "test"}, []string{"forbidden_format_change=delete-everything"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("expected unsupported action error, got %v", err)
	}
}

func TestImportExcludeFlagsParsed(t *testing.T) {
	// Verify that the flags are accepted without error at parse time (args parsing only)
	rootCmd := &cobra.Command{Use: "port"}
	RegisterImport(rootCmd)

	importCmd, _, _ := rootCmd.Find([]string{"import"})
	if importCmd == nil {
		t.Fatal("import command not found")
	}

	// Parse args without executing RunE
	importCmd.DisableFlagParsing = false
	err := importCmd.ParseFlags([]string{
		"--exclude-blueprints", "service,microservice",
		"--exclude-blueprint-schema", "region",
		"--input", "dummy.tar.gz",
	})
	if err != nil {
		t.Errorf("unexpected error parsing flags: %v", err)
	}

	eb, err := importCmd.Flags().GetString("exclude-blueprints")
	if err != nil {
		t.Fatalf("could not get --exclude-blueprints: %v", err)
	}
	if eb != "service,microservice" {
		t.Errorf("expected 'service,microservice', got %q", eb)
	}

	ebs, err := importCmd.Flags().GetString("exclude-blueprint-schema")
	if err != nil {
		t.Fatalf("could not get --exclude-blueprint-schema: %v", err)
	}
	if ebs != "region" {
		t.Errorf("expected 'region', got %q", ebs)
	}

	skipSystemBlueprintProperties, err := importCmd.Flags().GetBool("skip-system-blueprint-properties")
	if err != nil {
		t.Fatalf("could not get --skip-system-blueprint-properties: %v", err)
	}
	if skipSystemBlueprintProperties {
		t.Error("expected --skip-system-blueprint-properties default to be false")
	}

	maxErrors, err := importCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if maxErrors != defaultMaxErrors {
		t.Errorf("expected --max-errors default to be %d, got %d", defaultMaxErrors, maxErrors)
	}
}

func TestImportMaxErrorsFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterImport(rootCmd)

	importCmd, _, _ := rootCmd.Find([]string{"import"})
	if importCmd == nil {
		t.Fatal("import command not found")
	}

	err := importCmd.ParseFlags([]string{
		"--max-errors", "-1",
		"--input", "dummy.tar.gz",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	maxErrors, err := importCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if maxErrors != hideAllErrors {
		t.Errorf("expected --max-errors to parse as -1, got %d", maxErrors)
	}
}
