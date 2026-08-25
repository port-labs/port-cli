package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
)

// Module orchestrates hook installation and skill syncing for Port AI skills.
type Module struct {
	client        *api.Client
	configManager *config.ConfigManager
	orgName       string
}

func NewModule(token *auth.Token, orgConfig *config.OrganizationConfig, configManager *config.ConfigManager, orgName string) *Module {
	client := api.NewClient(api.ClientOpts{
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Token:        token,
	})
	return &Module{
		client:        client,
		configManager: configManager,
		orgName:       strings.TrimSpace(orgName),
	}
}

// OrgName returns the Port organization this module is bound to.
func (m *Module) OrgName() string {
	return m.orgName
}

func (m *Module) FetchSkills(ctx context.Context) (*FetchedSkills, error) {
	return m.fetchSkills(ctx, nil, nil)
}

// MetadataCatalogQuery returns the full skills catalog query used for selection
// bookkeeping (init prompts, add/remove validation). It requests every group and
// ungrouped skills without file content so identifiers found by search resolve.
func MetadataCatalogQuery() FetchSkillsQuery {
	return FetchSkillsQuery{
		ExcludeFiles:     true,
		TeamsDefault:     BoolPtr(false),
		Exclude:          []string{"internal"},
		IncludeUngrouped: true,
	}
}

// FetchSkillsMetadata loads the catalog without file content for prompts and
// selection bookkeeping. Use LoadSkills for the write-to-disk path.
func (m *Module) FetchSkillsMetadata(ctx context.Context) (*FetchedSkills, error) {
	return FetchSkillsFromAPI(ctx, m.client, MetadataCatalogQuery())
}

// fetchSkills loads the sync catalog using saved config and optional per-call overrides.
func (m *Module) fetchSkills(ctx context.Context, cfg *config.SkillsConfig, opts *LoadSkillsOptions) (*FetchedSkills, error) {
	skillsCfg := cfg
	if skillsCfg == nil {
		var err error
		skillsCfg, err = m.configManager.LoadSkillsConfig()
		if err != nil {
			skillsCfg = &config.SkillsConfig{}
		}
	}
	return FetchSkillsFromAPI(ctx, m.client, buildFetchSkillsQuery(skillsCfg, opts))
}

// buildFetchSkillsQuery maps skills config and load options to GET /skills query params.
func buildFetchSkillsQuery(cfg *config.SkillsConfig, opts *LoadSkillsOptions) FetchSkillsQuery {
	query := FetchSkillsQuery{}
	if cfg == nil {
		cfg = &config.SkillsConfig{}
	}
	if cfg.UsesTeamGroupDefaults() {
		query.IncludeGroups = append([]string(nil), cfg.IncludeGroups...)
		query.ExcludeGroups = append([]string(nil), cfg.ExcludeGroups...)
		query.TeamsDefault = BoolPtr(true)
	} else if cfg.SelectAllGroups {
		// Include every group in the response so skills keep group folder layout on disk.
		query.TeamsDefault = BoolPtr(false)
	}
	if !cfg.SelectAll && !cfg.SelectAllUngrouped && len(cfg.SelectedSkills) > 0 {
		query.SkillIdentifiers = append([]string(nil), cfg.SelectedSkills...)
	}
	if cfg.SelectAll || cfg.SelectAllUngrouped {
		query.IncludeUngrouped = true
	}
	includeInternal := opts != nil && opts.IncludeInternalSkills
	if !includeInternal {
		query.Exclude = append(query.Exclude, "internal")
	}
	if opts != nil && opts.ExcludeLegacySkills {
		query.Exclude = append(query.Exclude, "legacy")
	}
	return query
}

// BoolPtr returns a bool pointer for optional skills API query flags.
func BoolPtr(v bool) *bool {
	return &v
}

