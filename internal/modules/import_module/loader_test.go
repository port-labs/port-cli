package import_module

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
)

func TestLoader_LoadJSON_IncludesFolders(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")

	content := `{
  "blueprints": [{"identifier":"service","title":"Service"}],
  "_folders": [{"identifier":"quorum","title":"Quorum","after":"catalog_tables"}],
  "pages": [{"identifier":"service_overview","title":"Service Overview"}]
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	loader := NewLoader()
	data, err := loader.LoadData(inputPath)
	if err != nil {
		t.Fatalf("LoadData error: %v", err)
	}

	if len(data.Folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(data.Folders))
	}
	if data.Folders[0]["identifier"] != "quorum" {
		t.Fatalf("expected quorum folder identifier, got %v", data.Folders[0]["identifier"])
	}
	if len(data.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(data.Pages))
	}
}

func TestCleanFolderForCreate(t *testing.T) {
	folder := map[string]interface{}{
		"identifier":  "quorum",
		"title":       "Quorum",
		"after":       "catalog_tables",
		"parent":      "root_catalog",
		"sidebarType": "folder",
		"id":          "internal-id",
	}

	cleaned := CleanFolderForCreate(folder)

	if len(cleaned) != 4 {
		t.Fatalf("expected only identifier/title/after/parent, got %v", cleaned)
	}
	if cleaned["identifier"] != "quorum" || cleaned["title"] != "Quorum" || cleaned["after"] != "catalog_tables" || cleaned["parent"] != "root_catalog" {
		t.Fatalf("unexpected cleaned folder payload: %v", cleaned)
	}
	if _, exists := cleaned["sidebarType"]; exists {
		t.Fatalf("expected sidebarType to be stripped, got %v", cleaned)
	}
}

func TestSortFoldersByAfterLevels(t *testing.T) {
	levels := SortFoldersByAfterLevels([]api.Folder{
		{"identifier": "child", "parent": "root"},
		{"identifier": "sibling", "after": "child"},
		{"identifier": "root"},
		{"identifier": "leaf", "parent": "child", "after": "sibling"},
	})

	if len(levels) != 4 {
		t.Fatalf("expected 4 levels, got %d", len(levels))
	}

	order := []string{
		levels[0][0]["identifier"].(string),
		levels[1][0]["identifier"].(string),
		levels[2][0]["identifier"].(string),
		levels[3][0]["identifier"].(string),
	}
	if strings.Join(order, ",") != "root,child,sibling,leaf" {
		t.Fatalf("unexpected folder order: %v", order)
	}
}

func TestPlanSidebarPipeline_MixedFolderAndPageDependencies(t *testing.T) {
	pipeline := PlanSidebarPipeline(
		[]api.Folder{
			{"identifier": "catalog_root", "title": "Catalog Root"},
			{"identifier": "after_page_folder", "title": "After Page Folder", "after": "service_overview"},
		},
		[]api.Page{
			{"identifier": "service_overview", "title": "Service Overview", "parent": "catalog_root"},
			{"identifier": "service_details", "title": "Service Details", "parent": "after_page_folder"},
		},
	)

	lines := DescribeSidebarPipeline(pipeline)
	expected := []string{
		"Step 1: folders [catalog_root]",
		"Step 2: pages [service_overview]",
		"Step 3: folders [after_page_folder]",
		"Step 4: pages [service_details]",
	}

	if strings.Join(lines, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unexpected pipeline:\n%s", strings.Join(lines, "\n"))
	}
}

func TestLoader_LoadJSON_PagePermissions(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")

	content := `{
  "blueprints": [],
  "page_permissions": {"home": {"read": {"roles": ["Admin"]}}}
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	loader := NewLoader()
	data, err := loader.LoadData(inputPath)
	if err != nil {
		t.Fatalf("LoadData error: %v", err)
	}

	if _, ok := data.PagePermissions["home"]; !ok {
		t.Error("expected page_permissions key to be loaded")
	}
}

func TestStreamLoader_LoadMetadataAndIteratesEntities(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")

	content := `{
  "blueprints": [{"identifier":"service","title":"Service"}],
  "entities": [
    {"identifier":"svc-1","blueprint":"service"},
    {"identifier":"svc-2","blueprint":"service"}
  ],
  "pages": [{"identifier":"home"}]
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	loader := NewStreamLoader()
	data, err := loader.LoadDataWithoutEntities(inputPath)
	if err != nil {
		t.Fatalf("LoadDataWithoutEntities error: %v", err)
	}
	if len(data.Blueprints) != 1 {
		t.Fatalf("expected 1 blueprint, got %d", len(data.Blueprints))
	}
	if len(data.Entities) != 0 {
		t.Fatalf("expected metadata load to skip entities, got %d", len(data.Entities))
	}
	if len(data.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(data.Pages))
	}

	var ids []string
	if err := loader.ForEachEntity(inputPath, func(entity api.Entity) error {
		id, _ := entity["identifier"].(string)
		ids = append(ids, id)
		return nil
	}); err != nil {
		t.Fatalf("ForEachEntity error: %v", err)
	}
	if strings.Join(ids, ",") != "svc-1,svc-2" {
		t.Fatalf("unexpected entity ids: %v", ids)
	}
}

func TestStreamLoader_TarMetadataAndEntities(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.tar.gz")
	if err := writeTestTarExport(inputPath); err != nil {
		t.Fatalf("write tar export: %v", err)
	}

	loader := NewStreamLoader()
	data, err := loader.LoadDataWithoutEntities(inputPath)
	if err != nil {
		t.Fatalf("LoadDataWithoutEntities error: %v", err)
	}
	if len(data.Blueprints) != 1 {
		t.Fatalf("expected 1 blueprint, got %d", len(data.Blueprints))
	}
	if len(data.Entities) != 0 {
		t.Fatalf("expected metadata load to skip entities, got %d", len(data.Entities))
	}

	var count int
	if err := loader.ForEachEntity(inputPath, func(entity api.Entity) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("ForEachEntity error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 entities, got %d", count)
	}
}

func writeTestTarExport(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gzw := gzip.NewWriter(file)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	entries := map[string]interface{}{
		"blueprints.json": []api.Blueprint{{"identifier": "service"}},
		"entities.json": []api.Entity{
			{"identifier": "svc-1", "blueprint": "service"},
			{"identifier": "svc-2", "blueprint": "service"},
		},
	}
	for name, value := range entries {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(value); err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(buf.Len()), Mode: 0o644}); err != nil {
			return err
		}
		if _, err := io.Copy(tw, &buf); err != nil {
			return err
		}
	}
	return nil
}

func TestLoader_LoadJSON_LegacyPascalCasePermissions(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")

	content := `{
  "blueprints": [],
  "BlueprintPermissions": {"svc": {"read": "everyone"}},
  "ActionPermissions": {"act1": {"execute": "admins"}}
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	loader := NewLoader()
	data, err := loader.LoadData(inputPath)
	if err != nil {
		t.Fatalf("LoadData error: %v", err)
	}

	if _, ok := data.BlueprintPermissions["svc"]; !ok {
		t.Error("expected legacy BlueprintPermissions key to be loaded")
	}
	if _, ok := data.ActionPermissions["act1"]; !ok {
		t.Error("expected legacy ActionPermissions key to be loaded")
	}
}

func TestLoader_LoadJSON_SnakeCasePermissions(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "export.json")

	content := `{
  "blueprints": [],
  "blueprint_permissions": {"svc": {"read": "everyone"}},
  "action_permissions": {"act1": {"execute": "admins"}}
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	loader := NewLoader()
	data, err := loader.LoadData(inputPath)
	if err != nil {
		t.Fatalf("LoadData error: %v", err)
	}

	if _, ok := data.BlueprintPermissions["svc"]; !ok {
		t.Error("expected snake_case blueprint_permissions key to be loaded")
	}
	if _, ok := data.ActionPermissions["act1"]; !ok {
		t.Error("expected snake_case action_permissions key to be loaded")
	}
}

func TestValidateData_FullImport_RequiresBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{Blueprints: []api.Blueprint{}}
	err := loader.ValidateData(data, nil)
	if err == nil {
		t.Error("expected error for full import with no blueprints")
	}
}

func TestValidateData_FullImport_PassesWithBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{Blueprints: []api.Blueprint{{"identifier": "svc"}}}
	err := loader.ValidateData(data, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateData_PagesOnly_PassesWithoutBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{
		Blueprints: []api.Blueprint{},
		Pages:      []api.Page{{"identifier": "home"}},
	}
	err := loader.ValidateData(data, []string{"pages"})
	if err != nil {
		t.Errorf("pages-only import should not require blueprints: %v", err)
	}
}

func TestValidateData_ActionsOnly_PassesWithoutBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{
		Blueprints: []api.Blueprint{},
		Actions:    []api.Action{{"identifier": "deploy"}},
	}
	err := loader.ValidateData(data, []string{"actions"})
	if err != nil {
		t.Errorf("actions-only import should not require blueprints: %v", err)
	}
}

func TestValidateData_IntegrationsOnly_PassesWithoutBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{
		Blueprints:   []api.Blueprint{},
		Integrations: []api.Integration{{"installationId": "int1"}},
	}
	err := loader.ValidateData(data, []string{"integrations"})
	if err != nil {
		t.Errorf("integrations-only import should not require blueprints: %v", err)
	}
}

func TestValidateData_TeamsOnly_PassesWithoutBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{
		Blueprints: []api.Blueprint{},
		Teams:      []api.Team{{"name": "Backend"}},
	}
	err := loader.ValidateData(data, []string{"teams"})
	if err != nil {
		t.Errorf("teams-only import should not require blueprints: %v", err)
	}
}

func TestValidateData_BlueprintDependent_StillRequiresBlueprints(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{Blueprints: []api.Blueprint{}}

	for _, resource := range []string{"blueprints", "entities", "scorecards", "blueprint-permissions"} {
		err := loader.ValidateData(data, []string{resource})
		if err == nil {
			t.Errorf("--include %s should still require blueprints in file", resource)
		}
	}
}

func TestValidateData_MixedInclude_RequiresBlueprintsWhenNeeded(t *testing.T) {
	loader := NewLoader()
	data := &export.Data{Blueprints: []api.Blueprint{}}
	err := loader.ValidateData(data, []string{"pages", "entities"})
	if err == nil {
		t.Error("mixed include with entities should require blueprints")
	}
}
