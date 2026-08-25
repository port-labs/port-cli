package commands

import (
	"testing"

	exportmodule "github.com/port-labs/port-cli/internal/modules/export"
	"github.com/spf13/cobra"
)

func TestExportSystemBlueprintPropertiesFlag(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterExport(rootCmd)

	exportCmd, _, err := rootCmd.Find([]string{"export"})
	if err != nil || exportCmd == nil {
		t.Fatal("export command not found")
	}

	flag := exportCmd.Flags().Lookup("skip-system-blueprint-properties")
	if flag == nil {
		t.Fatal("flag --skip-system-blueprint-properties not registered on export command")
	}

	value, err := exportCmd.Flags().GetBool("skip-system-blueprint-properties")
	if err != nil {
		t.Fatalf("could not get --skip-system-blueprint-properties: %v", err)
	}
	if value {
		t.Fatal("expected --skip-system-blueprint-properties default to be false")
	}
}

func TestExportMaxErrorsFlag(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterExport(rootCmd)

	exportCmd, _, err := rootCmd.Find([]string{"export"})
	if err != nil || exportCmd == nil {
		t.Fatal("export command not found")
	}

	flag := exportCmd.Flags().Lookup("max-errors")
	if flag == nil {
		t.Fatal("flag --max-errors not registered on export command")
	}

	value, err := exportCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if value != defaultMaxErrors {
		t.Fatalf("expected --max-errors default to be %d, got %d", defaultMaxErrors, value)
	}
}

func TestExportJSONSummaryIncludesP1Fields(t *testing.T) {
	result := &exportmodule.Result{
		Success:           true,
		Message:           "ok",
		OutputPath:        "backup.tar.gz",
		Format:            "tar",
		BlueprintsCount:   1,
		EntitiesCount:     2,
		ActionsCount:      3,
		UsersCount:        4,
		TeamsCount:        5,
		FoldersCount:      6,
		PagesCount:        7,
		IntegrationsCount: 8,
	}
	data := exportJSONSummary(result, exportJSONSummaryOptions{
		SkipEntities:             true,
		IncludedResources:        []string{"blueprints"},
		ExcludedBlueprints:       []string{"legacy"},
		SchemaExcludedBlueprints: []string{"schema-only"},
	})

	checks := map[string]interface{}{
		"format":             "tar",
		"blueprints_count":   1,
		"entities_count":     2,
		"actions_count":      3,
		"users_count":        4,
		"teams_count":        5,
		"folders_count":      6,
		"pages_count":        7,
		"integrations_count": 8,
		"skipped_entities":   true,
	}
	for key, want := range checks {
		if got := data[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestExportMaxErrorsFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterExport(rootCmd)

	exportCmd, _, _ := rootCmd.Find([]string{"export"})
	if exportCmd == nil {
		t.Fatal("export command not found")
	}

	err := exportCmd.ParseFlags([]string{
		"--max-errors", "-1",
		"--output", "dummy.json",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	value, err := exportCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if value != hideAllErrors {
		t.Fatalf("expected --max-errors to parse as -1, got %d", value)
	}
}
