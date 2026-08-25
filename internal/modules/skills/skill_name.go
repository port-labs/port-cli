package skills

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var agentSkillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func agentSkillNameFromIdentifier(identifier string) (string, error) {
	base := skillIdentifierBase(identifier)
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		default:
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	name := strings.Trim(b.String(), "-")
	if err := validateAgentSkillName(name); err != nil {
		return "", fmt.Errorf("cannot derive Agent Skills name from %q: %w", identifier, err)
	}
	return name, nil
}

func validateAgentSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("must not be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("must be at most 64 characters")
	}
	if !agentSkillNamePattern.MatchString(name) {
		return fmt.Errorf("must contain only lowercase letters, numbers, and single hyphens")
	}
	return nil
}
