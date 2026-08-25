package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestPackSkillFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: my-skill
description: Demo skill
---
# Instructions
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "guide.md"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Identifier != "my-skill" {
		t.Fatalf("identifier = %q", pack.Identifier)
	}
	if pack.Description != "Demo skill" {
		t.Fatalf("description = %q", pack.Description)
	}
	if len(pack.Files) < 2 {
		t.Fatalf("files = %+v", pack.Files)
	}
	if pack.Location != "global" {
		t.Fatalf("location = %q, want global", pack.Location)
	}
}

func TestPackSkillFolder_acceptsUTF8BOMSkillMD(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "unicode-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := append([]byte{0xef, 0xbb, 0xbf}, []byte(`---
name: unicode-skill
description: Demo skill
---
# Instructions
Привет from Port
`)...)
	writeRawSkillMD(t, dir, content)

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatalf("PackSkillFolder: %v", err)
	}
	if pack.Description != "Demo skill" {
		t.Fatalf("description = %q", pack.Description)
	}
	skillMD := findFileContent(pack.Files, "SKILL.md")
	if strings.HasPrefix(skillMD, "\ufeff") {
		t.Fatal("SKILL.md content still includes UTF-8 BOM")
	}
	if !strings.Contains(skillMD, "Привет from Port") {
		t.Fatalf("SKILL.md content missing Unicode body: %q", skillMD)
	}
}

func TestPackSkillFolder_acceptsUTF16LESkillMD(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "utf16-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawSkillMD(t, dir, utf16LEBOMBytes(`---
name: utf16-skill
description: Unicode body
---
# Instructions
Snowman: ☃
`))

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatalf("PackSkillFolder: %v", err)
	}
	if pack.Description != "Unicode body" {
		t.Fatalf("description = %q", pack.Description)
	}
	if skillMD := findFileContent(pack.Files, "SKILL.md"); !strings.Contains(skillMD, "Snowman: ☃") {
		t.Fatalf("SKILL.md content missing decoded body: %q", skillMD)
	}
}

func TestPackSkillFolder_acceptsUTF16BESkillMD(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "utf16-be-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawSkillMD(t, dir, utf16BEBOMBytes(`---
name: utf16-be-skill
description: Unicode body
---
# Instructions
Rocket: 🚀
`))

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatalf("PackSkillFolder: %v", err)
	}
	if pack.Description != "Unicode body" {
		t.Fatalf("description = %q", pack.Description)
	}
	if skillMD := findFileContent(pack.Files, "SKILL.md"); !strings.Contains(skillMD, "Rocket: 🚀") {
		t.Fatalf("SKILL.md content missing decoded body: %q", skillMD)
	}
}

func TestPackSkillFolder_rejectsInvalidSkillFileEncoding(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "invalid-encoding")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawSkillMD(t, dir, []byte{0x80, 0x81, 0x82})

	_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err == nil {
		t.Fatal("expected invalid encoding error")
	}
	if !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected invalid UTF-8 error, got: %v", err)
	}
}

func TestPackSkillFolder_rejectsMalformedUTF16SkillMD(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "malformed-utf16")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawSkillMD(t, dir, utf16LEBOMCodeUnits([]uint16{
		'-', '-', '-', '\n',
		'n', 'a', 'm', 'e', ':', ' ', 'm', 'a', 'l', 'f', 'o', 'r', 'm', 'e', 'd', '-', 'u', 't', 'f', '1', '6', '\n',
		'd', 'e', 's', 'c', 'r', 'i', 'p', 't', 'i', 'o', 'n', ':', ' ', 'D', 'e', 'm', 'o', '\n',
		'-', '-', '-', '\n',
		0xd800,
	}))

	_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err == nil {
		t.Fatal("expected malformed UTF-16 error")
	}
	if !strings.Contains(err.Error(), "invalid UTF-16") {
		t.Fatalf("expected invalid UTF-16 error, got: %v", err)
	}
}

func TestPackSkillFolder_preservesBinaryBundledFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "binary-asset-skill")
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: binary-asset-skill
description: Includes a binary asset
---
# Skill
`)
	binaryContent := []byte{0x00, 0x80, 0xff, 0x47, 0x49, 0x46}
	if err := os.WriteFile(filepath.Join(dir, "assets", "image.gif"), binaryContent, 0o644); err != nil {
		t.Fatal(err)
	}

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatalf("PackSkillFolder: %v", err)
	}
	if got := findFileContent(pack.Files, "assets/image.gif"); string(binaryContent) != got {
		t.Fatalf("binary asset content changed: got %v want %v", []byte(got), binaryContent)
	}
}

