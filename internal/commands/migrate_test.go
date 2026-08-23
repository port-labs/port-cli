package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/modules/migrate"
	"github.com/spf13/cobra"
)

func TestMigrateExcludeFlags(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	// Verify that the flags exist on the migrate subcommand
	migrateCmd, _, err := rootCmd.Find([]string{"migrate"})
	if err != nil || migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	tests := []struct {
		name string
		flag string
	}{
		{"exclude-blueprints flag exists", "exclude-blueprints"},
		{"exclude-blueprint-schema flag exists", "exclude-blueprint-schema"},
		{"integrations flag exists", "integrations"},
		{"entities flag exists", "entities"},
		{"actions flag exists", "actions"},
		{"scorecards flag exists", "scorecards"},
		{"pages flag exists", "pages"},
		{"teams flag exists", "teams"},
		{"users flag exists", "users"},
		{"skip-system-blueprint-properties flag exists", "skip-system-blueprint-properties"},
		{"max-errors flag exists", "max-errors"},
		{"on-error flag exists", "on-error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := migrateCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Errorf("flag --%s not registered on migrate command", tt.flag)
			}
		})
	}
}

func TestMigrateOnErrorFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	migrateCmd, _, _ := rootCmd.Find([]string{"migrate"})
	if migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	err := migrateCmd.ParseFlags([]string{
		"--on-error", "forbidden_format_change=fail",
		"--target-org", "my-target",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}
	values, err := migrateCmd.Flags().GetStringArray("on-error")
	if err != nil {
		t.Fatalf("could not get --on-error: %v", err)
	}
	if len(values) != 1 || values[0] != "forbidden_format_change=fail" {
		t.Fatalf("unexpected --on-error values: %#v", values)
	}
}

func TestMigrateExcludeFlagsParsed(t *testing.T) {
	// Verify that the flags are accepted without error at parse time (args parsing only)
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	migrateCmd, _, _ := rootCmd.Find([]string{"migrate"})
	if migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	// Parse args without executing RunE
	migrateCmd.DisableFlagParsing = false
	err := migrateCmd.ParseFlags([]string{
		"--exclude-blueprints", "service,microservice",
		"--exclude-blueprint-schema", "region",
		"--target-org", "my-target",
	})
	if err != nil {
		t.Errorf("unexpected error parsing flags: %v", err)
	}

	eb, err := migrateCmd.Flags().GetString("exclude-blueprints")
	if err != nil {
		t.Fatalf("could not get --exclude-blueprints: %v", err)
	}
	if eb != "service,microservice" {
		t.Errorf("expected 'service,microservice', got %q", eb)
	}

	ebs, err := migrateCmd.Flags().GetString("exclude-blueprint-schema")
	if err != nil {
		t.Fatalf("could not get --exclude-blueprint-schema: %v", err)
	}
	if ebs != "region" {
		t.Errorf("expected 'region', got %q", ebs)
	}

	skipSystemBlueprintProperties, err := migrateCmd.Flags().GetBool("skip-system-blueprint-properties")
	if err != nil {
		t.Fatalf("could not get --skip-system-blueprint-properties: %v", err)
	}
	if skipSystemBlueprintProperties {
		t.Error("expected --skip-system-blueprint-properties default to be false")
	}

	maxErrors, err := migrateCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if maxErrors != defaultMaxErrors {
		t.Errorf("expected --max-errors default to be %d, got %d", defaultMaxErrors, maxErrors)
	}
}

func TestMigrateMaxErrorsFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	migrateCmd, _, _ := rootCmd.Find([]string{"migrate"})
	if migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	err := migrateCmd.ParseFlags([]string{
		"--max-errors", "0",
		"--target-org", "my-target",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	maxErrors, err := migrateCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if maxErrors != 0 {
		t.Errorf("expected --max-errors to parse as 0, got %d", maxErrors)
	}
}

func TestMigrateMaxErrorsMinusOneFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	migrateCmd, _, _ := rootCmd.Find([]string{"migrate"})
	if migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	err := migrateCmd.ParseFlags([]string{
		"--max-errors", "-1",
		"--target-org", "my-target",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	maxErrors, err := migrateCmd.Flags().GetInt("max-errors")
	if err != nil {
		t.Fatalf("could not get --max-errors: %v", err)
	}
	if maxErrors != hideAllErrors {
		t.Errorf("expected --max-errors to parse as -1, got %d", maxErrors)
	}
}

func TestMigrateIntegrationsFlagParsed(t *testing.T) {
	rootCmd := &cobra.Command{Use: "port"}
	RegisterMigrate(rootCmd)

	migrateCmd, _, _ := rootCmd.Find([]string{"migrate"})
	if migrateCmd == nil {
		t.Fatal("migrate command not found")
	}

	err := migrateCmd.ParseFlags([]string{
		"--integrations", "int1,int2",
		"--target-org", "my-target",
	})
	if err != nil {
		t.Fatalf("unexpected error parsing flags: %v", err)
	}

	integrations, err := migrateCmd.Flags().GetString("integrations")
	if err != nil {
		t.Fatalf("could not get --integrations: %v", err)
	}
	if integrations != "int1,int2" {
		t.Errorf("expected 'int1,int2', got %q", integrations)
	}
	if !migrateCmd.Flags().Changed("integrations") {
		t.Error("expected integrations flag to be marked as changed")
	}
}

func TestMigrationFailureMessageIncludesResultErrors(t *testing.T) {
	msg := migrationFailureMessage(&migrate.Result{
		Message: "Migration completed with 2 error(s)",
		Errors: []string{
			"Entities service: failed to get current entities",
			"Blueprint component: relation target missing",
		},
	}, defaultMaxErrors)

	for _, want := range []string{
		"Migration completed with 2 error(s)",
		"Entities service: failed to get current entities",
		"Blueprint component: relation target missing",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected failure message to contain %q, got:\n%s", want, msg)
		}
	}
}

func TestMigrationFailureMessageWithoutErrorsIsGeneric(t *testing.T) {
	msg := migrationFailureMessage(&migrate.Result{
		Message: "Migration completed with 0 error(s)",
	}, defaultMaxErrors)
	if msg != "migration failed" {
		t.Fatalf("expected generic failure without result errors, got %q", msg)
	}
}

func TestMigrationFailureMessageLimitsErrors(t *testing.T) {
	msg := migrationFailureMessage(&migrate.Result{
		Message: "Migration completed with 6 error(s)",
		Errors: []string{
			"err-1",
			"err-2",
			"err-3",
			"err-4",
			"err-5",
			"err-6",
		},
	}, defaultMaxErrors)

	if strings.Contains(msg, "err-6") {
		t.Fatalf("expected message to omit sixth error, got:\n%s", msg)
	}
	if !strings.Contains(msg, "... and 1 more") {
		t.Fatalf("expected truncation message, got:\n%s", msg)
	}
}

func TestMigrationFailureMessageShowsAllErrorsWhenMaxErrorsZero(t *testing.T) {
	msg := migrationFailureMessage(&migrate.Result{
		Message: "Migration completed with 6 error(s)",
		Errors: []string{
			"err-1",
			"err-2",
			"err-3",
			"err-4",
			"err-5",
			"err-6",
		},
	}, 0)

	if !strings.Contains(msg, "err-6") {
		t.Fatalf("expected message to include sixth error, got:\n%s", msg)
	}
	if strings.Contains(msg, "... and") {
		t.Fatalf("expected no truncation message, got:\n%s", msg)
	}
}

func TestMigrationFailureMessageHidesErrorsWhenMaxErrorsMinusOne(t *testing.T) {
	msg := migrationFailureMessage(&migrate.Result{
		Message: "Migration completed with 2 error(s)",
		Errors: []string{
			"err-1",
			"err-2",
		},
	}, hideAllErrors)

	if msg != "Migration completed with 2 error(s)" {
		t.Fatalf("expected only summary message when errors are hidden, got:\n%s", msg)
	}
}

func TestMigrationExecutionErrorMessageIncludesCause(t *testing.T) {
	err := errors.New("failed to migrate entities: entities service: API request failed: 410 Gone")
	msg := migrationExecutionErrorMessage(err, nil, defaultMaxErrors)

	for _, want := range []string{
		"migration failed",
		"failed to migrate entities",
		"entities service",
		"410 Gone",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected execution error message to contain %q, got:\n%s", want, msg)
		}
	}
}

func TestMigrationExecutionErrorMessageNilIsGeneric(t *testing.T) {
	msg := migrationExecutionErrorMessage(nil, nil, defaultMaxErrors)
	if msg != "migration failed" {
		t.Fatalf("expected generic failure for nil error, got %q", msg)
	}
}

func TestMigrationExecutionErrorMessageUsesPartialResultErrors(t *testing.T) {
	err := errors.New("failed to migrate entities: entities service: API request failed: 410 Gone")
	msg := migrationExecutionErrorMessage(err, &migrate.Result{
		Message: "Migration stopped with 1 error(s)",
		Errors: []string{
			"Entities service: API request failed: 410 Gone",
		},
	}, defaultMaxErrors)

	for _, want := range []string{
		"Migration stopped with 1 error(s)",
		"Entities service",
		"410 Gone",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected execution error message to contain %q, got:\n%s", want, msg)
		}
	}
}
