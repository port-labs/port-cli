package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertSkillMDFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		skillName   string
		description string
		want        string
	}{
		{
			name:        "updates leading frontmatter in place",
			content:     "---\nname: old-name\ndescription: Old description\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
		{
			name:        "preserves extra header fields",
			content:     "---\nname: old-name\ndescription: Old description\ndisable-model-invocation: true\nallowed-tools: bash\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\ndisable-model-invocation: true\nallowed-tools: bash\n---\n\nDeploy it.",
		},
		{
			name:        "updates agent skills header after another delimiter section without duplicating",
			content:     "# Notes\n\n---\nnot: skill metadata\n---\n\n---\nname: old-name\ndescription: Old description\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "# Notes\n\n---\nnot: skill metadata\n---\n\n---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
		{
			name:        "collapses already-duplicated frontmatter",
			content:     "---\nname: old-name\ndescription: Old description\n---\n---\nname: old-name\ndescription: Old description\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
		{
			name:        "updates in place when a blank line precedes the header",
			content:     "\n---\nname: old-name\ndescription: Old description\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "\n---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
		{
			name:        "prepends a single frontmatter block for body-only markdown",
			content:     "# Deploy\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\n---\n\n# Deploy\n\nDeploy it.",
		},
		{
			name:        "incomplete leading frontmatter is rewritten in place not prepended",
			content:     "---\nname: old-name\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
		{
			name:        "strips utf8 BOM before rewriting",
			content:     "\ufeff---\nname: old-name\ndescription: Old description\n---\n\nDeploy it.",
			skillName:   "deploy-service",
			description: "Deploy service",
			want:        "---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upsertSkillMDFrontmatter(tt.content, tt.skillName, tt.description)
			if got != tt.want {
				t.Errorf("upsertSkillMDFrontmatter() =\n%q\nwant:\n%q", got, tt.want)
			}
			if strings.Count(got, "description:") != 1 {
				t.Errorf("expected exactly one description: field, got %d in:\n%s", strings.Count(got, "description:"), got)
			}
			if strings.Count(got, "---\nname: "+tt.skillName+"\n") != 1 {
				t.Errorf("expected exactly one skill name frontmatter block, got:\n%s", got)
			}
		})
	}
}

func TestUpsertSkillMDFrontmatter_DoesNotTreatIncompleteMidDocBlockAsHeader(t *testing.T) {
	content := "# Notes\n\n---\nname: not-complete\n---\n\nBody only."
	got := upsertSkillMDFrontmatter(content, "deploy-service", "Deploy service")
	wantPrefix := "---\nname: deploy-service\ndescription: Deploy service\n---\n\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected prepended frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "---\nname: not-complete\n---") {
		t.Fatalf("expected original incomplete block to remain in body, got:\n%s", got)
	}
	if strings.Count(got, "description:") != 1 {
		t.Fatalf("expected a single description: field, got %d in:\n%s", strings.Count(got, "description:"), got)
	}
}

func TestNormalizeSkillMDContent_APIDescriptionWinsWithoutDuplicating(t *testing.T) {
	content := "---\nname: deploy-service\ndescription: Frontmatter description\n---\n\nDeploy it."
	got := normalizeSkillMDContent(Skill{
		Identifier:  "deploy-service",
		Description: "API description",
	}, "deploy-service", content)
	want := "---\nname: deploy-service\ndescription: API description\n---\n\nDeploy it."
	if got != want {
		t.Fatalf("normalizeSkillMDContent() =\n%q\nwant:\n%q", got, want)
	}
	if strings.Count(got, "description:") != 1 {
		t.Fatalf("expected one description: field, got %d", strings.Count(got, "description:"))
	}
}

func TestWriteSkills_DoesNotDuplicateFrontmatterOnSync(t *testing.T) {
	dir := t.TempDir()
	skill := Skill{
		Identifier:  "deploy-service",
		Title:       "Deploy",
		Description: "API description",
		Files: []SkillFile{{
			Path:    "SKILL.md",
			Content: "\n---\nname: deploy-service\ndescription: Frontmatter description\ndisable-model-invocation: true\n---\n\nDeploy it.",
		}},
	}
	if err := WriteSkills([]Skill{skill}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	path := filepath.Join(dir, "skills", "deploy-service", "SKILL.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)
	want := "\n---\nname: deploy-service\ndescription: API description\ndisable-model-invocation: true\n---\n\nDeploy it."
	if content != want {
		t.Fatalf("SKILL.md =\n%q\nwant:\n%q", content, want)
	}
	if strings.Count(content, "description:") != 1 {
		t.Fatalf("expected one description: field, got %d", strings.Count(content, "description:"))
	}
}

func TestWriteSkills_HealsDuplicatedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	skill := Skill{
		Identifier:  "deploy-service",
		Title:       "Deploy",
		Description: "API description",
		Files: []SkillFile{{
			Path:    "SKILL.md",
			Content: "---\nname: deploy-service\ndescription: Dup A\n---\n---\nname: deploy-service\ndescription: Dup B\n---\n\nDeploy it.",
		}},
	}
	if err := WriteSkills([]Skill{skill}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	path := filepath.Join(dir, "skills", "deploy-service", "SKILL.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)
	want := "---\nname: deploy-service\ndescription: API description\n---\n\nDeploy it."
	if content != want {
		t.Fatalf("SKILL.md =\n%q\nwant:\n%q", content, want)
	}
}
