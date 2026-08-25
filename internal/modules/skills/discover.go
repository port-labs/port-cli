package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverSkillRoots returns skill directory paths under path.
// If path contains SKILL.md at its root, returns [path].
// Otherwise searches descendants for directories whose root contains SKILL.md.
// Descent stops at each skill directory (contents are not searched further).
func DiscoverSkillRoots(path string) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve skill path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("skill path %q: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill path %q is not a directory", path)
	}

	if skillDirectory(abs) {
		return []string{abs}, nil
	}

	var roots []string
	if err := discoverSkillRootsRecursive(abs, &roots); err != nil {
		return nil, err
	}
	sort.Strings(roots)
	if len(roots) == 0 {
		return nil, fmt.Errorf("no skill directories found under %q (expected SKILL.md at the root of a skill folder)", path)
	}
	return roots, nil
}

func discoverSkillRootsRecursive(dir string, roots *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", dir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." || strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}

		child := filepath.Join(dir, name)
		if skillDirectory(child) {
			*roots = append(*roots, child)
			continue
		}
		if searchableDirectory(child, entry) {
			if err := discoverSkillRootsRecursive(child, roots); err != nil {
				return err
			}
		}
	}
	return nil
}

func skillDirectory(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil && info != nil && !info.IsDir()
}

func searchableDirectory(path string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
