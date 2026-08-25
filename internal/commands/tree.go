package commands

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TreeFlagName is the persistent flag name that triggers a tree dump.
const TreeFlagName = "tree"

const (
	treeIndent = "    "
	branchMid  = "├── "
	branchLast = "└── "
	contMid    = "│   "
	contLast   = "    "
)

// fallbackTreeWidth is used when stdout isn't a terminal (no wrapping needed)
// or the terminal size can't be queried. Zero disables wrapping entirely.
const fallbackTreeWidth = 0

// HasTreeFlag reports whether argv requests a command-tree dump.
func HasTreeFlag(args []string) bool {
	for _, a := range args {
		if a == "--"+TreeFlagName || a == "-"+TreeFlagName {
			return true
		}
		if strings.HasPrefix(a, "--"+TreeFlagName+"=") {
			return true
		}
	}
	return false
}

// FilterTreeFlag returns argv with the --tree token stripped so cobra's
// command resolution can ignore it.
func FilterTreeFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--"+TreeFlagName || a == "-"+TreeFlagName {
			continue
		}
		if strings.HasPrefix(a, "--"+TreeFlagName+"=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ResolveTreeTarget locates the command the user is asking about. Positional
// args and unknown flags are ignored — the tree dump describes shape, not
// invocation.
func ResolveTreeTarget(root *cobra.Command, args []string) *cobra.Command {
	target, _, err := root.Find(FilterTreeFlag(args))
	if err != nil || target == nil {
		return root
	}
	return target
}

type treeRenderer struct {
	w     io.Writer
	width int
}

func newTreeRenderer(w io.Writer) *treeRenderer {
	r := &treeRenderer{w: w, width: fallbackTreeWidth}
	if f, ok := w.(*os.File); ok && term.IsTerminal(f.Fd()) {
		if cols, _, err := term.GetSize(f.Fd()); err == nil && cols > 0 {
			r.width = cols
		}
	}
	return r
}

// PrintCommandTree prints the command, its subcommands, and their flags as a
// connected tree. Long lines are wrapped with prefix continuation so the
// vertical bars are never broken by overflowing text.
func PrintCommandTree(w io.Writer, cmd *cobra.Command) {
	newTreeRenderer(w).renderTree(cmd)
}

func (r *treeRenderer) renderTree(cmd *cobra.Command) {
	title := cmd.CommandPath()
	if cmd.Short != "" {
		title += " — " + cmd.Short
	}
	r.writeWrapped("", "", title)

	isRoot := cmd == cmd.Root()

	if isRoot {
		if globals := collectFlags(cmd.PersistentFlags()); len(globals) > 0 {
			fmt.Fprintln(r.w)
			fmt.Fprintln(r.w, treeIndent+"Global flags:")
			r.writeFlagLines(globals, treeIndent+treeIndent)
		}
	} else {
		if inherited := collectFlags(cmd.InheritedFlags()); len(inherited) > 0 {
			fmt.Fprintln(r.w)
			fmt.Fprintln(r.w, treeIndent+"Inherited flags:")
			r.writeFlagLines(inherited, treeIndent+treeIndent)
		}
		if local := collectFlags(cmd.LocalFlags()); len(local) > 0 {
			fmt.Fprintln(r.w)
			fmt.Fprintln(r.w, treeIndent+"Flags:")
			r.writeFlagLines(local, treeIndent+treeIndent)
		}
	}

	if groups := cmd.Groups(); len(groups) > 0 && cmd.AllChildCommandsHaveGroup() {
		fmt.Fprintln(r.w)
		for gi, group := range groups {
			if gi > 0 {
				fmt.Fprintln(r.w)
			}
			fmt.Fprintln(r.w, treeIndent+strings.TrimSpace(group.Title))
			r.emitSiblings(visibleSubcommandsInGroup(cmd, group.ID), treeIndent)
		}
		return
	}

	subs := visibleSubcommands(cmd)
	if len(subs) == 0 {
		return
	}

	fmt.Fprintln(r.w)
	fmt.Fprintln(r.w, treeIndent+"Commands:")
	r.emitSiblings(subs, treeIndent)
}

func (r *treeRenderer) emitSiblings(subs []*cobra.Command, prefix string) {
	for i, sub := range subs {
		if i > 0 {
			fmt.Fprintln(r.w, prefix+"│")
		}
		r.printCommandBranch(sub, prefix, i == len(subs)-1)
	}
}

func (r *treeRenderer) printCommandBranch(cmd *cobra.Command, prefix string, isLast bool) {
	connector := branchMid
	cont := contMid
	if isLast {
		connector = branchLast
		cont = contLast
	}

	line := cmd.Use
	if cmd.Short != "" {
		line = cmd.Use + " — " + cmd.Short
	}

	childPrefix := prefix + cont
	r.writeWrapped(prefix+connector, childPrefix, line)

	if flags := collectFlags(cmd.LocalFlags()); len(flags) > 0 {
		r.writeFlagLines(flags, childPrefix+treeIndent)
	}

	subs := visibleSubcommands(cmd)
	if len(subs) > 0 {
		r.emitSiblings(subs, childPrefix)
	}
}

type flagLine struct {
	name  string
	usage string
}

func collectFlags(fs *pflag.FlagSet) []flagLine {
	var out []flagLine
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == TreeFlagName {
			return
		}
		out = append(out, flagLine{
			name:  formatFlagName(f),
			usage: f.Usage,
		})
	})
	return out
}

