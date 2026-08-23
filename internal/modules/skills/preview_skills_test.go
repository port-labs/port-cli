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

func TestPreviewSkills_AllClearsSelectedSkillIdentifiers(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{
			OK: true,
			UngroupedSkills: []api.SkillAtLatestVersion{
				{Identifier: "integrations-overview", Title: "Integrations overview", Location: "global", Version: "1.0.0"},
				{Identifier: "incident-triage", Title: "Incident triage", Location: "global"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	writeCfg(t, cm, &config.SkillsConfig{
		Targets:            []string{filepath.Join(dir, ".agents")},
		TeamGroupDefaults:  true,
		IncludeGroups:      []string{"operations"},
		SelectedSkills:     []string{"integrations-overview"},
		SelectAllUngrouped: false,
	})
	mod := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       srv.URL,
	}, cm, "")

	resp, err := mod.PreviewSkills(context.Background(), PreviewSkillsOptions{All: true})
	if err != nil {
		t.Fatalf("PreviewSkills: %v", err)
	}
	if strings.Contains(rawQuery, "skill_identifier") || strings.Contains(rawQuery, "skillIdentifiers") ||
		strings.Contains(rawQuery, "integrations-overview") {
		t.Fatalf("--all must not send selected skill identifiers; query=%q", rawQuery)
	}
	if !strings.Contains(rawQuery, "include_ungrouped=true") {
		t.Fatalf("query %q missing include_ungrouped=true", rawQuery)
	}
	if !strings.Contains(rawQuery, "teams_default=false") {
		t.Fatalf("query %q missing teams_default=false", rawQuery)
	}
	if len(resp.UngroupedSkills) != 2 {
		t.Fatalf("UngroupedSkills = %d, want 2 (full catalog)", len(resp.UngroupedSkills))
	}
}

func TestPreviewSkills_WithoutAllKeepsSelectedSkillIdentifiers(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		rawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{OK: true})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	writeCfg(t, cm, &config.SkillsConfig{
		Targets:           []string{filepath.Join(dir, ".agents")},
		TeamGroupDefaults: true,
		SelectedSkills:    []string{"integrations-overview"},
	})
	mod := NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       srv.URL,
	}, cm, "")

	if _, err := mod.PreviewSkills(context.Background(), PreviewSkillsOptions{All: false}); err != nil {
		t.Fatalf("PreviewSkills: %v", err)
	}
	if !strings.Contains(rawQuery, "integrations-overview") {
		t.Fatalf("filtered list should keep selected skill identifiers; query=%q", rawQuery)
	}
}
