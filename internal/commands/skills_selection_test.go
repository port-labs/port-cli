package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/skills"
)

func TestBuildNonInteractiveSelectLoadOpts_GroupPersistsTeamDefaults(t *testing.T) {
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
			Groups: []api.SkillGroupAtLatestVersion{{
				Identifier:       "coding",
				Title:            "coding",
				MatchesUserTeams: false,
				Skills: []api.SkillAtLatestVersion{{
					Identifier: "platform-triage",
					Title:      "Platform Triage",
					Location:   "global",
					Version:    "1.0.0",
				}},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	mod := skills.NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       srv.URL,
	}, cm, "")

	loadOpts, fetched, err := buildNonInteractiveSelectLoadOpts(
		context.Background(),
		mod,
		cm,
		[]string{"coding"},
		nil,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("buildNonInteractiveSelectLoadOpts: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected fetched catalog")
	}
	if !loadOpts.TeamGroupDefaults {
		t.Fatal("expected TeamGroupDefaults true")
	}
	if len(loadOpts.IncludeGroups) != 1 || loadOpts.IncludeGroups[0] != "coding" {
		t.Fatalf("IncludeGroups = %v, want [coding]", loadOpts.IncludeGroups)
	}
	if len(loadOpts.SelectedGroups) != 0 {
		t.Fatalf("SelectedGroups = %v, want empty in team mode", loadOpts.SelectedGroups)
	}

	if err := mod.ConfigureSelection(loadOpts); err != nil {
		t.Fatalf("ConfigureSelection: %v", err)
	}
	cfg, err := cm.LoadSkillsConfig()
	if err != nil {
		t.Fatalf("LoadSkillsConfig: %v", err)
	}
	if !cfg.TeamGroupDefaults {
		t.Fatal("expected team_group_defaults true in saved config")
	}
	if len(cfg.IncludeGroups) != 1 || cfg.IncludeGroups[0] != "coding" {
		t.Fatalf("include_groups = %v, want [coding]", cfg.IncludeGroups)
	}
	if len(cfg.SelectedGroups) != 0 {
		t.Fatalf("selected_groups = %v, want empty", cfg.SelectedGroups)
	}
}

func TestBuildNonInteractiveSelectLoadOpts_RequiresSelectionFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		_ = json.NewEncoder(w).Encode(api.GroupedSkillsResponse{OK: true, Groups: nil})
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cm := config.NewConfigManager(filepath.Join(dir, "config.yaml"))
	mod := skills.NewModule(nil, &config.OrganizationConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		APIURL:       srv.URL,
	}, cm, "")

	_, _, err := buildNonInteractiveSelectLoadOpts(
		context.Background(),
		mod,
		cm,
		nil,
		nil,
		false,
		false,
	)
	if err == nil {
		t.Fatal("expected error when no selection flags provided")
	}
}
