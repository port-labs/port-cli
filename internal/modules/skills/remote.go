package skills

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
)

// SearchSkills finds skills by identifier or title (GET /skills/search).
func (m *Module) SearchSkills(ctx context.Context, query api.SearchSkillsQuery) ([]api.SkillCatalogEntry, error) {
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("search query is required")
	}
	resp, err := m.client.SearchSkills(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search skills: %w", err)
	}
	entries := append([]api.SkillCatalogEntry(nil), resp.Skills...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Skill.Identifier < entries[j].Skill.Identifier
	})
	return entries, nil
}

// UploadSkillFromFolder uploads a skill via POST /skills/upload.
func (m *Module) UploadSkillFromFolder(ctx context.Context, folder string, opts PackSkillFolderOptions, writeOpts UploadSkillWriteOptions) (*api.SkillVersionWriteResponse, error) {
	pack, err := PackSkillFolder(folder, opts)
	if err != nil {
		return nil, err
	}
	return m.UploadSkillFromPack(ctx, pack, filepath.Base(folder), writeOpts)
}

// UploadSkillWriteOptions controls version creation when uploading skills.
type UploadSkillWriteOptions struct {
	Publish  bool
	GroupIDs []string

	VersionBump api.VersionBump
}

// UploadSkillFromPack uploads a packed skill folder via POST /skills/upload.
func (m *Module) UploadSkillFromPack(ctx context.Context, pack *SkillFolderPack, folderBase string, writeOpts UploadSkillWriteOptions) (*api.SkillVersionWriteResponse, error) {
	if pack == nil {
		return nil, fmt.Errorf("skill pack is required")
	}
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	return m.client.UploadSkill(ctx, uploadRequestFromPack(pack, folderBase, writeOpts))
}

// SkillPackWithFolder pairs a packed skill with its source folder basename.
type SkillPackWithFolder struct {
	Pack       *SkillFolderPack
	FolderBase string
}

// UploadSkillsBatch uploads multiple skills via POST /skills/upload/batch.
func (m *Module) UploadSkillsBatch(ctx context.Context, packs []SkillPackWithFolder, writeOpts UploadSkillWriteOptions) (*api.BatchUploadSkillsResponse, error) {
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	if len(packs) == 0 {
		return nil, fmt.Errorf("at least one skill is required")
	}
	skills := make([]api.UploadSkillRequest, 0, len(packs))
	for _, item := range packs {
		if item.Pack == nil {
			return nil, fmt.Errorf("skill pack is required")
		}
		skills = append(skills, uploadRequestFromPack(item.Pack, item.FolderBase, writeOpts))
	}
	resp, err := m.client.UploadSkillsBatch(ctx, api.BatchUploadSkillsRequest{Skills: skills})
	if err != nil {
		return nil, fmt.Errorf("failed to batch upload skills: %w", err)
	}
	return resp, nil
}

// FetchSkill loads one published skill.
func (m *Module) FetchSkill(ctx context.Context, identifier string) (Skill, error) {
	if m.client == nil {
		return Skill{}, fmt.Errorf("API client is not configured")
	}
	resp, err := m.client.GetSkill(ctx, identifier)
	if err != nil {
		return Skill{}, fmt.Errorf("failed to fetch skill %q: %w", identifier, err)
	}
	return skillFromAPI(resp.Skill, nil), nil
}

// PublishSkill sets the active version to the latest semver in Port.
func (m *Module) PublishSkill(ctx context.Context, identifier string) (*api.SkillVersionWriteResponse, error) {
	if m.client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	resp, err := m.client.PublishSkill(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to publish skill %q: %w", identifier, err)
	}
	return resp, nil
}

// UnpublishSkill clears the active version in Port.
func (m *Module) UnpublishSkill(ctx context.Context, identifier string) error {
	if m.client == nil {
		return fmt.Errorf("API client is not configured")
	}
	_, err := m.client.UnpublishSkill(ctx, identifier)
	if err != nil {
		return fmt.Errorf("failed to unpublish skill %q: %w", identifier, err)
	}
	return nil
}

func uploadRequestFromPack(pack *SkillFolderPack, folderBase string, writeOpts UploadSkillWriteOptions) api.UploadSkillRequest {
	return api.UploadSkillRequest{
		Identifier:     pack.Identifier,
		Title:          pack.Title,
		Description:    pack.Description,
		Location:       pack.Location,
		Publish:        writeOpts.Publish,
		VersionBump:    writeOpts.VersionBump,
		GroupIDs:       append([]string(nil), writeOpts.GroupIDs...),
		FolderBaseName: folderBase,
		Files:          pack.Files,
	}
}