// FetchGroupsForInit fetches all skill groups with team ownership for the init selection UI.
func (m *Module) FetchGroupsForInit(ctx context.Context) ([]api.SkillGroupAtLatestVersion, error) {
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	resp, err := m.client.GetSkillsGrouped(ctx, api.GetSkillsQuery{
		TeamsDefault: BoolPtr(false),
		Exclude:      []string{"internal", "files"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skill groups: %w", err)
	}
	return resp.Groups, nil
}

// PreviewSkillsOptions controls what skills are returned by PreviewSkills.
type PreviewSkillsOptions struct {
	All                bool
	IncludeUnpublished bool
}

// PreviewSkills returns the grouped skills response matching the saved sync configuration.
// It never downloads file content. Pass All=true to bypass saved filters and show everything.
func (m *Module) PreviewSkills(ctx context.Context, opts PreviewSkillsOptions) (*api.GroupedSkillsResponse, error) {
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	fetchQuery := buildFetchSkillsQuery(skillsCfg, nil)
	fetchQuery.ExcludeFiles = true

	if opts.All {
		// Bypass saved selection entirely — including selected_skills, which
		// would otherwise become skillIdentifiers and hide other ungrouped skills.
		fetchQuery.IncludeGroups = nil
		fetchQuery.ExcludeGroups = nil
		fetchQuery.SkillIdentifiers = nil
		fetchQuery.TeamsDefault = BoolPtr(false)
		fetchQuery.IncludeUngrouped = true
	}
	fetchQuery.IncludeUnpublished = opts.IncludeUnpublished

	skillQuery := api.GetSkillsQuery{
		SkillIdentifiers:   fetchQuery.SkillIdentifiers,
		IncludeGroups:      fetchQuery.IncludeGroups,
		ExcludeGroups:      fetchQuery.ExcludeGroups,
		TeamsDefault:       fetchQuery.TeamsDefault,
		Exclude:            append([]string(nil), fetchQuery.Exclude...),
		IncludeUngrouped:   fetchQuery.IncludeUngrouped,
		IncludeUnpublished: fetchQuery.IncludeUnpublished,
	}
	if fetchQuery.ExcludeFiles {
		skillQuery.Exclude = append(skillQuery.Exclude, ExcludeSkillFiles)
	}

	return m.client.GetSkillsGrouped(ctx, skillQuery)
}

// FetchSkillsWithQuery loads the sync catalog using explicit skills API query parameters.
func (m *Module) FetchSkillsWithQuery(ctx context.Context, query FetchSkillsQuery) (*FetchedSkills, error) {
	return FetchSkillsFromAPI(ctx, m.client, query)
}

// InitOptions holds options for the init operation.
type InitOptions struct {
	Targets []HookTarget
}

// InitResult holds the result of an init operation.
type InitResult struct {
	InstalledTargets []string
}

// RegisterTargets saves hook target paths without installing hooks.
func (m *Module) RegisterTargets(ctx context.Context, targets []HookTarget) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}
	skillsCfg.Targets = replaceManagedTargets(skillsCfg.Targets, TargetPaths(targets, home, cwd), home, cwd)
	skillsCfg.ProjectDirs = appendUnique(skillsCfg.ProjectDirs, cwd)
	return m.configManager.SaveSkillsConfig(skillsCfg)
}

// ConfigureSelection persists the selected skill groups and ungrouped skills
// without downloading or writing skill files.
func (m *Module) ConfigureSelection(opts LoadSkillsOptions) error {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}
	applySelectionToConfig(skillsCfg, opts)
	if err := m.configManager.SaveSkillsConfig(skillsCfg); err != nil {
		return fmt.Errorf("failed to save skills config: %w", err)
	}
	return nil
}

