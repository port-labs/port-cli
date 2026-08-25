package commands

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestPrintGroupedSkillsPreview(t *testing.T) {
	resp := &api.GroupedSkillsResponse{
		OK: true,
		Groups: []api.SkillGroupAtLatestVersion{
			{
				Identifier: "platform-engineering",
				Title:      "Platform Engineering",
				Skills: []api.SkillAtLatestVersion{
					{
						Identifier: "demo-api-guide",
						Title:      "API Guide",
						Location:   "global",
						Version:    "1.2.3",
					},
				},
			},
		},
		UngroupedSkills: []api.SkillAtLatestVersion{
			{
				Identifier: "solo-skill",
				Title:      "Solo Skill",
				Location:   "project",
				Version:    "2.0.0",
			},
		},
	}

	var buf bytes.Buffer
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printGroupedSkillsPreview(resp)
	w.Close()
	os.Stdout = orig
	io.Copy(&buf, r)

	out := buf.String()
	for _, want := range []string{
		"Platform Engineering",
		"demo-api-guide",
		"API Guide",
		"location: global",
		"version: 1.2.3",
		"Ungrouped",
		"solo-skill",
		"version: 2.0.0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintGroupedSkillsPreview_NoneVersion(t *testing.T) {
	resp := &api.GroupedSkillsResponse{
		OK: true,
		UngroupedSkills: []api.SkillAtLatestVersion{
			{Identifier: "unpublished-skill", Title: "Unpublished", Location: "global", Version: ""},
		},
	}

	var buf bytes.Buffer
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printGroupedSkillsPreview(resp)
	w.Close()
	os.Stdout = orig
	io.Copy(&buf, r)

	if out := buf.String(); !strings.Contains(out, "version: (none)") {
		t.Fatalf("expected 'version: (none)' in output:\n%s", out)
	}
}
