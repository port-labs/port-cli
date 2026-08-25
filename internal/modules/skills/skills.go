package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	skillFrontmatterPattern                 = regexp.MustCompile(`(?s)(?:^|\n)---[ \t]*\n(.*?)\n---[ \t]*(?:\n|$)`)
	skillFrontmatterNamePattern             = regexp.MustCompile(`(?m)^[ \t]*name[ \t]*:[ \t]*(?:"([^"\n]*)"|'([^'\n]*)'|([^#\n]*?))[ \t]*(?:#.*)?$`)
	skillFrontmatterDescriptionPattern      = regexp.MustCompile(`(?m)^[ \t]*description[ \t]*:`)
	skillFrontmatterDescriptionValuePattern = regexp.MustCompile(
		`(?m)^[ \t]*description[ \t]*:[ \t]*(?:"([^"\n]*)"|'([^'\n]*)'|([^#\n]*?))[ \t]*(?:#.*)?$`,
	)
)

func filterOrphanSkillFiles(skill Skill, files []SkillFile) []SkillFile {
	filtered := make([]SkillFile, 0, len(files))
	for _, file := range files {
		if isOrphanSkillFile(skill, file.Path) {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func isOrphanSkillFile(skill Skill, path string) bool {
	parts, ok := pathPartsAfterSkillsDir(path)
	if !ok {
		return false
	}
	skillDirName, err := skillDirName(skill)
	if err != nil {
		return true
	}
	_, found := trimToSkillDir(parts, skillDirName, skill)
	return !found
}

// FilterSkills returns skills matching the provided selection criteria.
func FilterSkills(fetched *FetchedSkills, selectAll, selectAllGroups, selectAllUngrouped bool, selectedGroups, selectedSkills []string, serverFilteredGroups bool) []Skill {
	if selectAll {
		return append([]Skill(nil), fetched.Skills...)
	}

	selectedGroupSet := toSet(selectedGroups)
	selectedSkillSet := toSet(selectedSkills)

	var result []Skill
	for _, s := range fetched.Skills {
		ungrouped := len(s.GroupIDs) == 0
		if !ungrouped && serverFilteredGroups {
			result = append(result, s)
			continue
		}
		switch {
		case ungrouped && selectAllUngrouped:
			result = append(result, s)
		case ungrouped && selectedSkillSet[s.Identifier]:
			result = append(result, s)
		case !ungrouped && selectAllGroups:
			result = append(result, s)
		case !ungrouped && anyGroupSelected(selectedGroupSet, s.GroupIDs):
			result = append(result, s)
		case selectedSkillSet[s.Identifier]:
			result = append(result, s)
		}
	}
	return result
}

func anyGroupSelected(selectedGroupSet map[string]bool, groupIDs []string) bool {
	for _, gid := range groupIDs {
		if selectedGroupSet[gid] {
			return true
		}
	}
	return false
}

// GroupName resolves the display name for a group, falling back to its identifier.
func GroupName(groups []SkillGroup, groupID string) string {
	for _, g := range groups {
		if g.Identifier == groupID {
			if g.Title != "" {
				return g.Title
			}
			return g.Identifier
		}
	}
	if groupID != "" {
		return groupID
	}
	return NoGroupDir
}

const portSkillsManifestFile = ".port-skills-manifest.json"

type portSkillsManifest struct {
	Skills []portSkillsManifestEntry `json:"skills"`
}

type portSkillsManifestEntry struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

type WriteSkillsOptions struct {
	FailOnSkillError bool
}

type preparedSkill struct {
	skill Skill
	name  string
}

// WriteSkills writes SKILL.md files (plus references, assets, scripts, and
// additional files) for each skill,
// routing each one based on its Location property:
//   - SkillLocationGlobal  → written into every dir in globalTargets
//   - SkillLocationProject → written into the matching tool sub-directory
//     inside every projectDir (e.g. <projectDir>/.agents/skills/…)
func WriteSkills(skills []Skill, groups []SkillGroup, globalTargets []string, projectDirs []string) error {
	_, err := WriteSkillsWithOptions(skills, groups, globalTargets, projectDirs, WriteSkillsOptions{FailOnSkillError: true})
	return err
}

func WriteSkillsWithOptions(skills []Skill, groups []SkillGroup, globalTargets []string, projectDirs []string, opts WriteSkillsOptions) ([]string, error) {
	globalSkills := make([]Skill, 0, len(skills))
	projectSkills := make([]Skill, 0)
	for _, s := range skills {
		if s.Location == SkillLocationProject {
			projectSkills = append(projectSkills, s)
		} else {
			globalSkills = append(globalSkills, s)
		}
	}

	skillsByDir := make(map[string][]Skill)
	addSkillsForTargets := func(targets []string, list []Skill) {
		for _, target := range targets {
			skillsDir := skillsDirForTarget(target)
			skillsByDir[skillsDir] = append(skillsByDir[skillsDir], list...)
		}
	}
	addSkillsForTargets(globalTargets, globalSkills)
	// Always process project targets when project dirs are configured, even if no
	// skill is currently project-scoped: a skill whose location just changed away
	// from "project" (or was removed) still needs its stale project-dir copy
	// reconciled away by writeSkillsToDir.
	if len(projectDirs) > 0 {
		addSkillsForTargets(buildProjectTargets(globalTargets, projectDirs), projectSkills)
	}

	var warnings []string
	for skillsDir, list := range skillsByDir {
		dirWarnings, err := writeSkillsToDir(mergeSkillsByIdentifier(list), skillsDir, opts)
		warnings = append(warnings, dirWarnings...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func skillsDirForTarget(target string) string {
	return filepath.Join(expandHome(target), "skills")
}

func mergeSkillsByIdentifier(skills []Skill) []Skill {
	if len(skills) == 0 {
		return nil
	}
	byID := make(map[string]Skill, len(skills))
	order := make([]string, 0, len(skills))
	for _, s := range skills {
		if _, seen := byID[s.Identifier]; !seen {
			order = append(order, s.Identifier)
		}
		byID[s.Identifier] = s
	}
	out := make([]Skill, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// buildProjectTargets creates project-level target paths by combining each
// project directory with the tool sub-directory derived from the global
// targets. When a tool defines a ProjectDir override, that name is used under
// each project dir instead of Dir (rare; most tools use Dir only).
func buildProjectTargets(globalTargets []string, projectDirs []string) []string {
	toolDirs := extractProjectDirs(globalTargets)
	seen := make(map[string]bool)
	var result []string
	for _, pd := range projectDirs {
		for _, td := range toolDirs {
			p := filepath.Join(pd, td)
			if !seen[p] {
				result = append(result, p)
				seen[p] = true
			}
		}
	}
	return result
}

// extractProjectDirs returns the relative directory names to use for
// project-scoped skills. For each global target it checks known hook targets:
// if the target has a ProjectDir override that directory is used, otherwise
// the target's Dir is used. Unrecognized paths fall back to the base name.
// Legacy GitHub Copilot paths ending in /.copilot map to ".github".
func extractProjectDirs(globalTargets []string) []string {
	knownTargets := DefaultHookTargets()
	seen := make(map[string]bool)
	var dirs []string
	for _, gt := range globalTargets {
		expanded := expandHome(gt)
		matched := false
		for _, kt := range knownTargets {
			if matchesTarget(expanded, kt) {
				d := kt.Dir
				if kt.ProjectDir != "" {
					d = kt.ProjectDir
				}
				if !seen[d] {
					dirs = append(dirs, d)
					seen[d] = true
				}
				matched = true
				break
			}
		}
		if !matched {
			base := filepath.Base(expanded)
			if !seen[base] {
				dirs = append(dirs, base)
				seen[base] = true
			}
		}
	}
	return dirs
}

func writeSkillsToDir(skills []Skill, skillsDir string, opts WriteSkillsOptions) ([]string, error) {
	prepared, skipped, warnings, err := prepareSkillsForWrite(skills, opts)
	if err != nil {
		return warnings, err
	}

	previousManifest, err := readPortSkillsManifest(skillsDir)
	if err != nil {
		return warnings, err
	}

	expected := make(map[string]bool)
	manifest := portSkillsManifest{Skills: make([]portSkillsManifestEntry, 0, len(prepared))}
	for _, item := range prepared {
		expected[item.name] = true
		manifest.Skills = append(manifest.Skills, portSkillsManifestEntry{
			Identifier: item.skill.Identifier,
			Name:       item.name,
		})
	}
	for _, entry := range previousManifest.Skills {
		// Only carry forward a skipped skill's stale manifest entry if its
		// directory name isn't already claimed by a skill written in this
		// run. Otherwise two identifiers would map to the same directory in
		// the manifest, and a later unload of the skipped identifier would
		// delete the directory that now belongs to the live, prepared skill
		// (removeSkillFromDir matches by Identifier, not by Name).
		if skipped[entry.Identifier] && !expected[entry.Name] {
			expected[entry.Name] = true
			manifest.Skills = append(manifest.Skills, entry)
		}
	}

	for _, item := range prepared {
		s := item.skill
		skillDirName := item.name
		skillDir := filepath.Join(skillsDir, skillDirName)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return warnings, fmt.Errorf("failed to create skill directory %s: %w", skillDir, err)
		}

		if err := writeSkillFiles(skillDir, skillDirName, s); err != nil {
			if opts.FailOnSkillError {
				return warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("skipped skill %s: %v", s.Identifier, err))
			delete(expected, skillDirName)
			_ = os.RemoveAll(skillDir)
		}
	}

	if err := reconcileSkills(skillsDir, previousManifest, expected); err != nil {
		return warnings, fmt.Errorf("reconciliation failed for %s: %w", skillsDir, err)
	}
	if err := removeLegacyPortSkillsDir(skillsDir); err != nil {
		return warnings, err
	}
	if err := writePortSkillsManifest(skillsDir, manifest); err != nil {
		return warnings, err
	}
	return warnings, nil
}

func prepareSkillsForWrite(skills []Skill, opts WriteSkillsOptions) ([]preparedSkill, map[string]bool, []string, error) {
	var warnings []string
	skipped := make(map[string]bool)
	byName := make(map[string][]Skill)
	for _, s := range skills {
		if !hasSkillMD(s.Files) {
			if err := handleSkillWriteError(s.Identifier, fmt.Errorf("skill %s has no SKILL.md in catalog files", s.Identifier), opts, &warnings, skipped); err != nil {
				return nil, skipped, warnings, err
			}
			continue
		}
		name, err := skillDirName(s)
		if err != nil {
			if err := handleSkillWriteError(s.Identifier, err, opts, &warnings, skipped); err != nil {
				return nil, skipped, warnings, err
			}
			continue
		}
		byName[name] = append(byName[name], s)
	}

	prepared := make([]preparedSkill, 0, len(skills))
	for name, namedSkills := range byName {
		if len(namedSkills) > 1 {
			err := fmt.Errorf("multiple skills resolve to local skill name %q", name)
			for _, s := range namedSkills {
				if handleErr := handleSkillWriteError(s.Identifier, err, opts, &warnings, skipped); handleErr != nil {
					return nil, skipped, warnings, handleErr
				}
			}
			continue
		}
		prepared = append(prepared, preparedSkill{skill: namedSkills[0], name: name})
	}
	return prepared, skipped, warnings, nil
}

func handleSkillWriteError(identifier string, err error, opts WriteSkillsOptions, warnings *[]string, skipped map[string]bool) error {
	if opts.FailOnSkillError {
		return err
	}
	skipped[identifier] = true
	*warnings = append(*warnings, fmt.Sprintf("skipped skill %s: %v", identifier, err))
	return nil
}

func findSkillMDFile(files []SkillFile) (SkillFile, bool) {
	for _, file := range files {
		path := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if path == "SKILL.md" || filepath.Base(path) == "SKILL.md" {
			return file, true
		}
	}
	return SkillFile{}, false
}

func hasSkillMD(files []SkillFile) bool {
	_, ok := findSkillMDFile(files)
	return ok
}

func reconcileSkills(skillsDir string, previousManifest portSkillsManifest, expected map[string]bool) error {
	cleanSkillsDir := filepath.Clean(skillsDir) + string(filepath.Separator)
	for _, entry := range previousManifest.Skills {
		skillName := entry.Name
		if !isSafeDirName(skillName) {
			continue
		}
		cleanSkillPath := filepath.Clean(filepath.Join(skillsDir, skillName))
		if !strings.HasPrefix(cleanSkillPath+string(filepath.Separator), cleanSkillsDir) {
			continue
		}
		if !expected[skillName] {
			if err := os.RemoveAll(cleanSkillPath); err != nil {
				return fmt.Errorf("failed to remove stale skill %s: %w", skillName, err)
			}
		}
	}
	return nil
}

func readPortSkillsManifest(skillsDir string) (portSkillsManifest, error) {
	path := filepath.Join(skillsDir, portSkillsManifestFile)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return portSkillsManifest{}, nil
		}
		return portSkillsManifest{}, fmt.Errorf("failed to read skills manifest %s: %w", path, err)
	}
	var manifest portSkillsManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return portSkillsManifest{}, fmt.Errorf("failed to parse skills manifest %s: %w", path, err)
	}
	return manifest, nil
}

func writePortSkillsManifest(skillsDir string, manifest portSkillsManifest) error {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skills directory %s: %w", skillsDir, err)
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode skills manifest: %w", err)
	}
	content = append(content, '\n')
	path := filepath.Join(skillsDir, portSkillsManifestFile)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write skills manifest %s: %w", path, err)
	}
	return nil
}

func removeLegacyPortSkillsDir(skillsDir string) error {
	legacyDir := filepath.Join(skillsDir, PortSkillsDir)
	if _, err := os.Stat(legacyDir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(legacyDir); err != nil {
		return fmt.Errorf("failed to remove legacy skills directory %s: %w", legacyDir, err)
	}
	return nil
}

func clearPortManagedSkillsDir(skillsDir string) (bool, error) {
	removed := false
	manifest, err := readPortSkillsManifest(skillsDir)
	if err != nil {
		return false, err
	}
	cleanSkillsDir := filepath.Clean(skillsDir) + string(filepath.Separator)
	for _, entry := range manifest.Skills {
		if !isSafeDirName(entry.Name) {
			continue
		}
		skillDir := filepath.Clean(filepath.Join(skillsDir, entry.Name))
		if !strings.HasPrefix(skillDir+string(filepath.Separator), cleanSkillsDir) {
			continue
		}
		if _, err := os.Stat(skillDir); err == nil {
			if err := os.RemoveAll(skillDir); err != nil {
				return removed, err
			}
			removed = true
		}
	}
	manifestPath := filepath.Join(skillsDir, portSkillsManifestFile)
	if _, err := os.Stat(manifestPath); err == nil {
		if err := os.Remove(manifestPath); err != nil {
			return removed, err
		}
		removed = true
	}
	legacyDir := filepath.Join(skillsDir, PortSkillsDir)
	if _, err := os.Stat(legacyDir); err == nil {
		if err := os.RemoveAll(legacyDir); err != nil {
			return removed, err
		}
		removed = true
	}
	return removed, nil
}

// isSafeDirName returns true if name is a plain directory basename with no path
// traversal sequences or separators. This prevents path traversal when names
// sourced from os.ReadDir are used in subsequent file operations.
func isSafeDirName(name string) bool {
	return name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

func writeSkillFiles(skillDir, skillDirName string, s Skill) error {
	hasSkillMD := false
	for _, f := range filterOrphanSkillFiles(s, s.Files) {
		relPath, err := normalizeSkillFilePath(f.Path, skillDirName, s)
		if err != nil {
			return fmt.Errorf("failed to write file %s for skill %s: %w", f.Path, s.Identifier, err)
		}
		file := f
		if relPath == "SKILL.md" {
			hasSkillMD = true
			file.Content = normalizeSkillMDContent(s, skillDirName, f.Content)
		}
		if err := writeSkillFile(skillDir, SkillFile{Path: relPath, Content: file.Content}); err != nil {
			return fmt.Errorf("failed to write file %s for skill %s: %w", f.Path, s.Identifier, err)
		}
	}
	if !hasSkillMD {
		return fmt.Errorf("skill %s has no SKILL.md in catalog files", s.Identifier)
	}
	return nil
}

func writeSkillFile(skillDir string, f SkillFile) error {
	dest := filepath.Join(skillDir, filepath.FromSlash(f.Path))
	cleanDest := filepath.Clean(dest)
	cleanBase := filepath.Clean(skillDir) + string(filepath.Separator)
	if !strings.HasPrefix(cleanDest+string(filepath.Separator), cleanBase) {
		return fmt.Errorf("skill file path %q escapes skill directory", f.Path)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", dest, err)
	}
	return os.WriteFile(dest, []byte(f.Content), 0o644)
}

func normalizeSkillFilePath(path, skillDirName string, s Skill) (string, error) {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if path == "." || path == "" || strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("skill file path %q escapes skill directory", path)
	}

	parts := strings.Split(path, "/")
	if skillsParts, ok := pathPartsAfterSkillsDir(path); ok {
		trimmedParts, found := trimToSkillDir(skillsParts, skillDirName, s)
		if !found {
			return "", fmt.Errorf("skill file path %q is not inside a skill directory", path)
		}
		parts = trimmedParts
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("skill file path %q escapes skill directory", path)
	}
	return strings.Join(parts, "/"), nil
}

func pathPartsAfterSkillsDir(path string) ([]string, bool) {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "skills" {
			return parts[i+1:], true
		}
	}
	return nil, false
}

func trimToSkillDir(parts []string, skillDirName string, s Skill) ([]string, bool) {
	for i := 0; i < len(parts); i++ {
		if isSkillDirPart(parts[i], skillDirName, s) && i+1 < len(parts) {
			return parts[i+1:], true
		}
	}
	return nil, false
}

func isSkillDirPart(part, skillDirName string, s Skill) bool {
	return part == skillDirName || part == s.Title || part == skillIdentifierBase(s.Identifier)
}

func skillIdentifierBase(identifier string) string {
	identifier = strings.Trim(identifier, "/\\")
	if identifier == "" {
		return ""
	}
	return filepath.Base(filepath.ToSlash(identifier))
}

func skillDirName(s Skill) (string, error) {
	content := skillMDContent(s.Files)
	if name := frontmatterSkillName(content); name != "" {
		if err := validateAgentSkillName(name); err != nil {
			return "", fmt.Errorf("invalid skill directory name for %q: %w", s.Identifier, err)
		}
		return name, nil
	}
	if s.AgentSkillName != "" {
		if err := validateAgentSkillName(s.AgentSkillName); err != nil {
			return "", fmt.Errorf("invalid skill directory name for %q: %w", s.Identifier, err)
		}
		return s.AgentSkillName, nil
	}
	name, err := agentSkillNameFromIdentifier(s.Identifier)
	if err != nil {
		return "", fmt.Errorf("invalid skill directory name for %q: %w", s.Identifier, err)
	}
	return name, nil
}

func skillMDContent(files []SkillFile) string {
	file, _ := findSkillMDFile(files)
	return file.Content
}

func normalizeSkillMDContent(s Skill, skillName, content string) string {
	description := strings.TrimSpace(s.Description)
	if description == "" {
		description = frontmatterDescription(content)
	}
	if description == "" {
		description = fmt.Sprintf("Port skill %s.", skillName)
	}
	return upsertSkillMDFrontmatter(content, skillName, description)
}

func upsertSkillMDFrontmatter(content, skillName, description string) string {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := []string{
		fmt.Sprintf("name: %s", skillName),
		fmt.Sprintf("description: %s", sanitizeFrontmatterScalar(description)),
	}
	if span, ok := findAgentSkillFrontmatter(content); ok {
		return rewriteFrontmatterSpan(content, span, lines)
	}
	if span, ok := findLeadingFrontmatter(content); ok {
		return rewriteFrontmatterSpan(content, span, lines)
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n\n" + content
}

func rewriteFrontmatterSpan(content string, span agentSkillFrontmatterSpan, lines []string) string {
	for _, line := range strings.Split(span.inner, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "name:") || strings.HasPrefix(trimmed, "description:") {
			continue
		}
		lines = append(lines, line)
	}
	suffix := stripLeadingAgentSkillFrontmatter(content[span.fenceEnd:])
	return content[:span.openStart] + "---\n" + strings.Join(lines, "\n") + "\n---" + suffix
}

type agentSkillFrontmatterSpan struct {
	openStart int
	fenceEnd  int
	inner     string
}

// findAgentSkillFrontmatter locates the first --- block that looks like a real
// Agent Skills header (valid name + description), matching frontmatterSkillName.
func findAgentSkillFrontmatter(content string) (agentSkillFrontmatterSpan, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	for _, match := range skillFrontmatterPattern.FindAllStringSubmatchIndex(content, -1) {
		span, ok := spanFromFrontmatterMatch(content, match)
		if !ok {
			continue
		}
		if !skillFrontmatterDescriptionPattern.MatchString(span.inner) {
			continue
		}
		if agentSkillNameFromFrontmatterInner(span.inner) == "" {
			continue
		}
		return span, true
	}
	return agentSkillFrontmatterSpan{}, false
}

// findLeadingFrontmatter locates a leading --- block even when it is incomplete
// (e.g. name without description), preserving the previous HasPrefix("---\n")
// update path so we rewrite in place instead of prepending a second header.
func findLeadingFrontmatter(content string) (agentSkillFrontmatterSpan, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---") {
		return agentSkillFrontmatterSpan{}, false
	}
	match := skillFrontmatterPattern.FindStringSubmatchIndex(content)
	if len(match) < 4 || match[0] != 0 {
		return agentSkillFrontmatterSpan{}, false
	}
	return spanFromFrontmatterMatch(content, match)
}

func spanFromFrontmatterMatch(content string, match []int) (agentSkillFrontmatterSpan, bool) {
	if len(match) < 4 || match[2] < 0 || match[3] < 0 {
		return agentSkillFrontmatterSpan{}, false
	}
	openStart := match[0]
	if openStart < len(content) && content[openStart] == '\n' {
		openStart++
	}
	fenceEnd := match[3] + 1 // skip the \n before closing ---
	if fenceEnd+3 > len(content) || content[fenceEnd:fenceEnd+3] != "---" {
		return agentSkillFrontmatterSpan{}, false
	}
	fenceEnd += 3
	for fenceEnd < len(content) && (content[fenceEnd] == ' ' || content[fenceEnd] == '\t') {
		fenceEnd++
	}
	return agentSkillFrontmatterSpan{
		openStart: openStart,
		fenceEnd:  fenceEnd,
		inner:     content[match[2]:match[3]],
	}, true
}

// stripLeadingAgentSkillFrontmatter removes a second Agent Skills frontmatter
// block at the start of body (after optional blank lines), healing content that
// was previously corrupted by a blind prepend.
func stripLeadingAgentSkillFrontmatter(body string) string {
	trimmed := strings.TrimLeft(body, "\n")
	span, ok := findAgentSkillFrontmatter(trimmed)
	if !ok || span.openStart != 0 {
		return body
	}
	rest := trimmed[span.fenceEnd:]
	if rest == "" {
		return "\n"
	}
	if strings.HasPrefix(rest, "\n") {
		return rest
	}
	return "\n" + rest
}

func agentSkillNameFromFrontmatterInner(inner string) string {
	nameMatch := skillFrontmatterNamePattern.FindStringSubmatch(inner)
	if len(nameMatch) < 4 {
		return ""
	}
	for _, value := range nameMatch[1:] {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if validateAgentSkillName(name) == nil {
			return name
		}
	}
	return ""
}

// frontmatterDescription extracts the description field using the same
// anywhere-in-content block scanning as frontmatterSkillName, so a fallback
// description is found even when the real Agent Skills header isn't the
// leading block (e.g. preceded by an unrelated "---" delimited section).
func frontmatterDescription(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	for _, frontmatterMatch := range skillFrontmatterPattern.FindAllStringSubmatch(content, -1) {
		if len(frontmatterMatch) < 2 {
			continue
		}
		valueMatch := skillFrontmatterDescriptionValuePattern.FindStringSubmatch(frontmatterMatch[1])
		if len(valueMatch) < 4 {
			continue
		}
		for _, value := range valueMatch[1:] {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func frontmatterSkillName(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if span, ok := findAgentSkillFrontmatter(content); ok {
		return agentSkillNameFromFrontmatterInner(span.inner)
	}
	return ""
}

func sanitizeFrontmatterScalar(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func toSet(slice []string) map[string]bool {
	s := make(map[string]bool, len(slice))
	for _, v := range slice {
		s[v] = true
	}
	return s
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home := userHomeDir(); home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func collectPortSkillDirs(globalTargets, projectDirs []string) []string {
	seen := make(map[string]bool)
	var dirs []string
	add := func(targets []string) {
		for _, target := range targets {
			skillsDir := skillsDirForTarget(target)
			if !seen[skillsDir] {
				seen[skillsDir] = true
				dirs = append(dirs, skillsDir)
			}
		}
	}
	add(globalTargets)
	if len(projectDirs) > 0 {
		add(buildProjectTargets(globalTargets, projectDirs))
	}
	return dirs
}

// UnloadSkillFromTargets removes local Port-managed copies of a skill for every target.
func UnloadSkillFromTargets(identifier string, globalTargets, projectDirs []string) error {
	for _, portDir := range collectPortSkillDirs(globalTargets, projectDirs) {
		if err := removeSkillFromDir(portDir, identifier); err != nil {
			return err
		}
	}
	return nil
}

func removeSkillFromDir(skillsDir, identifier string) error {
	manifestPath := filepath.Join(skillsDir, portSkillsManifestFile)
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return nil
	}
	manifest, err := readPortSkillsManifest(skillsDir)
	if err != nil {
		return err
	}
	cleanSkillsDir := filepath.Clean(skillsDir) + string(filepath.Separator)
	updated := portSkillsManifest{Skills: make([]portSkillsManifestEntry, 0, len(manifest.Skills))}
	for _, entry := range manifest.Skills {
		if entry.Identifier != identifier {
			updated.Skills = append(updated.Skills, entry)
			continue
		}
		if !isSafeDirName(entry.Name) {
			continue
		}
		skillPath := filepath.Clean(filepath.Join(skillsDir, entry.Name))
		if !strings.HasPrefix(skillPath+string(filepath.Separator), cleanSkillsDir) {
			continue
		}
		if err := os.RemoveAll(skillPath); err != nil {
			return fmt.Errorf("failed to remove skill %s: %w", skillPath, err)
		}
	}
	if len(updated.Skills) == 0 {
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writePortSkillsManifest(skillsDir, updated)
}