// Init installs hooks into the user's home directory for all selected targets,
// registers the current working directory as a project dir for project-scoped
// skills, and persists the configuration.
func (m *Module) Init(ctx context.Context, opts InitOptions) (*InitResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	if err := InstallHooks(opts.Targets, home, cwd, m.orgName); err != nil {
		return nil, fmt.Errorf("failed to install hooks: %w", err)
	}

	targetPaths := TargetPaths(opts.Targets, home, cwd)

	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	skillsCfg.Targets = replaceManagedTargets(skillsCfg.Targets, targetPaths, home, cwd)
	skillsCfg.ProjectDirs = appendUnique(skillsCfg.ProjectDirs, cwd)

	if err := m.configManager.SaveSkillsConfig(skillsCfg); err != nil {
		return nil, fmt.Errorf("failed to save skills config: %w", err)
	}

	return &InitResult{InstalledTargets: targetPaths}, nil
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func mergeUnique(existing, additions []string) []string {
	seen := make(map[string]bool, len(existing))
	for _, v := range existing {
		seen[v] = true
	}
	result := make([]string, len(existing))
	copy(result, existing)
	for _, v := range additions {
		if !seen[v] {
			result = append(result, v)
			seen[v] = true
		}
	}
	return result
}

// replaceManagedTargets reconciles saved target paths with a fresh selection.
// 'init' re-runs the full tool selection, so every target this CLI can resolve for the
// current scope (home dir + cwd) is replaced by selectedPaths — deselected tools are
// dropped. Saved paths that don't resolve to a known tool here, e.g. another
// repository's repo-scoped hook dir, are preserved so re-running init in one repo does
// not silently drop another repo's targets.
func replaceManagedTargets(saved, selectedPaths []string, home, cwd string) []string {
	managed := make(map[string]bool)
	for _, p := range TargetPaths(DefaultHookTargets(), home, cwd) {
		managed[p] = true
	}
	preserved := make([]string, 0, len(saved))
	for _, p := range saved {
		if !managed[p] {
			preserved = append(preserved, p)
		}
	}
	return mergeUnique(preserved, selectedPaths)
}

// AddSkillsOptions holds options for incrementally extending the saved selection.
type AddSkillsOptions struct {
	Groups  []string
	Skills  []string
	Targets []HookTarget
}

// AddSkillsResult summarises an add operation.
type AddSkillsResult struct {
	Merge       MergeSelectionResult
	Sync        *LoadSkillsResult
	NewTargets  []string
	InstalledOK bool
}

// AddSkills merges new groups/skills (and optionally new hook targets) into the
// saved configuration and syncs skills to disk.
func (m *Module) AddSkills(ctx context.Context, opts AddSkillsOptions) (*AddSkillsResult, error) {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	// 'add' is incremental and requires a prior 'init'. Check before mutating
	// state so a fresh-system invocation like `port skills add --tool Cursor`
	// errors out cleanly instead of installing hooks and then no-op-syncing.
	if !skillsCfg.HasSelection() && len(skillsCfg.Targets) == 0 {
		return nil, fmt.Errorf("no skills configuration found — run 'port skills init' first")
	}

	result := &AddSkillsResult{}

	if len(opts.Targets) > 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		if err := InstallHooks(opts.Targets, home, cwd, m.orgName); err != nil {
			return nil, fmt.Errorf("failed to install hooks: %w", err)
		}
		newPaths := TargetPaths(opts.Targets, home, cwd)
		skillsCfg.Targets = mergeUnique(skillsCfg.Targets, newPaths)
		skillsCfg.ProjectDirs = appendUnique(skillsCfg.ProjectDirs, cwd)
		result.NewTargets = newPaths
		result.InstalledOK = true
	}

	fetched, err := m.FetchSkillsMetadata(ctx)
	if err != nil {
		return nil, err
	}

	mergeResult, err := MergeSelection(skillsCfg, fetched, opts.Groups, opts.Skills)
	if err != nil {
		return nil, err
	}
	result.Merge = mergeResult

	if err := m.configManager.SaveSkillsConfig(skillsCfg); err != nil {
		return nil, fmt.Errorf("failed to save skills config: %w", err)
	}

	if !mergeResult.HasChanges() && len(result.NewTargets) == 0 {
		return result, nil
	}

	syncResult, err := m.LoadSkills(ctx, LoadSkillsOptions{})
	if err != nil {
		return nil, err
	}
	result.Sync = syncResult
	return result, nil
}

// RemoveSkillsOptions holds options for removing items from the saved selection.
type RemoveSkillsOptions struct {
	Groups  []string
	Skills  []string
	Targets []HookTarget
}

// RemoveSkillsResult summarises a remove operation.
type RemoveSkillsResult struct {
	Remove         RemoveSelectionResult
	Sync           *LoadSkillsResult
	RemovedTargets []string
}

