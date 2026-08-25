package migrate

import (
	"sort"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/import_module"
)

func blueprintIdentifiers(blueprints []api.Blueprint) []string {
	ids := make([]string, 0, len(blueprints))
	for _, bp := range blueprints {
		if id, ok := bp["identifier"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func permissionsChangeIdentifiers(changes []import_module.PermissionsChange) []string {
	ids := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Identifier != "" {
			ids = append(ids, change.Identifier)
		}
	}
	sort.Strings(ids)
	return ids
}
