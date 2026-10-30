package commands

import (
	"fmt"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/spf13/cobra"
)

// newCommandAPIClient returns an authenticated client for org (or the default org).
func newCommandAPIClient(cmd *cobra.Command, org string) (*api.Client, error) {
	flags := GetGlobalFlags(cmd.Context())
	configManager := config.NewConfigManager(flags.ConfigFile)

	cfg, err := configManager.LoadWithOverrides(flags.ClientID, flags.ClientSecret, flags.APIURL, org)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	useOrg := cfg.GetOrgOrDefault(org)
	orgConfig, err := cfg.GetOrgConfig(useOrg)
	if err != nil {
		return nil, err
	}
	token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
	if err != nil {
		return nil, err
	}
	return api.NewClient(api.ClientOpts{
		Token:        token,
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Timeout:      0,
	}), nil
}

// printPageAfterWrite prints the page as stored after a successful write. The
// Pages API write routes return only an identifier, so the page is re-read.
func printPageAfterWrite(cmd *cobra.Command, client *api.Client, pageID string) error {
	page, err := client.GetPage(cmd.Context(), pageID)
	if err != nil {
		cmd.PrintErrf("Could not read page '%s' after the write: %v\n", pageID, err)
		return formatOutput(map[string]interface{}{"identifier": pageID}, "json")
	}
	return formatOutput(page, "json")
}

// registerPageReplace registers the page replace (PUT) command.
func registerPageReplace() *cobra.Command {
	var org, dataFile string

	cmd := &cobra.Command{
		Use:   "replace [page-id]",
		Short: "Replace a page with the given definition",
		Long:  "Replace a page (PUT). Fields left out of the data file are reset, unlike 'update', which only changes the fields it is given.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := loadJSONFile(dataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			client, err := newCommandAPIClient(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			pageID, err := client.ReplacePage(cmd.Context(), args[0], api.Page(data))
			if err != nil {
				return fmt.Errorf("failed to replace page: %w", err)
			}

			cmd.Printf("✓ Page replaced successfully!\n")
			return printPageAfterWrite(cmd, client, pageID)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&dataFile, "data", "", "JSON file with the full page definition")
	cmd.MarkFlagRequired("data")

	return cmd
}

// registerPageDuplicate registers the page duplicate command.
func registerPageDuplicate() *cobra.Command {
	var org string

	cmd := &cobra.Command{
		Use:   "duplicate [page-id]",
		Short: "Copy a page, including its permissions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newCommandAPIClient(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			newPageID, err := client.DuplicatePage(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("failed to duplicate page: %w", err)
			}

			cmd.Printf("✓ Page '%s' duplicated as '%s'\n", args[0], newPageID)
			return printPageAfterWrite(cmd, client, newPageID)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")

	return cmd
}

// registerPageVersion registers the pages API version command.
func registerPageVersion() *cobra.Command {
	var org, format string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show whether the organization uses the beta or GA Pages API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newCommandAPIClient(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			version, err := client.GetPagesAPIVersion(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get pages API version: %w", err)
			}
			return formatOutput(map[string]interface{}{"version": version}, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

// registerPageOrgPermissions registers get/update for organization-level page
// permissions, which control who can create pages.
func registerPageOrgPermissions() *cobra.Command {
	parent := &cobra.Command{
		Use:   "org-permissions",
		Short: "Organization-level page permissions (who can create pages)",
	}

	var getOrg, getFormat string
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get organization-level page permissions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newCommandAPIClient(cmd, getOrg)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.GetOrgPagePermissions(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get organization page permissions: %w", err)
			}
			return formatOutput(result, getFormat)
		},
	}
	getCmd.Flags().StringVar(&getOrg, "org", "", "Organization name (uses default if not specified)")
	getCmd.Flags().StringVarP(&getFormat, "format", "f", "json", "Output format: json, yaml")

	var updateOrg, updateDataFile string
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update organization-level page permissions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := loadJSONFile(updateDataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			client, err := newCommandAPIClient(cmd, updateOrg)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.UpdateOrgPagePermissions(cmd.Context(), api.Permissions(data))
			if err != nil {
				return fmt.Errorf("failed to update organization page permissions: %w", err)
			}

			cmd.Printf("✓ Organization page permissions updated successfully!\n")
			return formatOutput(result, "json")
		},
	}
	updateCmd.Flags().StringVar(&updateOrg, "org", "", "Organization name (uses default if not specified)")
	updateCmd.Flags().StringVar(&updateDataFile, "data", "", "JSON file with permissions data, e.g. {\"create\": {\"roles\": [\"Admin\"]}}")
	updateCmd.MarkFlagRequired("data")

	parent.AddCommand(getCmd)
	parent.AddCommand(updateCmd)
	return parent
}

