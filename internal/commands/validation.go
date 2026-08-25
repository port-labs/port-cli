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
