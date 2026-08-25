package commands

import (
	"testing"

	"github.com/port-labs/port-cli/internal/modules/skills"
)

func TestFilterIDsToUngrouped(t *testing.T) {
	ungrouped := []skills.Skill{
		{Identifier: "standalone"},
	}
	got := filterIDsToUngrouped([]string{"standalone", "was-in-excluded-group"}, ungrouped)
	if len(got) != 1 || got[0] != "standalone" {
		t.Fatalf("got %v", got)
	}
}