// RemoveSkills drops groups/skills and/or hook targets from the saved
// configuration. Targets have their hooks uninstalled and their synced
// Port-managed skill directories deleted; remaining skills are re-synced so any
// pruned items are removed from disk on the remaining targets.
func (m *Module) RemoveSkills(ctx context.Context, opts RemoveSkillsOptions) (*RemoveSkillsResult, error) {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		return nil, fmt.Errorf("no skills configuration found — run 'port skills init' first")
	}
	if !skillsCfg.HasSelection() && len(skillsCfg.Targets) == 0 {
		return nil, fmt.Errorf("no skills configuration found — run 'port skills init' first")
	}

	result := &RemoveSkillsResult{}

	if len(opts.Targets) > 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}

		if _, err := RemoveHooks(opts.Targets, home, cwd, skillsCfg.Targets); err != nil {
			return nil, fmt.Errorf("failed to remove hooks: %w", err)
		}

		var pathsToRemove []string
		for _, savedPath := range skillsCfg.Targets {
			expanded := expandHome(savedPath)
			for _, t := range opts.Targets {
				if matchesTarget(expanded, t) {
					pathsToRemove = append(pathsToRemove, savedPath)
					break
				}
			}
		}
		for _, target := range pathsToRemove {
			if _, err := clearPortManagedSkillsDir(skillsDirForTarget(target)); err != nil {
				return nil, fmt.Errorf("failed to remove synced skills from %s: %w", target, err)
			}
		}
		skillsCfg.Targets = subtractStrings(skillsCfg.Targets, pathsToRemove)
		result.RemovedTargets = pathsToRemove
	}

	fetched, err := m.FetchSkillsMetadata(ctx)
	if err != nil {
		return nil, err
	}

	removeResult, err := RemoveSelection(skillsCfg, fetched, opts.Groups, opts.Skills)
	if err != nil {
		return nil, err
	}
	result.Remove = removeResult

	if err := m.configManager.SaveSkillsConfig(skillsCfg); err != nil {
		return nil, fmt.Errorf("failed to save skills config: %w", err)
	}

	if !removeResult.HasChanges() && len(result.RemovedTargets) == 0 {
		return result, nil
	}

	if len(skillsCfg.Targets) > 0 && skillsCfg.HasSelection() {
		syncResult, err := m.LoadSkills(ctx, LoadSkillsOptions{})
		if err != nil {
			return nil, err
		}
		result.Sync = syncResult
	}

	return result, nil
}

func subtractStrings(existing, remove []string) []string {
	rmSet := make(map[string]bool, len(remove))
	for _, p := range remove {
		rmSet[p] = true
	}
	out := make([]string, 0, len(existing))
	for _, p := range existing {
		if !rmSet[p] {
			out = append(out, p)
		}
	}
	return out
}

// LoadSkillsOptions holds options for the load-skills operation.
type LoadSkillsOptions struct {
	SelectAll          bool
	SelectAllGroups    bool
	SelectAllUngrouped bool
	SelectedGroups     []string
	SelectedSkills     []string
	IncludeGroups      []string
	ExcludeGroups      []string
	TeamGroupDefaults  bool
	// Fetched is an optional pre-fetched catalog. When set, LoadSkills skips the
	// FetchSkills API call and uses this data directly, avoiding duplicate
	// network requests when the caller already has the catalog in hand (e.g.,
	// the init command fetches once for prompts and reuses the same data for sync).
	Fetched *FetchedSkills
	// ReplaceSelection overwrites saved group/skill selection from opts instead of
	// only updating when opts carry selection fields (used by port skills select).
	ReplaceSelection bool
	// ExcludeLegacySkills omits legacy blueprint `skill` entities from the catalog fetch.
	ExcludeLegacySkills bool
	// IncludeInternalSkills includes Port built-in registry skills (excluded by default).
	IncludeInternalSkills bool
	// TargetOverrides writes to these target directories for this sync only.
	TargetOverrides []string
	// ProjectDirOverrides writes project-scoped skills under these project dirs
	// for this sync only.
	ProjectDirOverrides []string
	// NoGitignore disables best-effort .gitignore updates for project-scoped syncs.
	NoGitignore bool
	// FailOnSkillError fails the whole sync when a single skill cannot be written.
	FailOnSkillError bool
	// NoSave prevents sync-only options from being written to config.yaml.
	NoSave bool
}

// TargetResult holds the sync result for a single AI tool directory.
type TargetResult struct {
	Path       string
	GroupCount int
	SkillCount int
	IsProject  bool
	// GitHubCopilotRepo is true for a unified row under <repo>/.github/skills:
	// Port catalog "global" and "project" skills are both written there only, not
	// to a separate home-directory global path — avoid labeling as plain "global".
	GitHubCopilotRepo bool
}

// LoadSkillsResult summarises what was written.
type LoadSkillsResult struct {
	GroupCount    int
	SkillCount    int
	TargetCount   int
	TargetResults []TargetResult
	Warnings      []string
}

