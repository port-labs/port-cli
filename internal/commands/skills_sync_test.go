package commands

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// printLoadResult — output routing
// ---------------------------------------------------------------------------

func TestPrintLoadResult_WritesToStderr(t *testing.T) {
	result := &skills.LoadSkillsResult{
		SkillCount: 4,
		TargetResults: []skills.TargetResult{
			{Path: "/home/user/.cursor", SkillCount: 4, IsProject: false},
		},
	}

	// Capture stderr.
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = stderrW

	// Capture stdout to make sure nothing leaks there.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = stdoutW

	printLoadResult(result)

	stdoutW.Close()
	os.Stdout = origStdout
	stderrW.Close()
	os.Stderr = origStderr

	var stderrBuf, stdoutBuf bytes.Buffer
	io.Copy(&stderrBuf, stderrR)
	io.Copy(&stdoutBuf, stdoutR)

	if stderrBuf.Len() == 0 {
		t.Error("expected printLoadResult to write to stderr, but stderr was empty")
	}
	if stdoutBuf.Len() != 0 {
		t.Errorf("expected nothing on stdout, got: %q", stdoutBuf.String())
	}
}

func TestPrintLoadResult_GitHubCopilotRepoRow(t *testing.T) {
	result := &skills.LoadSkillsResult{
		SkillCount: 36,
		TargetResults: []skills.TargetResult{
			{
				Path:              "/repo/.github",
				SkillCount:        36,
				GitHubCopilotRepo: true,
			},
		},
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	printLoadResult(result)
	w.Close()
	os.Stderr = orig

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("not synced to a global directory")) {
		t.Errorf("expected global-sync disclaimer, got: %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("GitHub Copilot")) {
		t.Errorf("expected GitHub Copilot repo label, got: %q", out)
	}
}

func TestPrintLoadResult_IncludesSkillCount(t *testing.T) {
	result := &skills.LoadSkillsResult{
		GroupCount: 2,
		SkillCount: 7,
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	printLoadResult(result)

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("2 skill group(s), 7 skill(s) synced")) {
		t.Errorf("expected skill count in output, got: %q", output)
	}
}

func TestPrintLoadResult_IncludesTargetGroupAndSkillCounts(t *testing.T) {
	result := &skills.LoadSkillsResult{
		GroupCount: 1,
		SkillCount: 3,
		TargetResults: []skills.TargetResult{
			{Path: "/home/user/.cursor", GroupCount: 1, SkillCount: 3, IsProject: false},
		},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	printLoadResult(result)

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("1 skill group(s), 3 skill(s)")) {
		t.Errorf("expected target group and skill counts in output, got: %q", output)
	}
}

func TestPrintLoadResult_IncludesWarnings(t *testing.T) {
	result := &skills.LoadSkillsResult{
		SkillCount: 1,
		Warnings:   []string{"could not update .gitignore"},
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	printLoadResult(result)

	w.Close()
	os.Stderr = origStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Warning")) {
		t.Errorf("expected warning label in output, got: %q", output)
	}
	if !bytes.Contains([]byte(output), []byte("could not update .gitignore")) {
		t.Errorf("expected warning detail in output, got: %q", output)
	}
}

// ---------------------------------------------------------------------------
// sync flags
// ---------------------------------------------------------------------------

func TestSkillsSync_QuietFlagRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	syncCmd, _, err := root.Find([]string{"skills", "sync"})
	if err != nil || syncCmd == nil {
		t.Fatal("skills sync command not found")
	}

	if err := syncCmd.ParseFlags([]string{"--quiet"}); err != nil {
		t.Fatalf("failed to parse --quiet flag: %v", err)
	}

	quiet, err := syncCmd.Flags().GetBool("quiet")
	if err != nil {
		t.Fatalf("could not get --quiet flag: %v", err)
	}
	if !quiet {
		t.Error("expected --quiet to be true after parsing")
	}
}

func TestSkillsSync_CatalogFlagsRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	syncCmd, _, err := root.Find([]string{"skills", "sync"})
	if err != nil || syncCmd == nil {
		t.Fatal("skills sync command not found")
	}

	for _, flag := range []string{"exclude-legacy", "include-internal"} {
		if syncCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("flag --%s not registered", flag)
		}
	}

	if err := syncCmd.ParseFlags([]string{"--exclude-legacy", "--include-internal"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	legacy, _ := syncCmd.Flags().GetBool("exclude-legacy")
	internal, _ := syncCmd.Flags().GetBool("include-internal")
	if !legacy || !internal {
		t.Fatalf("exclude-legacy=%v include-internal=%v", legacy, internal)
	}
}

func TestSkillsSync_NoGitignoreFlagRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	syncCmd, _, err := root.Find([]string{"skills", "sync"})
	if err != nil || syncCmd == nil {
		t.Fatal("skills sync command not found")
	}

	if syncCmd.Flags().Lookup("no-gitignore") == nil {
		t.Fatal("flag --no-gitignore not registered")
	}
	if err := syncCmd.ParseFlags([]string{"--no-gitignore"}); err != nil {
		t.Fatalf("failed to parse --no-gitignore flag: %v", err)
	}

	noGitignore, err := syncCmd.Flags().GetBool("no-gitignore")
	if err != nil {
		t.Fatalf("could not get --no-gitignore flag: %v", err)
	}
	if !noGitignore {
		t.Error("expected --no-gitignore to be true after parsing")
	}
}

func TestSkillsSync_InitSelectionFlagsRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	syncCmd, _, err := root.Find([]string{"skills", "sync"})
	if err != nil || syncCmd == nil {
		t.Fatal("skills sync command not found")
	}

	for _, flag := range []string{"tool", "install-hooks", "group", "skill", "select-all-groups", "select-all-ungrouped"} {
		if syncCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("flag --%s not registered", flag)
		}
	}

	if err := syncCmd.ParseFlags([]string{
		"--tool", "Cursor",
		"--group", "platform",
		"--skill", "standalone",
		"--select-all-ungrouped",
	}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	tools, _ := syncCmd.Flags().GetStringArray("tool")
	groups, _ := syncCmd.Flags().GetStringArray("group")
	skillIDs, _ := syncCmd.Flags().GetStringArray("skill")
	allUngrouped, _ := syncCmd.Flags().GetBool("select-all-ungrouped")
	if len(tools) != 1 || tools[0] != "Cursor" {
		t.Fatalf("tool flag: %v", tools)
	}
	if len(groups) != 1 || groups[0] != "platform" {
		t.Fatalf("group flag: %v", groups)
	}
	if len(skillIDs) != 1 || skillIDs[0] != "standalone" {
		t.Fatalf("skill flag: %v", skillIDs)
	}
	if !allUngrouped {
		t.Fatal("select-all-ungrouped flag should parse true")
	}
}

func TestSkillsSync_QuietShorthandRegistered(t *testing.T) {
	root := &cobra.Command{Use: "port"}
	RegisterSkills(root)

	syncCmd, _, err := root.Find([]string{"skills", "sync"})
	if err != nil || syncCmd == nil {
		t.Fatal("skills sync command not found")
	}

	if err := syncCmd.ParseFlags([]string{"-q"}); err != nil {
		t.Fatalf("failed to parse -q flag: %v", err)
	}

	quiet, err := syncCmd.Flags().GetBool("quiet")
	if err != nil {
		t.Fatalf("could not get --quiet via -q: %v", err)
	}
	if !quiet {
		t.Error("expected --quiet to be true after parsing -q")
	}
}
