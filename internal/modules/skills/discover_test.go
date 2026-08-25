package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkillRoots_singleSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := DiscoverSkillRoots(dir)
	if err != nil {
		t.Fatalf("DiscoverSkillRoots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d: %v", len(roots), roots)
	}
	if roots[0] != dir {
		t.Fatalf("expected root %q, got %q", dir, roots[0])
	}
}

func TestDiscoverSkillRoots_bundle(t *testing.T) {
	parent := t.TempDir()
	for _, name := range []string{"skill-a", "skill-b"} {
		child := filepath.Join(parent, name)
		if err := os.Mkdir(child, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(child, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	roots, err := DiscoverSkillRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverSkillRoots: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %v", len(roots), roots)
	}
}

func TestDiscoverSkillRoots_symlinkedChildren(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "bundle")
	targetA := filepath.Join(root, "skill-a")
	targetB := filepath.Join(root, "skill-b")
	for _, target := range []string{targetA, targetB} {
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetA, filepath.Join(parent, "skill-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, filepath.Join(parent, "skill-b")); err != nil {
		t.Fatal(err)
	}

	roots, err := DiscoverSkillRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverSkillRoots: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %v", len(roots), roots)
	}
}

func TestDiscoverSkillRoots_nested(t *testing.T) {
	parent := t.TempDir()
	nested := filepath.Join(parent, "group", "skill-a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("# skill-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := DiscoverSkillRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverSkillRoots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d: %v", len(roots), roots)
	}
	if roots[0] != nested {
		t.Fatalf("expected %q, got %q", nested, roots[0])
	}
}

func TestDiscoverSkillRoots_doesNotDescendIntoSkill(t *testing.T) {
	parent := t.TempDir()
	skill := filepath.Join(parent, "skill-a")
	nested := filepath.Join(skill, "nested-skill")
	for _, dir := range []string{skill, nested} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# skill-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("# nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := DiscoverSkillRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverSkillRoots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d: %v", len(roots), roots)
	}
	if roots[0] != skill {
		t.Fatalf("expected %q, got %q", skill, roots[0])
	}
}

func TestDiscoverSkillRoots_none(t *testing.T) {
	dir := t.TempDir()
	_, err := DiscoverSkillRoots(dir)
	if err == nil {
		t.Fatal("expected error when no SKILL.md found")
	}
}
