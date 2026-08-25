package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncHookCommand_EmptyOrg(t *testing.T) {
	got := SyncHookCommand("")
	if got != hookCommandBase {
		t.Fatalf("SyncHookCommand(\"\") = %q, want %q", got, hookCommandBase)
	}
}

func TestSyncHookCommand_PinsOrg(t *testing.T) {
	got := SyncHookCommand("production")
	want := "port skills sync --quiet --org production"
	if got != want {
		t.Fatalf("SyncHookCommand = %q, want %q", got, want)
	}
}

func TestSyncHookCommand_EscapesUnsafeOrg(t *testing.T) {
	got := SyncHookCommand("acme corp")
	want := "port skills sync --quiet --org 'acme corp'"
	if got != want {
		t.Fatalf("SyncHookCommand = %q, want %q", got, want)
	}
}

func TestIsPortCommand_RecognizesOrgPinnedHook(t *testing.T) {
	if !isPortCommand("port skills sync --quiet --org production") {
		t.Fatal("org-pinned hook should be recognized as Port command")
	}
}

func TestInstallHooks_BakesOrgIntoCommand(t *testing.T) {
	dir := t.TempDir()
	targets := []HookTarget{{Name: "Cursor", Dir: "cursor", Format: hookFormatJSON}}
	if err := InstallHooks(targets, dir, dir, "demo"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := SyncHookCommand("demo")
	if !containsStr(string(body), want) {
		t.Fatalf("hooks.json missing %q\nbody:\n%s", want, body)
	}
	if containsStr(string(body), `"port skills sync --quiet"`) && !containsStr(string(body), "--org demo") {
		t.Fatal("expected org-pinned hook, found unpinned command")
	}
}