func TestPackSkillFolder_symlinkedSkillDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real-skill")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(`---
name: find-skills
description: Via symlink
---
`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "find-skills")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	pack, err := PackSkillFolder(link, PackSkillFolderOptions{})
	if err != nil {
		t.Fatalf("PackSkillFolder symlink: %v", err)
	}
	if pack.Identifier != "find-skills" {
		t.Fatalf("identifier = %q", pack.Identifier)
	}
	if len(pack.Files) != 1 || pack.Files[0].Path != "SKILL.md" {
		t.Fatalf("files = %+v", pack.Files)
	}
}

func TestPackSkillFolder_locationFromFlag(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "flag-location-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: flag-location-skill
description: Demo skill
---
# Skill
`)

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{Location: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Location != "project" {
		t.Fatalf("location = %q", pack.Location)
	}
}

func TestPackSkillFolder_locationFromFrontmatter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "frontmatter-location-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: frontmatter-location-skill
description: Demo skill
location: project
---
# Skill
`)

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Location != "project" {
		t.Fatalf("location = %q", pack.Location)
	}
}

func TestPackSkillFolder_flagOverridesFrontmatter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "override-location-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: override-location-skill
description: Demo skill
location: project
---
# Skill
`)

	pack, err := PackSkillFolder(dir, PackSkillFolderOptions{Location: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Location != "global" {
		t.Fatalf("location = %q", pack.Location)
	}
}

func TestNormalizeSkillLocation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "global", false},
		{"global", "global", false},
		{"PROJECT", "project", false},
		{"invalid", "", true},
	} {
		got, err := NormalizeSkillLocation(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("NormalizeSkillLocation(%q) expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeSkillLocation(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("got %q want %q", got, tt.want)
		}
	}
}

func writeSkillMD(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRawSkillMD(t *testing.T, dir string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func utf16LEBOMBytes(content string) []byte {
	encoded := utf16.Encode([]rune(content))
	return utf16LEBOMCodeUnits(encoded)
}

func utf16BEBOMBytes(content string) []byte {
	encoded := utf16.Encode([]rune(content))
	out := []byte{0xfe, 0xff}
	for _, r := range encoded {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func utf16LEBOMCodeUnits(encoded []uint16) []byte {
	out := []byte{0xff, 0xfe}
	for _, r := range encoded {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func TestPackSkillFolder_rejectsNameFolderMismatch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-folder")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: other-name
description: Demo
---
# Skill
`)
	_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err == nil {
		t.Fatal("expected error when folder name and SKILL.md name differ")
	}
	if !strings.Contains(err.Error(), "does not match SKILL.md name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackSkillFolder_rejectsNameOutsideAgentSkillsSpec(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "deploy_helper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillMD(t, dir, `---
name: deploy_helper
description: Demo
---
# Skill
`)
	_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err == nil {
		t.Fatal("expected error when skill name uses an underscore")
	}
	if !strings.Contains(err.Error(), "Agent Skills name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackSkillFolder_requiresSkillMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
	if err == nil {
		t.Fatal("expected error without SKILL.md")
	}
}

func TestPackSkillFolder_requiresSkillMDNameAndDescription(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty skill file",
			content: "",
			want:    "SKILL.md frontmatter must include name",
		},
		{
			name: "missing name",
			content: `---
description: Demo skill
---
# Skill
`,
			want: "SKILL.md frontmatter must include name",
		},
		{
			name: "missing description",
			content: `---
name: missing-description
---
# Skill
`,
			want: "SKILL.md frontmatter must include description",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "missing-description")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			writeSkillMD(t, dir, tc.content)

			_, err := PackSkillFolder(dir, PackSkillFolderOptions{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}
