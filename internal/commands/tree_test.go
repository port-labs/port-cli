package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWriteWrappedPreservesPrefix(t *testing.T) {
	var buf bytes.Buffer
	r := &treeRenderer{w: &buf, width: 60}

	head := "    │   │   ├── "
	cont := "    │   │   │   "
	body := "command with a description long enough to force wrapping over multiple lines"

	r.writeWrapped(head, cont, body)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output to span multiple lines, got %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], head) {
		t.Errorf("first line missing head prefix:\n%q", lines[0])
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, cont) {
			t.Errorf("continuation line missing cont prefix:\n%q", l)
		}
	}
}

func TestWriteFlagLinesPreservesPrefixOnWrap(t *testing.T) {
	var buf bytes.Buffer
	r := &treeRenderer{w: &buf, width: 70}

	indent := "    │   │       "
	flags := []flagLine{
		{name: "    --blueprint stringArray", usage: "Restrict --entities, --actions, --scorecards, and --blueprints to specific blueprint identifiers (repeatable)"},
	}
	r.writeFlagLines(flags, indent)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected long usage to wrap, got %d lines:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "    │   │       ") {
			t.Errorf("flag wrap broke prefix:\n%q", l)
		}
	}
}

func TestWriteWrappedShortLineUntouched(t *testing.T) {
	var buf bytes.Buffer
	r := &treeRenderer{w: &buf, width: 200}
	r.writeWrapped("    ├── ", "    │   ", "short — fits easily")
	got := buf.String()
	want := "    ├── short — fits easily\n"
	if got != want {
		t.Errorf("unexpected output:\nwant %q\ngot  %q", want, got)
	}
}

func TestWriteWrappedNoWidthEmitsSingleLine(t *testing.T) {
	var buf bytes.Buffer
	r := &treeRenderer{w: &buf, width: 0}
	r.writeWrapped("    ├── ", "    │   ", "this line is long but width=0 means no wrap")
	if got := buf.String(); strings.Count(got, "\n") != 1 {
		t.Errorf("expected exactly one line when width is 0:\n%s", got)
	}
}

// dumpTree is a hand-eyeball helper. Run with: go test -run TestDumpTree -v
func TestDumpTreeForEyeball(t *testing.T) {
	root := &cobra.Command{Use: "root", Short: "Root"}
	parent := &cobra.Command{Use: "parent", Short: "Parent command"}
	sibling := &cobra.Command{Use: "sibling", Short: "Sibling of parent"}
	alpha := &cobra.Command{Use: "alpha", Short: "First child"}
	beta := &cobra.Command{Use: "beta", Short: "Second child"}
	beta.Flags().String("filter", "", "A very long usage description that absolutely must wrap onto a continuation line so that we can verify the prefix bars are preserved through the wrap")
	parent.AddCommand(alpha, beta)
	root.AddCommand(parent, sibling)

	var buf bytes.Buffer
	(&treeRenderer{w: &buf, width: 70}).renderTree(root)
	t.Logf("\n%s", buf.String())
}

func TestPrintCommandTreeGroupedCommands(t *testing.T) {
	parent := &cobra.Command{Use: "skills", Short: "Skills"}
	configureSkillsCommandGroups(parent)
	parent.AddCommand(
		withSkillsGroup(&cobra.Command{Use: "init", Short: "Setup"}, skillsGroupSetup),
		withSkillsGroup(&cobra.Command{Use: "sync", Short: "Sync"}, skillsGroupSelection),
		withSkillsGroup(&cobra.Command{Use: "upload <dir>", Short: "Upload"}, skillsGroupRemote),
	)

	var buf bytes.Buffer
	PrintCommandTree(&buf, parent)
	out := buf.String()
	for _, want := range []string{"Setup:", "init", "Selection & sync", "sync", "Port catalog (in your organization)", "upload"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree missing %q:\n%s", want, out)
		}
	}
}

func TestPrintCommandTreeWrapKeepsBarsAligned(t *testing.T) {
	// Tree: root -> {parent, sibling}.  parent -> {alpha, beta}.  beta has a
	// flag whose usage forces wrap. Because parent is not the last child of
	// root, an open `│` must persist at column 4 through beta's wrapped flag
	// lines.
	root := &cobra.Command{Use: "root", Short: "Root"}
	parent := &cobra.Command{Use: "parent", Short: "Parent command"}
	sibling := &cobra.Command{Use: "sibling", Short: "Sibling of parent"}
	alpha := &cobra.Command{Use: "alpha", Short: "First child"}
	beta := &cobra.Command{Use: "beta", Short: "Second child"}
	beta.Flags().String("filter", "", "A very long usage description that absolutely must wrap onto a continuation line so that we can verify the prefix bars are preserved through the wrap")
	parent.AddCommand(alpha, beta)
	root.AddCommand(parent, sibling)

	var buf bytes.Buffer
	r := &treeRenderer{w: &buf, width: 70}
	r.renderTree(root)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	wrapped := 0
	for _, l := range lines {
		// Lines that belong to parent's subtree must carry the column-4 bar.
		// We detect them by their wider indent (any line indented past the
		// "    Commands:" header and not at the column-4 branch).
		if len(l) >= 5 && (l[0] == ' ' && l[1] == ' ' && l[2] == ' ' && l[3] == ' ') {
			fourth := l[4]
			// column 4 should be '│', '├', '└', or end-of-line (blank tail OK
			// only when parent has been closed, but here parent is non-last).
			if fourth == ' ' && strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "    Commands:") && !strings.HasPrefix(l, "    Global flags:") && !strings.HasPrefix(l, "    Inherited flags:") && !strings.HasPrefix(l, "    Flags:") {
				t.Errorf("line missing column-4 bar (parent subtree should keep it open):\n%q", l)
			}
		}
		if strings.Contains(l, "absolutely must wrap") || strings.Contains(l, "preserved through the wrap") {
			wrapped++
		}
	}
	if wrapped == 0 {
		t.Fatalf("expected the long usage to wrap, but no wrapped fragment found:\n%s", out)
	}
}
