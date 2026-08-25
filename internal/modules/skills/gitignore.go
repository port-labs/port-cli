package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ensureProjectSkillGitignores(ctx context.Context, projectTargets []string) []string {
	var warnings []string
	seen := make(map[string]bool)
	for _, target := range projectTargets {
		skillsDir := skillsDirForTarget(target)
		if seen[skillsDir] {
			continue
		}
		seen[skillsDir] = true
		if warning := ensureProjectSkillGitignore(ctx, skillsDir); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func ensureProjectSkillGitignore(ctx context.Context, portDir string) string {
	repoRoot, warning := gitRepoRoot(ctx, portDir)
	if warning != "" || repoRoot == "" {
		return warning
	}
	evaluatedRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return fmt.Sprintf("could not update .gitignore for %s: %v", portDir, err)
	}
	evaluatedPortDir, err := filepath.EvalSymlinks(portDir)
	if err != nil {
		return fmt.Sprintf("could not update .gitignore for %s: %v", portDir, err)
	}

	rel, err := filepath.Rel(evaluatedRepoRoot, evaluatedPortDir)
	if err != nil {
		return fmt.Sprintf("could not update .gitignore for %s: %v", portDir, err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return fmt.Sprintf("could not update .gitignore for %s: path is outside git repository %s", portDir, repoRoot)
	}
	if !strings.HasSuffix(rel, "/") {
		rel += "/"
	}

	ignored, warning := gitCheckIgnored(ctx, repoRoot, rel)
	if warning != "" || ignored {
		return warning
	}
	if err := appendGitignoreEntry(filepath.Join(repoRoot, ".gitignore"), rel); err != nil {
		return fmt.Sprintf("could not update .gitignore for %s: %v", portDir, err)
	}
	return ""
}

func gitRepoRoot(ctx context.Context, dir string) (string, string) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", ""
	}
	return "", fmt.Sprintf("could not inspect git repository for %s: %v", dir, err)
}

func gitCheckIgnored(ctx context.Context, repoRoot, relPath string) (bool, string) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "check-ignore", "-q", "--", relPath)
	err := cmd.Run()
	if err == nil {
		return true, ""
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, ""
	}
	return false, fmt.Sprintf("could not check whether %s is gitignored: %v", relPath, err)
}

func appendGitignoreEntry(path, entry string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(existing)
	prefix := ""
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + entry + "\n")
	return err
}
