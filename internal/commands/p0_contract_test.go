package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestValidateStringEnumRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		value   string
		allowed []string
		want    string
	}{
		{name: "compare output", flag: "--output", value: "xml", allowed: []string{"text", "json", "html"}, want: "Valid values: text, json, html"},
		{name: "json text output", flag: "--output-format", value: "yaml", allowed: []string{"text", "json"}, want: "Valid values: text, json"},
		{name: "api format", flag: "--format", value: "text", allowed: []string{"json", "yaml"}, want: "Valid values: json, yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStringEnum(tt.flag, tt.value, tt.allowed)
			if err == nil {
				t.Fatal("expected invalid enum error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVersionQuietPrintsOnlyVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	output.SetWriters(&out, &errOut)
	output.SetVerbosity(output.QuietLevel)
	defer output.SetWriters(os.Stdout, os.Stderr)
	defer output.SetVerbosity(output.NormalLevel)

	root := &cobra.Command{Use: "port"}
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SetContext(WithGlobalFlags(cmd.Context(), GlobalFlags{Quiet: true}))
	}
	RegisterVersion(root)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	got := out.String()
	if got != buildInfo.Version+"\n" {
		t.Fatalf("expected only version %q, got %q", buildInfo.Version+"\n", got)
	}
}

func TestDefaultConfigPathUsesPortConfigFileEnv(t *testing.T) {
	t.Setenv("PORT_CONFIG_FILE", "/tmp/custom-port-config.yaml")
	if got := config.DefaultConfigPath(); got != "/tmp/custom-port-config.yaml" {
		t.Fatalf("expected PORT_CONFIG_FILE path, got %q", got)
	}
}

func TestConfigWritesUsePrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows file mode semantics differ")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	manager := config.NewConfigManager(path)
	if err := manager.Write(&config.Config{Organizations: map[string]config.OrganizationConfig{}}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", got)
	}
}
