package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
)

// SkillFolderPack is the parsed content of a local skill directory.
type SkillFolderPack struct {
	Identifier  string
	Title       string
	Description string
	Location    string
	Files       []api.SkillFileInput
}

// PackSkillFolderOptions configures reading a skill directory from disk.
type PackSkillFolderOptions struct {
	Identifier  string
	Title       string
	Description string
	Location    string
}

// PackSkillFolder reads all files under dir into Port API file inputs.
// The folder must contain SKILL.md at its root. Identifier defaults to the
// directory name when opts.Identifier is empty.
func PackSkillFolder(dir string, opts PackSkillFolderOptions) (*SkillFolderPack, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, fmt.Errorf("skill folder %q: %w", dir, err)
	}
	absDir = resolvedDir

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("skill folder %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dir)
	}

	folderBase := filepath.Base(absDir)
	if strings.TrimSpace(opts.Identifier) == "" {
		if linkBase := filepath.Base(filepath.Clean(dir)); linkBase != "" && linkBase != "." {
			folderBase = linkBase
		}
	}

	var files []api.SkillFileInput
	hasSkillMD := false
	walkErr := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != absDir && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		rel, err := filepath.Rel(absDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		if rel == "SKILL.md" {
			hasSkillMD = true
			decoded, err := decodeSkillFileContent(rel, content)
			if err != nil {
				return err
			}
			files = append(files, api.SkillFileInput{
				Path:    rel,
				Content: decoded,
			})
			return nil
		}
		files = append(files, api.SkillFileInput{
			Path:    rel,
			Content: string(content),
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if !hasSkillMD {
		return nil, fmt.Errorf("skill folder must contain SKILL.md at its root")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("skill folder contains no files")
	}

	skillMD := findFileContent(files, "SKILL.md")
	meta, err := requireSkillMDMetadata(skillMD)
	if err != nil {
		return nil, err
	}
	title := opts.Title
	description := opts.Description
	location := opts.Location
	if title == "" && meta.Title != "" {
		title = meta.Title
	}
	if description == "" {
		description = meta.Description
	}
	if location == "" && meta.Location != "" {
		location = meta.Location
	}
	identifier, err := validateSkillFolderNameMatch(folderBase, files, opts.Identifier)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = identifier
	}
	var normErr error
	location, normErr = NormalizeSkillLocation(location)
	if normErr != nil {
		return nil, normErr
	}

	return &SkillFolderPack{
		Identifier:  identifier,
		Title:       title,
		Description: description,
		Location:    location,
		Files:       files,
	}, nil
}

// NormalizeSkillLocation validates and normalizes a skill location value.
// Empty input defaults to global.
func NormalizeSkillLocation(location string) (string, error) {
	location = strings.TrimSpace(strings.ToLower(location))
	if location == "" {
		return "global", nil
	}
	if location != "global" && location != "project" {
		return "", fmt.Errorf("location must be global or project, got %q", location)
	}
	return location, nil
}

type skillMDMetadata struct {
	Name        string
	Title       string
	Description string
	Location    string
}

func parseSkillMDMetadata(content string) *skillMDMetadata {
	frontmatterMatch := regexp.MustCompile(`(?s)^---\n(.*?)\n---`).FindStringSubmatch(content)
	if len(frontmatterMatch) < 2 {
		return nil
	}
	block := frontmatterMatch[1]
	meta := &skillMDMetadata{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		switch key {
		case "name":
			meta.Name = val
		case "title":
			meta.Title = val
		case "description":
			meta.Description = val
		case "location":
			meta.Location = val
		}
	}
	if meta.Name == "" && meta.Title == "" && meta.Description == "" && meta.Location == "" {
		return nil
	}
	return meta
}

func requireSkillMDMetadata(content string) (*skillMDMetadata, error) {
	meta := parseSkillMDMetadata(content)
	if meta == nil || strings.TrimSpace(meta.Name) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter must include name")
	}
	if strings.TrimSpace(meta.Description) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter must include description")
	}
	return meta, nil
}

func validateSkillFolderNameMatch(folderBase string, files []api.SkillFileInput, identifierOverride string) (string, error) {
	folderID := strings.TrimSpace(folderBase)
	if err := validateAgentSkillName(folderID); err != nil {
		return "", fmt.Errorf("skill folder name %q must be an Agent Skills name: %w", folderBase, err)
	}

	skillMD := findFileContent(files, "SKILL.md")
	if skillMD != "" {
		if meta := parseSkillMDMetadata(skillMD); meta != nil && meta.Name != "" {
			nameID := strings.TrimSpace(meta.Name)
			if err := validateAgentSkillName(nameID); err != nil {
				return "", fmt.Errorf("SKILL.md name %q must be an Agent Skills name: %w", meta.Name, err)
			}
			if nameID != folderID {
				return "", fmt.Errorf(
					`skill folder %q does not match SKILL.md name %q. Rename the folder or set name: in SKILL.md frontmatter so they match`,
					folderBase,
					meta.Name,
				)
			}
		}
	}

	if identifierOverride != "" {
		overrideID := strings.TrimSpace(identifierOverride)
		if err := validateAgentSkillName(overrideID); err != nil {
			return "", fmt.Errorf("--identifier %q must be an Agent Skills name: %w", identifierOverride, err)
		}
		if overrideID != folderID {
			return "", fmt.Errorf(
				`--identifier %q does not match skill folder %q. Use the folder name or align SKILL.md name: with the folder`,
				identifierOverride,
				folderBase,
			)
		}
	}

	return folderID, nil
}

func findFileContent(files []api.SkillFileInput, path string) string {
	for _, f := range files {
		if f.Path == path {
			return f.Content
		}
	}
	return ""
}