// LoadSkills fetches skills from Port and writes them to the appropriate targets.
// Skills with location="project" are written to the current working directory;
// all other skills are written to the configured global AI tool directories.
func (m *Module) LoadSkills(ctx context.Context, opts LoadSkillsOptions) (*LoadSkillsResult, error) {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	ApplySyncDefaults(skillsCfg)
	if opts.TargetOverrides != nil {
		skillsCfg.Targets = append([]string(nil), opts.TargetOverrides...)
	}
	if opts.ProjectDirOverrides != nil {
		skillsCfg.ProjectDirs = append([]string(nil), opts.ProjectDirOverrides...)
	}
	if len(skillsCfg.Targets) == 0 {
		return nil, fmt.Errorf("no skill targets configured; pass --tool or run 'port skills init' first")
	}
	applySelectionToConfig(skillsCfg, opts)

	fetched := opts.Fetched
	if fetched == nil {
		fetched, err = m.fetchSkills(ctx, skillsCfg, &opts)
		if err != nil {
			return nil, err
		}
	}

	skills := FilterSkills(
		fetched,
		skillsCfg.SelectAll,
		skillsCfg.SelectAllGroups,
		skillsCfg.SelectAllUngrouped,
		skillsCfg.SelectedGroups,
		skillsCfg.SelectedSkills,
		skillsCfg.UsesTeamGroupDefaults(),
	)

	globalTargets := skillsCfg.Targets
	projectDirs := skillsCfg.ProjectDirs
	projectTargets := buildProjectTargets(globalTargets, projectDirs)
	globalSkillCount := 0
	projectSkillCount := 0
	var globalSkills, projectSkills []Skill
	for _, s := range skills {
		if s.Location == SkillLocationProject {
			projectSkillCount++
			projectSkills = append(projectSkills, s)
		} else {
			globalSkillCount++
			globalSkills = append(globalSkills, s)
		}
	}
	var warnings []string

	if len(globalTargets) > 0 || len(projectDirs) > 0 {
		writeWarnings, err := WriteSkillsWithOptions(skills, fetched.Groups, globalTargets, projectDirs, WriteSkillsOptions{
			FailOnSkillError: opts.FailOnSkillError,
		})
		warnings = append(warnings, writeWarnings...)
		if err != nil {
			return nil, fmt.Errorf("failed to write skills: %w", err)
		}
	}
	if projectSkillCount > 0 && !opts.NoGitignore {
		warnings = append(warnings, ensureProjectSkillGitignores(ctx, projectTargets)...)
	}

	if !opts.NoSave {
		skillsCfg.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
		if err := m.configManager.SaveSkillsConfig(skillsCfg); err != nil {
			return nil, fmt.Errorf("failed to save skills config: %w", err)
		}
	}

	globalGroupCount := countSkillGroups(globalSkills)
	projectGroupCount := countSkillGroups(projectSkills)

	targetResults := make([]TargetResult, 0, len(globalTargets)+len(projectTargets))
	for _, t := range globalTargets {
		if isGitHubCopilotSkillRoot(t) {
			continue
		}
		if globalSkillCount == 0 {
			continue
		}
		targetResults = append(targetResults, TargetResult{
			GroupCount: globalGroupCount,
			Path:       t,
			SkillCount: globalSkillCount,
			IsProject:  false,
		})
	}
	for _, t := range projectTargets {
		if isGitHubCopilotSkillRoot(t) {
			continue
		}
		if projectSkillCount == 0 {
			continue
		}
		targetResults = append(targetResults, TargetResult{
			GroupCount: projectGroupCount,
			Path:       t,
			SkillCount: projectSkillCount,
			IsProject:  true,
		})
	}
	copilotRoots := uniqCopilotSkillRoots(append(append([]string{}, globalTargets...), projectTargets...))
	for _, root := range copilotRoots {
		if globalSkillCount+projectSkillCount == 0 {
			continue
		}
		targetResults = append(targetResults, TargetResult{
			Path:              root,
			GroupCount:        countSkillGroups(skills),
			SkillCount:        globalSkillCount + projectSkillCount,
			IsProject:         false,
			GitHubCopilotRepo: true,
		})
	}

	return &LoadSkillsResult{
		GroupCount:    countSkillGroups(skills),
		SkillCount:    len(skills),
		TargetCount:   len(globalTargets),
		TargetResults: targetResults,
		Warnings:      warnings,
	}, nil
}

func countSkillGroups(skills []Skill) int {
	groupIDs := make(map[string]bool)
	for _, skill := range skills {
		for _, groupID := range skill.GroupIDs {
			if groupID != "" {
				groupIDs[groupID] = true
			}
		}
	}
	return len(groupIDs)
}

// StatusResult contains the data surfaced by `port skills status`.
type StatusResult struct {
	Targets            []string
	ProjectDirs        []string
	SelectAll          bool
	SelectAllGroups    bool
	SelectAllUngrouped bool
	TeamGroupDefaults  bool
	IncludeGroups      []string
	ExcludeGroups      []string
	SelectedGroups     []string
	SelectedSkills     []string
	LastSyncedAt       string
}

