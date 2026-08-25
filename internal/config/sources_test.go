package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilesCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT_CLIENT_ID=from-dotenv\nPORT_CLIENT_SECRET=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT_CLIENT_ID", "")
	t.Setenv("PORT_CLIENT_SECRET", "")
	t.Setenv("PORT_NO_ENV_FILE", "1")

	_ = NewConfigManager(filepath.Join(dir, "config.yaml"))
	if got := os.Getenv("PORT_CLIENT_ID"); got != "" {
		t.Fatalf("expected .env not to load, got PORT_CLIENT_ID=%q", got)
	}
}

func TestEnvFileSourcesReportsCurrentDirectoryEnv(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT_CLIENT_ID=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sources := EnvFileSources()
	if len(sources) == 0 || sources[0].Path != ".env" || !sources[0].Exists {
		t.Fatalf("expected first source to be present .env, got %#v", sources)
	}
}
