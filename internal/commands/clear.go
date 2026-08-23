package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// RegisterClear registers the clear command.
func RegisterClear(rootCmd *cobra.Command) {
	var (
		org                     string
		blueprintScope          []string
		jqFilter                string
		clearBlueprints         bool
		clearEntities           bool
		clearActions            bool
		clearAutomations        bool
		clearScorecards         bool
		clearPages              bool
		includeSystemBlueprints bool
		deleteProtectedPages    bool
		force                   bool
	)

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete all or filtered resources from a Port organization",
		Long: `Delete all or filtered resources from a Port organization.

Deletion is opt-in by resource type and can be performed in bulk (all) or filtered by blueprints and JQ conditions. 
For example:
  # Delete everything of a specific type (Bulk)
  port clear --pages
  port clear --blueprints --entities --actions --scorecards

Supported resource types:
  --blueprints    Delete all blueprints
  --entities      Delete all entities across all blueprints
  --actions       Delete all self-service actions across all blueprints
  --automations   Delete all automations
  --scorecards    Delete all scorecards across all blueprints
  --pages         Delete root pages and root folders

When multiple types are selected, dependent resources are deleted before their
parents. For example, entities and actions are removed before blueprints.

Blueprints whose identifiers start with an underscore are system blueprints
(e.g. _user, _team). They are always skipped for --blueprints. Their entities,
actions, and scorecards are also skipped by default; use
--include-system-blueprints to include them. Pages and folders whose
identifiers start with an underscore are skipped unless
--delete-protected-pages is provided.

Use --blueprint to filter --entities, --actions, --scorecards, and
--blueprints to one or more specific blueprints:
  # Filter by Blueprint
  port clear --entities --blueprint service
  port clear --entities --blueprint service --blueprint repository

Use --jq to evaluate a jq expression on each entity before deleting it.
Only entities that return true (or a non-null, non-false truthy value) will be deleted.
  # Filter by Blueprint and JQ Condition
  port clear --entities --blueprint aiSpec --jq '.properties.organization == "example-org"'

If --org is omitted, the default organization from the Port config is used.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !clearBlueprints && !clearEntities && !clearActions && !clearAutomations && !clearScorecards && !clearPages {
				return fmt.Errorf("no resource types selected. Use at least one flag such as --pages, --blueprints, --entities, --actions, --automations, or --scorecards")
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			cfg, err := configManager.LoadWithOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				org,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			resolvedOrg := cfg.GetOrgOrDefault(org)

			if !ShouldSkipConfirm(cmd, force) {
				selection := []string{}
				if clearEntities {
					selection = append(selection, "entities")
				}
				if clearActions {
					selection = append(selection, "actions")
				}
				if clearScorecards {
					selection = append(selection, "scorecards")
				}
				if clearAutomations {
					selection = append(selection, "automations")
				}
				if clearPages {
					selection = append(selection, "pages")
				}
				if clearBlueprints {
					selection = append(selection, "blueprints")
				}
				confirmed, err := confirmPrompt(
					fmt.Sprintf("Delete %s?", strings.Join(selection, ", ")),
					fmt.Sprintf("This will delete %s from organization %q.", strings.Join(selection, ", "), resolvedOrg),
				)
				if err != nil {
					return err
				}
				if !confirmed {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			orgConfig, err := cfg.GetOrgConfig(resolvedOrg)
			if err != nil {
				return err
			}

			client := api.NewClient(api.ClientOpts{
				ClientID:     orgConfig.ClientID,
				ClientSecret: orgConfig.ClientSecret,
				APIURL:       orgConfig.APIURL,
			})
			defer client.Close()

			// Fetch blueprints once for all blueprint-dependent operations.
			// blueprintsForDeletion always excludes system (_-prefixed) blueprints.
			// blueprintsForResources excludes them too unless --include-system-blueprints is set.
			// Both are further narrowed to --blueprint scope if provided.
			var blueprintsForDeletion, blueprintsForResources []api.Blueprint
			if clearBlueprints || clearEntities || clearActions || clearScorecards {
				all, err := client.GetBlueprints(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to list blueprints: %w", err)
				}
				all, err = scopeBlueprints(all, blueprintScope)
				if err != nil {
					return err
				}
				blueprintsForDeletion = filterProtectedBlueprints(all, false)
				blueprintsForResources = filterProtectedBlueprints(all, includeSystemBlueprints)
			}

			// Delete in dependency order: dependents before parents.
			if clearEntities {
				if err := clearAllEntities(cmd, client, blueprintsForResources, jqFilter); err != nil {
					return err
				}
			}

			if clearActions {
				if err := clearAllActions(cmd, client, blueprintsForResources); err != nil {
					return err
				}
			}

			if clearScorecards {
				if err := clearAllScorecards(cmd, client, blueprintsForResources); err != nil {
					return err
				}
			}

			if clearAutomations {
				if err := clearAllAutomations(cmd, client); err != nil {
					return err
				}
			}

			if clearPages {
				if err := clearPagesAndFolders(cmd, client, deleteProtectedPages); err != nil {
					return err
				}
			}

			if clearBlueprints {
				if err := clearAllBlueprints(cmd, client, blueprintsForDeletion); err != nil {
					return err
				}
			}

			return nil
		},
	}

	clearCmd.Flags().StringVar(&org, "org", "", "Organization name (uses the default org from config if not specified)")
	clearCmd.Flags().StringArrayVar(&blueprintScope, "blueprint", nil, "Restrict --entities, --actions, --scorecards, and --blueprints to specific blueprint identifiers (repeatable)")
	clearCmd.Flags().StringVar(&jqFilter, "jq", "", "JQ expression to filter entities to delete when using --entities (e.g. '.properties.state == \"archived\"')")
	clearCmd.Flags().BoolVar(&clearBlueprints, "blueprints", false, "Delete all blueprints")
	clearCmd.Flags().BoolVar(&clearEntities, "entities", false, "Delete all entities across all blueprints")
	clearCmd.Flags().BoolVar(&clearActions, "actions", false, "Delete all self-service actions across all blueprints")
	clearCmd.Flags().BoolVar(&clearAutomations, "automations", false, "Delete all automations")
	clearCmd.Flags().BoolVar(&clearScorecards, "scorecards", false, "Delete all scorecards across all blueprints")
	clearCmd.Flags().BoolVar(&clearPages, "pages", false, "Delete root pages and root folders")
	clearCmd.Flags().BoolVar(&includeSystemBlueprints, "include-system-blueprints", false, "Also delete entities, actions, and scorecards from system blueprints (those whose identifiers start with an underscore, e.g. _user, _team)")
	clearCmd.Flags().BoolVar(&deleteProtectedPages, "delete-protected-pages", false, "Also delete protected root pages and folders whose identifiers start with an underscore, after non-protected items")
	clearCmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	rootCmd.AddCommand(clearCmd)
}

// scopeBlueprints restricts the list to the given identifiers. If scope is empty, all blueprints are returned.
func scopeBlueprints(blueprints []api.Blueprint, scope []string) ([]api.Blueprint, error) {
	if len(scope) == 0 {
		return blueprints, nil
	}

	allowed := make(map[string]bool, len(scope))
	for _, id := range scope {
		allowed[id] = true
	}

	filtered := make([]api.Blueprint, 0, len(allowed))
	for _, bp := range blueprints {
		id, _ := bp["identifier"].(string)
		if allowed[id] {
			filtered = append(filtered, bp)
			delete(allowed, id)
		}
	}

	if len(allowed) > 0 {
		var missing []string
		for id := range allowed {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("blueprint(s) not found in organization: %s", strings.Join(missing, ", "))
	}

	return filtered, nil
}

func filterProtectedBlueprints(blueprints []api.Blueprint, includeProtected bool) []api.Blueprint {
	if includeProtected {
		return blueprints
	}
	filtered := make([]api.Blueprint, 0, len(blueprints))
	for _, bp := range blueprints {
		id, _ := bp["identifier"].(string)
		if !strings.HasPrefix(id, "_") {
			filtered = append(filtered, bp)
		}
	}
	return filtered
}

func clearAllEntities(cmd *cobra.Command, client *api.Client, blueprints []api.Blueprint, jqFilter string) error {
	ctx := cmd.Context()
	total := 0

	var query *gojq.Query
	if jqFilter != "" {
		var err error
		query, err = gojq.Parse(jqFilter)
		if err != nil {
			return fmt.Errorf("invalid jq filter: %w", err)
		}
	}

	for _, bp := range blueprints {
		bpID, _ := bp["identifier"].(string)
		if bpID == "" {
			continue
		}

		entities, err := client.SearchEntities(ctx, bpID, map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("failed to search entities for blueprint %q: %w", bpID, err)
		}

		var entityIDs []string
		for _, entity := range entities {
			if query != nil {
				iter := query.Run(map[string]interface{}(entity))
				var match bool
				for {
					v, ok := iter.Next()
					if !ok {
						break
					}
					if err, isErr := v.(error); isErr {
						entityID, _ := entity["identifier"].(string)
						return fmt.Errorf("failed evaluating jq filter for entity %s: %w", entityID, err)
					}
					if v != nil && v != false {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			entityID, _ := entity["identifier"].(string)
			if entityID != "" {
				entityIDs = append(entityIDs, entityID)
			}
		}

		if len(entityIDs) == 0 {
			continue
		}

		batchSize := 100
		for i := 0; i < len(entityIDs); i += batchSize {
			end := i + batchSize
			if end > len(entityIDs) {
				end = len(entityIDs)
			}

			batch := entityIDs[i:end]
			if _, err := client.BulkDeleteEntities(ctx, bpID, batch, false); err != nil {
				return fmt.Errorf("failed to bulk delete entities from blueprint %q: %w", bpID, err)
			}

			for _, eID := range batch {
				output.Printf("Deleted entity: %s/%s\n", bpID, eID)
			}
			total += len(batch)
		}
	}

	output.SuccessPrint("Deleted %d entities\n", total)
	return nil
}

func clearAllActions(cmd *cobra.Command, client *api.Client, blueprints []api.Blueprint) error {
	ctx := cmd.Context()
	total := 0
	blueprintSet := blueprintIdentifierSet(blueprints)

	actions, err := client.GetAllActions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list actions: %w", err)
	}

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, action := range actions {
		actionID, _ := action["identifier"].(string)
		bpID := api.SelfServiceActionBlueprintID(action)
		if actionID == "" || !blueprintSet[bpID] {
			continue
		}
		total++
		bID, aID := bpID, actionID
		g.Go(func() error {
			if err := client.DeleteActionByID(groupCtx, aID); err != nil {
				return fmt.Errorf("failed to delete action %q from blueprint %q: %w", aID, bID, err)
			}
			output.Printf("Deleted action: %s/%s\n", bID, aID)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	output.SuccessPrint("Deleted %d self-service actions\n", total)
	return nil
}

func blueprintIdentifierSet(blueprints []api.Blueprint) map[string]bool {
	set := make(map[string]bool, len(blueprints))
	for _, bp := range blueprints {
		if id, _ := bp["identifier"].(string); id != "" {
			set[id] = true
		}
	}
	return set
}

func clearAllScorecards(cmd *cobra.Command, client *api.Client, blueprints []api.Blueprint) error {
	ctx := cmd.Context()
	total := 0

	for _, bp := range blueprints {
		bpID, _ := bp["identifier"].(string)
		if bpID == "" {
			continue
		}

		scorecards, err := client.GetScorecards(ctx, bpID)
		if err != nil {
			return fmt.Errorf("failed to list scorecards for blueprint %q: %w", bpID, err)
		}

		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(8)

		for _, scorecard := range scorecards {
			scorecardID, _ := scorecard["identifier"].(string)
			if scorecardID == "" {
				continue
			}
			total++
			bID, sID := bpID, scorecardID
			g.Go(func() error {
				if err := client.DeleteScorecard(groupCtx, bID, sID); err != nil {
					return fmt.Errorf("failed to delete scorecard %q from blueprint %q: %w", sID, bID, err)
				}
				output.Printf("Deleted scorecard: %s/%s\n", bID, sID)
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}
	}

	output.SuccessPrint("Deleted %d scorecards\n", total)
	return nil
}

func clearAllAutomations(cmd *cobra.Command, client *api.Client) error {
	ctx := cmd.Context()

	automations, err := client.GetAllActions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list automations: %w", err)
	}

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	total := 0

	for _, automation := range automations {
		automationID, _ := automation["identifier"].(string)
		if automationID == "" || !api.IsAutomationAction(automation) {
			continue
		}
		total++
		aID := automationID
		g.Go(func() error {
			if err := client.DeleteAutomation(groupCtx, aID); err != nil {
				return fmt.Errorf("failed to delete automation %q: %w", aID, err)
			}
			output.Printf("Deleted automation: %s\n", aID)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	output.SuccessPrint("Deleted %d automations\n", total)
	return nil
}

func clearAllBlueprints(cmd *cobra.Command, client *api.Client, blueprints []api.Blueprint) error {
	ctx := cmd.Context()

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	total := 0

	for _, bp := range blueprints {
		bpID, _ := bp["identifier"].(string)
		if bpID == "" {
			continue
		}
		total++
		bID := bpID
		g.Go(func() error {
			if err := client.DeleteBlueprint(groupCtx, bID); err != nil {
				return fmt.Errorf("failed to delete blueprint %q: %w", bID, err)
			}
			output.Printf("Deleted blueprint: %s\n", bID)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	output.SuccessPrint("Deleted %d blueprints\n", total)
	return nil
}

func clearPagesAndFolders(cmd *cobra.Command, client *api.Client, deleteProtected bool) error {
	ctx := cmd.Context()

	pages, err := client.GetPages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list pages: %w", err)
	}

	folders, err := client.GetFolders(ctx)
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	rootFolders := rootFoldersForDeletion(folders, deleteProtected)
	rootPages := rootPagesForDeletion(pages, deleteProtected)
	rootFolders, protectedRootFolders := partitionProtectedFolders(rootFolders)
	rootPages, protectedRootPages := partitionProtectedPages(rootPages)

	deletedRootFolders, deletedRootPages, err := deleteRootSidebarItems(ctx, client, rootFolders, rootPages)
	if err != nil {
		return err
	}
	if deleteProtected {
		deletedProtectedFolders, deletedProtectedPages, err := deleteRootSidebarItems(ctx, client, protectedRootFolders, protectedRootPages)
		if err != nil {
			return err
		}
		deletedRootFolders += deletedProtectedFolders
		deletedRootPages += deletedProtectedPages
	}

	output.SuccessPrint("Deleted %d root pages and %d root folders\n", deletedRootPages, deletedRootFolders)
	return nil
}

func deleteRootSidebarItems(ctx context.Context, client *api.Client, folders []api.Folder, pages []api.Page) (int, int, error) {
	var (
		deletedRootFolders int
		deletedRootPages   int
	)

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for _, folder := range folders {
		folderID, _ := folder["identifier"].(string)
		if folderID == "" {
			continue
		}
		g.Go(func() error {
			if err := client.DeleteFolder(groupCtx, folderID); err != nil {
				return fmt.Errorf("failed to delete folder %q: %w", folderID, err)
			}
			output.Printf("Deleted root folder: %s\n", folderID)
			return nil
		})
		deletedRootFolders++
	}

	for _, page := range pages {
		pageID, _ := page["identifier"].(string)
		if pageID == "" {
			continue
		}
		g.Go(func() error {
			if err := client.DeletePage(groupCtx, pageID); err != nil {
				return fmt.Errorf("failed to delete page %q: %w", pageID, err)
			}
			output.Printf("Deleted root page: %s\n", pageID)
			return nil
		})
		deletedRootPages++
	}

	if err := g.Wait(); err != nil {
		return 0, 0, err
	}

	return deletedRootFolders, deletedRootPages, nil
}

func rootPagesForDeletion(pages []api.Page, deleteProtected bool) []api.Page {
	roots := make([]api.Page, 0, len(pages))
	protectedRoots := make([]api.Page, 0, len(pages))
	for _, page := range pages {
		if !isDeletablePage(page) || !isRootSidebarItem(map[string]interface{}(page)) {
			continue
		}
		if isProtectedSidebarItemIdentifier(page["identifier"]) {
			if deleteProtected {
				protectedRoots = append(protectedRoots, page)
			}
			continue
		}
		roots = append(roots, page)
	}
	return append(roots, protectedRoots...)
}

func rootFoldersForDeletion(folders []api.Folder, deleteProtected bool) []api.Folder {
	roots := make([]api.Folder, 0, len(folders))
	protectedRoots := make([]api.Folder, 0, len(folders))
	for _, folder := range folders {
		if !isRootSidebarItem(map[string]interface{}(folder)) {
			continue
		}
		if isProtectedSidebarItemIdentifier(folder["identifier"]) {
			if deleteProtected {
				protectedRoots = append(protectedRoots, folder)
			}
			continue
		}
		roots = append(roots, folder)
	}
	return append(roots, protectedRoots...)
}

func partitionProtectedPages(pages []api.Page) ([]api.Page, []api.Page) {
	regular := make([]api.Page, 0, len(pages))
	protected := make([]api.Page, 0, len(pages))
	for _, page := range pages {
		if isProtectedSidebarItemIdentifier(page["identifier"]) {
			protected = append(protected, page)
			continue
		}
		regular = append(regular, page)
	}
	return regular, protected
}

func partitionProtectedFolders(folders []api.Folder) ([]api.Folder, []api.Folder) {
	regular := make([]api.Folder, 0, len(folders))
	protected := make([]api.Folder, 0, len(folders))
	for _, folder := range folders {
		if isProtectedSidebarItemIdentifier(folder["identifier"]) {
			protected = append(protected, folder)
			continue
		}
		regular = append(regular, folder)
	}
	return regular, protected
}

func isRootSidebarItem(item map[string]interface{}) bool {
	parent, exists := item["parent"]
	if !exists || parent == nil {
		return true
	}
	parentID, ok := parent.(string)
	return !ok || parentID == ""
}

func isDeletablePage(page api.Page) bool {
	showInSidebar, ok := page["showInSidebar"].(bool)
	return ok && showInSidebar
}

func isProtectedSidebarItemIdentifier(identifier interface{}) bool {
	id, ok := identifier.(string)
	if !ok || id == "" {
		return false
	}
	return strings.HasPrefix(id, "_")
}
