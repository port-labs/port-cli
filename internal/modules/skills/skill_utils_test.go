package skills

import "testing"

func TestParseSkillLocation(t *testing.T) {
	tests := []struct {
		input string
		want  SkillLocation
	}{
		{"project", SkillLocationProject},
		{"global", SkillLocationGlobal},
		{"", SkillLocationGlobal},
		{"other", SkillLocationGlobal},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseSkillLocation(tt.input); got != tt.want {
				t.Errorf("parseSkillLocation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGroupName(t *testing.T) {
	groups := []SkillGroup{
		{Identifier: "grp-1", Title: "My Group"},
		{Identifier: "grp-2", Title: ""},
	}
	tests := []struct{ groupID, want string }{
		{"grp-1", "My Group"},
		{"grp-2", "grp-2"},
		{"unknown", "unknown"},
		{"", NoGroupDir},
	}
	for _, tt := range tests {
		t.Run(tt.groupID, func(t *testing.T) {
			if got := GroupName(groups, tt.groupID); got != tt.want {
				t.Errorf("GroupName(%q) = %q, want %q", tt.groupID, got, tt.want)
			}
		})
	}
}
