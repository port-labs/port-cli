package commands

import (
	"fmt"
	"strings"
)

func validateStringEnum(flagName, value string, allowed []string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("invalid value for %s: %s. Valid values: %s", flagName, value, strings.Join(allowed, ", "))
}

// ValidResources is the default set of resource types supported by the --include flag.
// Individual commands may define their own allowed slice if they need to support
// a different set.
var ValidResources = []string{
	"blueprints", "entities", "scorecards", "actions", "teams", "users",
	"automations", "pages", "integrations", "blueprint-permissions",
	"action-permissions", "page-permissions",
}

// ValidateResource checks if a provided resource string matches one of the allowed resources.
// The allowed slice is passed explicitly so each command can restrict or extend
// the default ValidResources list without affecting other commands.
func ValidateResource(r string, allowed []string) error {
	for _, v := range allowed {
		if r == v {
			return nil
		}
	}
	return fmt.Errorf("invalid resource: %s. Valid resources: %s", r, strings.Join(allowed, ", "))
}