// registerPageWidgets registers create/update/delete for widgets on a page.
func registerPageWidgets() *cobra.Command {
	parent := &cobra.Command{
		Use:   "widgets",
		Short: "Widget operations on a page",
	}

	var createOrg, createDataFile, createParentWidget string
	createCmd := &cobra.Command{
		Use:   "create [page-id]",
		Short: "Add a widget to a page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			widget, err := loadJSONFile(createDataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			client, err := newCommandAPIClient(cmd, createOrg)
			if err != nil {
				return err
			}
			defer client.Close()

			result, err := client.CreatePageWidget(cmd.Context(), args[0], createParentWidget, widget)
			if err != nil {
				return fmt.Errorf("failed to create widget: %w", err)
			}

			cmd.Printf("✓ Widget added to page '%s'\n", args[0])
			if len(result) == 0 {
				return nil
			}
			return formatOutput(result, "json")
		},
	}
	createCmd.Flags().StringVar(&createOrg, "org", "", "Organization name (uses default if not specified)")
	createCmd.Flags().StringVar(&createDataFile, "data", "", "JSON file with the widget definition")
	createCmd.Flags().StringVar(&createParentWidget, "parent-widget", "", "ID of the widget to add this widget under, such as the page's dashboard widget")
	createCmd.MarkFlagRequired("data")
	createCmd.MarkFlagRequired("parent-widget")

	var updateOrg, updateDataFile string
	updateCmd := &cobra.Command{
		Use:   "update [page-id] [widget-id]",
		Short: "Change properties of a widget",
		Long:  "Partially update a widget. Only the properties in the data file change; set a property to null to clear it. A widget's type and id can't be changed.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch, err := loadJSONFile(updateDataFile)
			if err != nil {
				return fmt.Errorf("failed to load data file: %w", err)
			}
			client, err := newCommandAPIClient(cmd, updateOrg)
			if err != nil {
				return err
			}
			defer client.Close()

			if _, err := client.UpdatePageWidget(cmd.Context(), args[0], args[1], patch); err != nil {
				return fmt.Errorf("failed to update widget: %w", err)
			}

			cmd.Printf("✓ Widget '%s' on page '%s' updated successfully!\n", args[1], args[0])
			return nil
		},
	}
	updateCmd.Flags().StringVar(&updateOrg, "org", "", "Organization name (uses default if not specified)")
	updateCmd.Flags().StringVar(&updateDataFile, "data", "", "JSON file with the widget properties to change")
	updateCmd.MarkFlagRequired("data")

	var deleteOrg string
	var deleteForce bool
	deleteCmd := &cobra.Command{
		Use:   "delete [page-id] [widget-id]",
		Short: "Remove a widget from a page",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, widgetID := args[0], args[1]
			if !deleteForce {
				cmd.Printf("Are you sure you want to delete widget '%s' from page '%s'? [y/N]: ", widgetID, pageID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					cmd.Println("Operation cancelled")
					return nil
				}
			}

			client, err := newCommandAPIClient(cmd, deleteOrg)
			if err != nil {
				return err
			}
			defer client.Close()

			if err := client.DeletePageWidget(cmd.Context(), pageID, widgetID); err != nil {
				return fmt.Errorf("failed to delete widget: %w", err)
			}

			cmd.Printf("✓ Widget '%s' deleted from page '%s'\n", widgetID, pageID)
			return nil
		},
	}
	deleteCmd.Flags().StringVar(&deleteOrg, "org", "", "Organization name (uses default if not specified)")
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation")

	parent.AddCommand(createCmd)
	parent.AddCommand(updateCmd)
	parent.AddCommand(deleteCmd)
	return parent
}
