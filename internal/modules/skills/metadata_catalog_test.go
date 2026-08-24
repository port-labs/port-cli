package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
)

func TestMetadataCatalogQueryFetchesFullCatalog(t *testing.T) {
	query := MetadataCatalogQuery()
	if query.TeamsDefault == nil || *query.TeamsDefault {
		t.Fatalf("TeamsDefault = %v, want false", query.TeamsDefault)
	}
	if !query.ExcludeFiles {
		t.Fatal("ExcludeFiles = false, want true")
	}
	if !query.IncludeUngrouped {
		t.Fatal("IncludeUngrouped = false, want true")
	}
	if len(query.Exclude) != 1 || query.Exclude[0] != "internal" {
		t.Fatalf("Exclude = %v, want [internal]", query.Exclude)
	}
}

func TestFetchSkillsMetadata_UsesFullCatalogQuery(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/skills" {
			http.NotFound(w, r)
			return
		}
		rawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{OK: true})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	mod := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       srv.URL,
	}, cm, "")

	if _, err := mod.FetchSkillsMetadata(context.Background()); err != nil {
		t.Fatalf("FetchSkillsMetadata: %v", err)
	}
	if !strings.Contains(rawQuery, "teams_default=false") {
		t.Fatalf("query %q missing teams_default=false", rawQuery)
	}
	if !strings.Contains(rawQuery, "include_ungrouped=true") {
		t.Fatalf("query %q missing include_ungrouped=true", rawQuery)
	}
	if !strings.Contains(rawQuery, "exclude=internal") {
		t.Fatalf("query %q missing exclude=internal", rawQuery)
	}
	if !strings.Contains(rawQuery, "exclude=files") {
		t.Fatalf("query %q missing exclude=files", rawQuery)
	}
}

func TestAddSkills_AcceptsUngroupedSkillFromFullCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/skills" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{
			OK: true,
			UngroupedSkills: []api.SkillAtLatestVersion{{
				Identifier: "api-readiness",
				Title:      "API Readiness",
				Location:   "global",
				Version:    "1.0.0",
				Files: []api.SkillFile{{
					Properties: map[string]interface{}{
						"path":    "SKILL.md",
						"content": "# API Readiness\n",
					},
				}},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	mod, cm, tmpDir := newTestModuleWithAPI(t, srv.URL)
	target := filepath.Join(tmpDir, ".agents")
	writeCfg(t, cm, &config.SkillsConfig{
		Targets:           []string{target},
		TeamGroupDefaults: true,
	})

	result, err := mod.AddSkills(context.Background(), AddSkillsOptions{
		Skills: []string{"api-readiness"},
	})
	if err != nil {
		t.Fatalf("AddSkills: %v", err)
	}
	if len(result.Merge.AddedSkills) != 1 || result.Merge.AddedSkills[0] != "api-readiness" {
		t.Fatalf("AddedSkills = %v, want [api-readiness]", result.Merge.AddedSkills)
	}

	cfg, err := cm.LoadSkillsConfig()
	if err != nil {
		t.Fatalf("LoadSkillsConfig: %v", err)
	}
	if !contains(cfg.SelectedSkills, "api-readiness") {
		t.Fatalf("SelectedSkills = %v, want api-readiness", cfg.SelectedSkills)
	}
}

func TestAddSkills_UnknownSkillStillErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{
			OK: true,
			UngroupedSkills: []api.SkillAtLatestVersion{{
				Identifier: "other-skill",
				Title:      "Other",
				Location:   "global",
				Version:    "1.0.0",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	mod, cm, tmpDir := newTestModuleWithAPI(t, srv.URL)
	writeCfg(t, cm, &config.SkillsConfig{
		Targets:           []string{filepath.Join(tmpDir, ".agents")},
		TeamGroupDefaults: true,
	})

	_, err := mod.AddSkills(context.Background(), AddSkillsOptions{
		Skills: []string{"api-readiness"},
	})
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	if !strings.Contains(err.Error(), "unknown selection: skill:api-readiness") {
		t.Fatalf("error = %v, want unknown selection for api-readiness", err)
	}
}

func newTestModuleWithAPI(t *testing.T, apiURL string) (*Module, *config.ConfigManager, string) {
	t.Helper()
	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	mod := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       apiURL,
	}, cm, "")
	return mod, cm, dir
}
