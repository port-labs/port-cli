package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
)

// Module handles exporting data from Port.
type Module struct {
	client *api.Client
}

// NewModule creates a new export module.
func NewModule(token *auth.Token, orgConfig *config.OrganizationConfig) *Module {
	client := api.NewClient(api.ClientOpts{
		Token:        token,
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Timeout:      0,
	})
	return &Module{
		client: client,
	}
}

// Result represents the result of an export operation.
type Result struct {
	Success           bool
	Message           string
	OutputPath        string
	BlueprintsCount   int
	EntitiesCount     int
	ActionsCount      int
	PagesCount        int
	IntegrationsCount int
	UsersCount        int
	TeamsCount        int
	FoldersCount      int
	Format            string
	TimeoutErrors     []string // Blueprints that timed out during export
	Error             error
}

// Execute performs the export operation.
func (m *Module) Execute(ctx context.Context, opts Options) (*Result, error) {
	// Validate options
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	// Collect non-entity data concurrently. Entity data can be much larger than
	// the rest of the export, so it is streamed directly to the archive below.
	collector := NewCollector(m.client)
	metadataOpts := opts
	metadataOpts.SkipEntities = true
	data, err := collector.Collect(ctx, metadataOpts)
	if err != nil {
		return &Result{
			Success: false,
			Message: "Export failed",
			Error:   err,
		}, nil
	}

	// Write output
	formatType := opts.Format
	if formatType == "" {
		// Determine format from file extension
		ext := strings.ToLower(filepath.Ext(opts.OutputPath))
		if ext == ".json" {
			formatType = "json"
		} else {
			formatType = "tar"
		}
	}

	entitiesCount, timeoutErrors, err := m.writeStreamingExport(ctx, data, opts, formatType)
	if err != nil {
		return &Result{
			Success: false,
			Message: "Export failed",
			Error:   fmt.Errorf("failed to write output: %w", err),
		}, nil
	}
	data.TimeoutErrors = append(data.TimeoutErrors, timeoutErrors...)

	return &Result{
		Success:           true,
		Message:           fmt.Sprintf("Successfully exported data to %s", opts.OutputPath),
		OutputPath:        opts.OutputPath,
		BlueprintsCount:   len(data.Blueprints),
		EntitiesCount:     entitiesCount,
		ActionsCount:      len(data.Actions),
		PagesCount:        len(data.Pages),
		IntegrationsCount: len(data.Integrations),
		UsersCount:        len(data.Users),
		TeamsCount:        len(data.Teams),
		FoldersCount:      len(data.Folders),
		Format:            formatType,
		TimeoutErrors:     data.TimeoutErrors,
	}, nil
}

func (m *Module) writeStreamingExport(ctx context.Context, data *Data, opts Options, formatType string) (int, []string, error) {
	writer, err := newArchiveWriter(formatType, opts.OutputPath)
	if err != nil {
		return 0, nil, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = writer.Close()
		}
	}()

	// Entities are streamed BEFORE "blueprints" is written: when
	// AutoScopeBlueprints is set, streaming records which blueprints actually
	// had a matching entity into data.ReferencedBlueprintIDs, and that has to
	// happen before blueprints are narrowed below. Resource write order has no
	// effect on the archive's correctness (see archive_writer.go — each
	// resource is its own tar entry / JSON object key, read back by name).
	entitiesCount := 0
	timeoutErrors := []string{}
	if shouldStreamEntities(opts) {
		count, errs, err := m.writeEntities(ctx, writer, opts, data)
		if err != nil {
			return 0, nil, err
		}
		entitiesCount = count
		timeoutErrors = errs
	} else if err := writer.WriteResource("entities", []api.Entity{}); err != nil {
		return 0, nil, err
	}

	if opts.AutoScopeBlueprints && shouldCollect("blueprints", opts.IncludeResources) {
		data.Blueprints = FilterBlueprintsToReferenced(data.Blueprints, data.ReferencedBlueprintIDs)
	}
	if err := writer.WriteResource("blueprints", data.Blueprints); err != nil {
		return 0, nil, err
	}

	resources := []struct {
		name  string
		value interface{}
	}{
		{"scorecards", data.Scorecards},
		{"actions", data.Actions},
		{"teams", data.Teams},
		{"users", data.Users},
		{"_folders", data.Folders},
		{"pages", data.Pages},
		{"integrations", data.Integrations},
		{"blueprint_permissions", data.BlueprintPermissions},
		{"action_permissions", data.ActionPermissions},
		{"page_permissions", data.PagePermissions},
	}
	for _, resource := range resources {
		if err := writer.WriteResource(resource.name, resource.value); err != nil {
			return 0, nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return 0, nil, err
	}
	closed = true
	return entitiesCount, timeoutErrors, nil
}

