package commands

import (
	"context"
	"fmt"
	"sync"

	"charm.land/huh/v2"
	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/import_module"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

func buildErrorHandlingOptions(cmd *cobra.Command, raw []string) (import_module.ErrorHandlingOptions, error) {
	policies, err := import_module.ParseErrorPolicies(raw)
	if err != nil {
		return import_module.ErrorHandlingOptions{}, err
	}
	opts := import_module.ErrorHandlingOptions{Policies: policies}
	if IsInteractive() {
		opts.ResolveAction = newErrorActionPromptResolver()
	}
	return opts, nil
}

func newErrorActionPromptResolver() import_module.ErrorActionResolver {
	var mu sync.Mutex
	cache := make(map[string]import_module.ErrorAction)
	return func(ctx context.Context, apiErr *api.APIError, actions []import_module.ErrorAction) (import_module.ErrorAction, error) {
		mu.Lock()
		defer mu.Unlock()
		if action, ok := cache[apiErr.Code]; ok {
			return action, nil
		}
		if len(actions) == 0 {
			return "", fmt.Errorf("no interactive actions are available for API error %s", apiErr.Code)
		}
		var selected import_module.ErrorAction
		options := make([]huh.Option[import_module.ErrorAction], 0, len(actions))
		for _, action := range actions {
			options = append(options, huh.NewOption(errorActionLabel(action), action))
		}
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[import_module.ErrorAction]().
					Title(fmt.Sprintf("Handle Port API error: %s", apiErr.Code)).
					Description(apiErr.Message).
					Options(options...).
					Value(&selected),
			),
		).WithTheme(&styles.FormTheme{})
		if err := form.RunWithContext(ctx); err != nil {
			return "", fmt.Errorf("prompt error: %w", err)
		}
		cache[apiErr.Code] = selected
		return selected, nil
	}
}

func errorActionLabel(action import_module.ErrorAction) string {
	switch action {
	case import_module.ErrorActionFail:
		return "Fail"
	case import_module.ErrorActionIgnoreProperty:
		return "Ignore property update"
	case import_module.ErrorActionRecreateProperty:
		return "Recreate property and preserve values"
	default:
		return string(action)
	}
}
