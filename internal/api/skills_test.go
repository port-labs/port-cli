package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetSkillsGrouped_ExcludeFilesQuery(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"groups":[],"ungroupedSkills":[]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(ClientOpts{APIURL: srv.URL})
	_, err := client.GetSkillsGrouped(context.Background(), GetSkillsQuery{
		Exclude: []string{"files"},
	})
	if err != nil {
		t.Fatalf("GetSkillsGrouped: %v", err)
	}
	if !strings.Contains(rawQuery, "exclude=files") {
		t.Fatalf("query %q missing exclude=files", rawQuery)
	}
}

func TestGetSkillsGrouped_ExcludeInternalQuery(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"groups":[],"ungroupedSkills":[]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(ClientOpts{APIURL: srv.URL})
	_, err := client.GetSkillsGrouped(context.Background(), GetSkillsQuery{
		Exclude: []string{"internal"},
	})
	if err != nil {
		t.Fatalf("GetSkillsGrouped: %v", err)
	}
	if !strings.Contains(rawQuery, "exclude=internal") {
		t.Fatalf("query %q missing exclude=internal", rawQuery)
	}
}

func TestGetSkillsGrouped_IncludeUngroupedQuery(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"groups":[],"ungroupedSkills":[]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(ClientOpts{APIURL: srv.URL})
	_, err := client.GetSkillsGrouped(context.Background(), GetSkillsQuery{
		IncludeUngrouped: true,
	})
	if err != nil {
		t.Fatalf("GetSkillsGrouped: %v", err)
	}
	if !strings.Contains(rawQuery, "include_ungrouped=true") {
		t.Fatalf("query %q missing include_ungrouped=true", rawQuery)
	}
}

func TestSkillsWriteEndpoints(t *testing.T) {
	var gotMethod, gotPath string
	var uploadMetadata struct {
		GroupIDs []string `json:"groupIdentifiers"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"accessToken": "tok", "expiresIn": 3600})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/skills":
			_ = json.NewEncoder(w).Encode(GroupedSkillsResponse{
				OK: true,
				Groups: []SkillGroupAtLatestVersion{{
					Identifier: "platform", Title: "Platform",
					Skills: []SkillAtLatestVersion{{Identifier: "skill-a", Title: "A", Version: "1.0.0"}},
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/skills/search":
			if r.URL.Query().Get("q") != "api" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(SkillsSummaryResponse{
				OK: true,
				Skills: []SkillCatalogEntry{{
					Skill: CatalogEntitySnapshot{Identifier: "demo-api-guide", Title: "Demo API Guide"},
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/skills/upload":
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "multipart/form-data") {
				http.Error(w, "expected multipart/form-data", http.StatusUnsupportedMediaType)
				return
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal([]byte(r.FormValue("metadata")), &uploadMetadata); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(SkillVersionWriteResponse{
				OK: true, SkillIdentifier: "skill-a", Version: "1.0.0", ActiveVersionSet: false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/skills/upload/batch":
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "multipart/form-data") {
				http.Error(w, "expected multipart/form-data", http.StatusUnsupportedMediaType)
				return
			}
			_ = json.NewEncoder(w).Encode(BatchUploadSkillsResponse{
				OK: true,
				Results: []BatchUploadSkillResultItem{
					{Identifier: "skill-a", OK: true, Result: &SkillVersionWriteResponse{SkillIdentifier: "skill-a", Version: "1.0.0"}},
					{Identifier: "skill-b", OK: false, Error: &BatchSkillError{Name: "validation_error", Message: "invalid", StatusCode: 400}},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/skills/skill-a/unpublish":
			_ = json.NewEncoder(w).Encode(UnpublishSkillResponse{OK: true, SkillIdentifier: "skill-a"})
		case r.Method == http.MethodPost && r.URL.Path == "/skills/skill-a/publish":
			_ = json.NewEncoder(w).Encode(SkillVersionWriteResponse{
				OK: true, SkillIdentifier: "skill-a", Version: "1.0.0", ActiveVersionSet: true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{
		APIURL:       srv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
	})
	client.tokenMgr.SetToken("tok", time.Now().Add(time.Hour))
	ctx := context.Background()

	grouped, err := client.GetSkillsGrouped(ctx, GetSkillsQuery{})
	if err != nil || len(grouped.Groups) != 1 || len(grouped.Groups[0].Skills) != 1 {
		t.Fatalf("GetSkillsGrouped: %v %+v", err, grouped)
	}

	search, err := client.SearchSkills(ctx, SearchSkillsQuery{Query: "api"})
	if err != nil || len(search.Skills) != 1 || search.Skills[0].Skill.Identifier != "demo-api-guide" {
		t.Fatalf("SearchSkills: %v %+v", err, search)
	}

	uploaded, err := client.UploadSkill(ctx, UploadSkillRequest{
		Identifier: "skill-a",
		GroupIDs:   []string{"platform-engineering"},
		Files:      []SkillFileInput{{Path: "SKILL.md", Content: "# x"}},
	})
	if err != nil || uploaded.Version != "1.0.0" {
		t.Fatalf("UploadSkill: %v %+v", err, uploaded)
	}
	if len(uploadMetadata.GroupIDs) != 1 || uploadMetadata.GroupIDs[0] != "platform-engineering" {
		t.Fatalf("upload groupIdentifiers: %+v", uploadMetadata.GroupIDs)
	}

	batch, err := client.UploadSkillsBatch(ctx, BatchUploadSkillsRequest{
		Skills: []UploadSkillRequest{
			{Identifier: "skill-a", Files: []SkillFileInput{{Path: "SKILL.md", Content: "# a"}}},
			{Identifier: "skill-b", Files: []SkillFileInput{{Path: "SKILL.md", Content: "# b"}}},
		},
	})
	if err != nil || len(batch.Results) != 2 || !batch.Results[0].OK || batch.Results[1].OK {
		t.Fatalf("UploadSkillsBatch: %v %+v", err, batch)
	}

	unpublished, err := client.UnpublishSkill(ctx, "skill-a")
	if err != nil || unpublished.SkillIdentifier != "skill-a" {
		t.Fatalf("UnpublishSkill: %v %+v", err, unpublished)
	}

	published, err := client.PublishSkill(ctx, "skill-a")
	if err != nil || published.Version != "1.0.0" || !published.ActiveVersionSet {
		t.Fatalf("PublishSkill: %v %+v", err, published)
	}

	if gotMethod == "" || gotPath == "" {
		t.Fatalf("expected requests, got %s %s", gotMethod, gotPath)
	}
}

func TestSkillsAPI_NotFoundErrorIsActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"message":"not found"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(ClientOpts{APIURL: srv.URL})
	client.tokenMgr.SetToken("tok", time.Now().Add(time.Hour))

	_, err := client.GetSkillsGrouped(context.Background(), GetSkillsQuery{})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "ai service") || strings.Contains(msg, "AI service") {
		t.Fatalf("error should not mention ai service: %s", msg)
	}
	for _, want := range []string{"skills are not available", "404", "api_url"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}