func shouldStreamEntities(opts Options) bool {
	return !opts.SkipEntities && shouldCollect("entities", opts.IncludeResources)
}

func (m *Module) writeEntities(ctx context.Context, writer ArchiveWriter, opts Options, data *Data) (int, []string, error) {
	blueprints, err := m.blueprintsForEntityStreaming(ctx, opts)
	if err != nil {
		return 0, nil, err
	}

	entitySet := make(map[string]bool, len(opts.Entities))
	for _, id := range opts.Entities {
		entitySet[id] = true
	}

	total := 0
	timeoutErrors := []string{}
	err = writer.WriteEntities(func(sink EntitySink) error {
		for _, bp := range blueprints {
			bpID, _ := bp["identifier"].(string)
			if bpID == "" {
				continue
			}
			err := m.client.ForEachEntity(ctx, bpID, func(entities []api.Entity) error {
				for _, entity := range entities {
					if len(entitySet) > 0 {
						id, _ := entity["identifier"].(string)
						if !entitySet[id] {
							continue
						}
					}
					if err := sink.WriteEntity(entity); err != nil {
						return err
					}
					total++
					if opts.AutoScopeBlueprints {
						data.ReferencedBlueprintIDs[bpID] = true
					}
				}
				return nil
			})
			if err != nil {
				if strings.Contains(err.Error(), "410 Gone") {
					continue
				}
				return fmt.Errorf("failed to get entities for blueprint %s: %w", bpID, err)
			}
		}
		return nil
	})
	return total, timeoutErrors, err
}

func (m *Module) blueprintsForEntityStreaming(ctx context.Context, opts Options) ([]api.Blueprint, error) {
	allBlueprints, err := m.client.GetBlueprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blueprints: %w", err)
	}

	blueprints := allBlueprints
	if len(opts.Blueprints) > 0 {
		blueprintSet := make(map[string]bool, len(opts.Blueprints))
		for _, bpID := range opts.Blueprints {
			blueprintSet[bpID] = true
		}
		blueprints = nil
		for _, bp := range allBlueprints {
			if identifier, ok := bp["identifier"].(string); ok && blueprintSet[identifier] {
				blueprints = append(blueprints, bp)
			}
		}
	}

	excludeDeep := append([]string{}, opts.ExcludeBlueprints...)
	if !opts.IncludeRuleResults {
		excludeDeep = append(excludeDeep, "_rule_result")
	}
	if opts.SkipSystemBlueprints {
		for _, bp := range blueprints {
			id, _ := bp["identifier"].(string)
			if strings.HasPrefix(id, "_") {
				excludeDeep = append(excludeDeep, id)
			}
		}
	}
	iterBlueprints, _ := ApplyBlueprintExclusions(blueprints, excludeDeep, opts.ExcludeBlueprintSchema)
	return iterBlueprints, nil
}

// Close closes the API client.
func (m *Module) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// writeTar writes data to a tar.gz file.
func writeTar(data *Data, outputPath string) error {
	writer, err := newTarArchiveWriter(outputPath)
	if err != nil {
		return err
	}
	return writeDataArchive(data, writer)
}

// WriteJSON encodes the Data as indented JSON into w.
func (d *Data) WriteJSON(w io.Writer) error {
	output := map[string]interface{}{
		"blueprints":            d.Blueprints,
		"entities":              d.Entities,
		"scorecards":            d.Scorecards,
		"actions":               d.Actions,
		"teams":                 d.Teams,
		"users":                 d.Users,
		"_folders":              d.Folders,
		"pages":                 d.Pages,
		"integrations":          d.Integrations,
		"blueprint_permissions": d.BlueprintPermissions,
		"action_permissions":    d.ActionPermissions,
		"page_permissions":      d.PagePermissions,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// writeJSON writes data to a JSON file.
func writeJSON(data *Data, outputPath string) error {
	writer, err := newJSONArchiveWriter(outputPath)
	if err != nil {
		return err
	}
	return writeDataArchive(data, writer)
}

func writeDataArchive(data *Data, writer ArchiveWriter) error {
	resources := []struct {
		name  string
		value interface{}
	}{
		{"blueprints", data.Blueprints},
		{"entities", data.Entities},
		{"scorecards", data.Scorecards},
		{"actions", data.Actions},
		{"teams", data.Teams},
		{"users", data.Users},
		{"_folders", data.Folders},
		{"pages", data.Pages},
		{"integrations", data.Integrations},
		{"blueprint_permissions", data.BlueprintPermissions},
		{"action_permissions", data.ActionPermissions},
		{"page_permissions", data.PagePermissions},
	}
	for _, resource := range resources {
		if err := writer.WriteResource(resource.name, resource.value); err != nil {
			writer.Close()
			return err
		}
	}
	return writer.Close()
}