// ClearSkillsResult summarises what was deleted.
type ClearSkillsResult struct {
	DeletedTargets []string
	SkippedTargets []string
}

// ClearSkills removes Port-managed skills from every configured AI tool target
// and project directory. Targets without Port-managed skills are silently skipped.
func (m *Module) ClearSkills() (*ClearSkillsResult, error) {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	targets := skillsCfg.Targets

	projectTargets := buildProjectTargets(targets, skillsCfg.ProjectDirs)

	allDirs := make([]string, 0, len(targets)+len(projectTargets))
	allDirs = append(allDirs, targets...)
	allDirs = append(allDirs, projectTargets...)

	result := &ClearSkillsResult{}
	for _, target := range allDirs {
		removed, err := clearPortManagedSkillsDir(skillsDirForTarget(target))
		if err != nil {
			return nil, fmt.Errorf("failed to remove skills from %s: %w", target, err)
		}
		if !removed {
			result.SkippedTargets = append(result.SkippedTargets, target)
			continue
		}
		result.DeletedTargets = append(result.DeletedTargets, target)
	}

	return result, nil
}

// RemoveResult summarises what was removed by a full skills/hooks uninstall.
type RemoveResult struct {
	HooksResult  *RemoveHooksResult
	SkillsResult *ClearSkillsResult
}

// Remove uninstalls hooks, local synced skills, and clears skills config:
//   - Port hook entries from hooks.json / settings.json (other hooks preserved)
//   - Local Port-managed skill directories
//   - The skills section from ~/.port/config.yaml
func (m *Module) Remove() (*RemoveResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}

	hooksResult, err := RemoveHooks(DefaultHookTargets(), home, cwd, skillsCfg.Targets)
	if err != nil {
		return nil, fmt.Errorf("failed to remove hooks: %w", err)
	}

	skillsResult, err := m.ClearSkills()
	if err != nil {
		return nil, fmt.Errorf("failed to clear skills: %w", err)
	}

	if err := m.configManager.SaveSkillsConfig(&config.SkillsConfig{}); err != nil {
		return nil, fmt.Errorf("failed to clear skills config: %w", err)
	}

	return &RemoveResult{
		HooksResult:  hooksResult,
		SkillsResult: skillsResult,
	}, nil
}

// Status returns the current skills configuration state.
func (m *Module) Status() (*StatusResult, error) {
	skillsCfg, err := m.configManager.LoadSkillsConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load skills config: %w", err)
	}

	return &StatusResult{
		Targets:            skillsCfg.Targets,
		ProjectDirs:        skillsCfg.ProjectDirs,
		SelectAll:          skillsCfg.SelectAll,
		SelectAllGroups:    skillsCfg.SelectAllGroups,
		SelectAllUngrouped: skillsCfg.SelectAllUngrouped,
		TeamGroupDefaults:  skillsCfg.UsesTeamGroupDefaults(),
		IncludeGroups:      skillsCfg.IncludeGroups,
		ExcludeGroups:      skillsCfg.ExcludeGroups,
		SelectedGroups:     skillsCfg.SelectedGroups,
		SelectedSkills:     skillsCfg.SelectedSkills,
		LastSyncedAt:       skillsCfg.LastSyncedAt,
	}, nil
}

// isGitHubCopilotSkillRoot reports whether absPath is the GitHub Copilot
// repository skill root (…/.github), i.e. where Port writes Copilot skills.
func isGitHubCopilotSkillRoot(absPath string) bool {
	exp := filepath.Clean(expandHome(absPath))
	for _, t := range DefaultHookTargets() {
		if t.Name != "GitHub Copilot" {
			continue
		}
		if matchesTarget(exp, t) {
			return true
		}
	}
	return false
}

// uniqCopilotSkillRoots returns deduplicated paths from candidates that are
// GitHub Copilot skill roots, sorted for stable output.
func uniqCopilotSkillRoots(candidates []string) []string {
	byCanon := make(map[string]string)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		exp := filepath.Clean(expandHome(p))
		if !isGitHubCopilotSkillRoot(exp) {
			continue
		}
		can := filepath.Clean(exp)
		if _, ok := byCanon[can]; !ok {
			byCanon[can] = p
		}
	}
	out := make([]string, 0, len(byCanon))
	for _, orig := range byCanon {
		out = append(out, orig)
	}
	sort.Strings(out)
	return out
}
