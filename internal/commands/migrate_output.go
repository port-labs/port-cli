package commands

import (
	"strings"

	"github.com/port-labs/port-cli/internal/modules/migrate"
	"github.com/port-labs/port-cli/internal/output"
)

func addMigrationDetailJSON(data map[string]interface{}, result *migrate.Result) {
	if result == nil {
		return
	}
	if len(result.BlueprintsToCreate) > 0 {
		data["blueprints_to_create"] = result.BlueprintsToCreate
	}
	if len(result.BlueprintsToUpdate) > 0 {
		data["blueprints_to_update"] = result.BlueprintsToUpdate
	}
	if len(result.BlueprintsToSkip) > 0 {
		data["blueprints_skipped_ids"] = result.BlueprintsToSkip
	}
	if len(result.BlueprintPermissionsToUpdate) > 0 {
		data["blueprint_permissions_to_update"] = result.BlueprintPermissionsToUpdate
	}
	if len(result.ActionPermissionsToUpdate) > 0 {
		data["action_permissions_to_update"] = result.ActionPermissionsToUpdate
	}
	if len(result.PagePermissionsToUpdate) > 0 {
		data["page_permissions_to_update"] = result.PagePermissionsToUpdate
	}
}

func printMigrationVerboseDetails(result *migrate.Result) {
	if result == nil {
		return
	}
	printedHeader := false
	printList := func(label string, values []string) {
		if len(values) == 0 {
			return
		}
		if !printedHeader {
			output.Printf("\nDry-run details:\n")
			printedHeader = true
		}
		output.Printf("  %s: %s\n", label, strings.Join(values, ", "))
	}
	printList("Blueprints to create", result.BlueprintsToCreate)
	printList("Blueprints to update", result.BlueprintsToUpdate)
	printList("Blueprints skipped", result.BlueprintsToSkip)
	printList("Blueprint permissions to update", result.BlueprintPermissionsToUpdate)
	printList("Action permissions to update", result.ActionPermissionsToUpdate)
	printList("Page permissions to update", result.PagePermissionsToUpdate)
}