func formatFlagName(f *pflag.Flag) string {
	short := "    "
	if f.Shorthand != "" {
		short = "-" + f.Shorthand + ", "
	}
	name := "--" + f.Name
	if t := f.Value.Type(); t != "" && t != "bool" {
		name += " " + t
	}
	return short + name
}

func (r *treeRenderer) writeFlagLines(flags []flagLine, indent string) {
	width := 0
	for _, fl := range flags {
		if len(fl.name) > width {
			width = len(fl.name)
		}
	}
	gap := "    "
	for _, fl := range flags {
		head := indent + fmt.Sprintf("%-*s", width, fl.name)
		if fl.usage == "" {
			fmt.Fprintln(r.w, strings.TrimRight(head, " "))
			continue
		}
		cont := indent + strings.Repeat(" ", width) + gap
		r.writeWrapped(head+gap, cont, fl.usage)
	}
}

// displayWidth returns the number of grid cells the string occupies. Tree
// glyphs (│ ├ └ ─) are each one cell, so a simple rune count is correct for
// our prefixes; for arbitrary body text it remains a reasonable approximation
// in monospaced terminals.
func displayWidth(s string) int {
	return utf8.RuneCountInString(s)
}

// writeWrapped writes text with `head` as the first-line prefix and `cont` as
// the continuation prefix for any wrapped lines. Lines longer than the
// terminal width are wrapped on whitespace; the continuation prefix preserves
// the tree's vertical bars and column alignment.
func (r *treeRenderer) writeWrapped(head, cont, body string) {
	if r.width <= 0 {
		fmt.Fprintln(r.w, head+body)
		return
	}

	headW := displayWidth(head)
	if headW+displayWidth(body) <= r.width {
		fmt.Fprintln(r.w, head+body)
		return
	}

	available := r.width - headW
	contAvailable := r.width - displayWidth(cont)
	if available < 20 || contAvailable < 20 {
		// Available space is so narrow that wrapping would shred the text
		// into useless slivers; emit unwrapped to preserve readability.
		fmt.Fprintln(r.w, head+body)
		return
	}

	first, rest := wrapByWords(body, available, contAvailable)
	fmt.Fprintln(r.w, head+first)
	for _, line := range rest {
		fmt.Fprintln(r.w, cont+line)
	}
}

// wrapByWords splits body into a first-line chunk of up to firstWidth chars
// and subsequent chunks of up to restWidth chars. Splits occur on whitespace
// where possible; a word longer than the target width is emitted whole on its
// own line.
func wrapByWords(body string, firstWidth, restWidth int) (string, []string) {
	if firstWidth < 1 {
		firstWidth = 1
	}
	if restWidth < 1 {
		restWidth = 1
	}

	words := strings.Fields(body)
	if len(words) == 0 {
		return body, nil
	}

	var lines []string
	current := ""
	limit := firstWidth
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if displayWidth(current)+1+displayWidth(word) <= limit {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
		limit = restWidth
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines[0], lines[1:]
}

func visibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		out = append(out, sub)
	}
	return out
}

func visibleSubcommandsInGroup(cmd *cobra.Command, groupID string) []*cobra.Command {
	var out []*cobra.Command
	for _, sub := range visibleSubcommands(cmd) {
		if sub.GroupID == groupID {
			out = append(out, sub)
		}
	}
	return out
}
