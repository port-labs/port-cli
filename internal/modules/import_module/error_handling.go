package import_module

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/port-labs/port-cli/internal/api"
)

const (
	defaultMigrationPollInterval = 2 * time.Second
	defaultMigrationWaitTimeout  = 30 * time.Minute
)

type ErrorAction string

const (
	ErrorActionFail             ErrorAction = "fail"
	ErrorActionPrompt           ErrorAction = "prompt"
	ErrorActionIgnoreProperty   ErrorAction = "ignore-property"
	ErrorActionRecreateProperty ErrorAction = "recreate-property"
)

type ErrorActionResolver func(ctx context.Context, apiErr *api.APIError, actions []ErrorAction) (ErrorAction, error)

type ErrorHandlingOptions struct {
	Policies              map[string]ErrorAction
	ResolveAction         ErrorActionResolver
	AddWarning            func(string)
	MigrationPollInterval time.Duration
	MigrationWaitTimeout  time.Duration
}

type BlueprintUpdateMode string

const (
	BlueprintUpdatePUT   BlueprintUpdateMode = "put"
	BlueprintUpdatePATCH BlueprintUpdateMode = "patch"
)

type BlueprintUpdater struct {
	client  *api.Client
	options ErrorHandlingOptions
	mu      sync.Mutex
}

func NewBlueprintUpdater(client *api.Client, options ErrorHandlingOptions) *BlueprintUpdater {
	return &BlueprintUpdater{client: client, options: options}
}

func ParseErrorPolicies(values []string) (map[string]ErrorAction, error) {
	policies := make(map[string]ErrorAction)
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("--on-error must be in the form <error>=<action>")
		}
		code := strings.TrimSpace(parts[0])
		action := ErrorAction(strings.TrimSpace(parts[1]))
		if code != "forbidden_format_change" {
			return nil, fmt.Errorf("unsupported --on-error type %q", code)
		}
		if !isSupportedForbiddenFormatChangeAction(action) {
			return nil, fmt.Errorf("unsupported action %q for %s; supported actions: fail, prompt, ignore-property, recreate-property", action, code)
		}
		policies[code] = action
	}
	return policies, nil
}

func SupportedErrorActions(code string) []ErrorAction {
	if code == "forbidden_format_change" {
		return []ErrorAction{ErrorActionFail, ErrorActionIgnoreProperty, ErrorActionRecreateProperty}
	}
	return nil
}

func (u *BlueprintUpdater) Update(ctx context.Context, id string, blueprint api.Blueprint, mode BlueprintUpdateMode) error {
	_, err := u.applyUpdate(ctx, id, blueprint, mode)
	if err == nil {
		return nil
	}

	apiErr := apiErrorForCode(err, "forbidden_format_change")
	if apiErr == nil {
		return err
	}
	property, _ := apiErr.Details["property"].(string)
	if property == "" {
		return err
	}

	action, resolveErr := u.resolveAction(ctx, apiErr)
	if resolveErr != nil {
		return resolveErr
	}
	switch action {
	case ErrorActionFail:
		return err
	case ErrorActionIgnoreProperty:
		return u.ignorePropertyUpdate(ctx, id, blueprint, mode, property)
	case ErrorActionRecreateProperty:
		return u.recreateProperty(ctx, id, blueprint, mode, property)
	default:
		return fmt.Errorf("unsupported action %q for API error %s", action, apiErr.Code)
	}
}

func (u *BlueprintUpdater) applyUpdate(ctx context.Context, id string, blueprint api.Blueprint, mode BlueprintUpdateMode) (api.Blueprint, error) {
	if mode == BlueprintUpdatePATCH {
		return u.client.PatchBlueprint(ctx, id, blueprint)
	}
	return u.client.UpdateBlueprint(ctx, id, blueprint)
}

func (u *BlueprintUpdater) resolveAction(ctx context.Context, apiErr *api.APIError) (ErrorAction, error) {
	action, ok := u.options.Policies[apiErr.Code]
	if !ok {
		if u.options.ResolveAction == nil {
			return "", fmt.Errorf("%s. Provide --on-error %s=fail, --on-error %s=ignore-property, or --on-error %s=recreate-property", apiErr.Error(), apiErr.Code, apiErr.Code, apiErr.Code)
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.options.ResolveAction(ctx, apiErr, SupportedErrorActions(apiErr.Code))
	}
	if action == ErrorActionPrompt {
		if u.options.ResolveAction == nil {
			return "", fmt.Errorf("%s. Cannot prompt in this environment; provide --on-error %s=fail, --on-error %s=ignore-property, or --on-error %s=recreate-property", apiErr.Error(), apiErr.Code, apiErr.Code, apiErr.Code)
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.options.ResolveAction(ctx, apiErr, SupportedErrorActions(apiErr.Code))
	}
	return action, nil
}

func (u *BlueprintUpdater) ignorePropertyUpdate(ctx context.Context, id string, blueprint api.Blueprint, mode BlueprintUpdateMode, property string) error {
	existing, err := u.client.GetBlueprint(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to fetch blueprint before ignoring property %s: %w", property, err)
	}
	retry := cloneBlueprint(blueprint)
	retryProps := ensureProperties(retry)
	if existingProps, ok := existing["properties"].(map[string]interface{}); ok {
		if existingProp, ok := existingProps[property]; ok {
			retryProps[property] = existingProp
		} else {
			delete(retryProps, property)
		}
	} else {
		delete(retryProps, property)
	}
	if _, err := u.applyUpdate(ctx, id, retry, mode); err != nil {
		return err
	}
	u.warn(fmt.Sprintf("Blueprint %s: ignored property %s schema update after forbidden_format_change", id, property))
	return nil
}

func (u *BlueprintUpdater) recreateProperty(ctx context.Context, id string, blueprint api.Blueprint, mode BlueprintUpdateMode, property string) error {
	existing, err := u.client.GetBlueprint(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to fetch blueprint before recreating property %s: %w", property, err)
	}
	existingProps, _ := existing["properties"].(map[string]interface{})
	currentSchema, hasCurrent := existingProps[property]
	if !hasCurrent {
		return fmt.Errorf("cannot recreate property %s on blueprint %s: property does not exist in target", property, id)
	}
	desiredProps, _ := blueprint["properties"].(map[string]interface{})
	desiredSchema, hasDesired := desiredProps[property]
	if !hasDesired {
		return fmt.Errorf("cannot recreate property %s on blueprint %s: desired property schema is missing", property, id)
	}

	tempProperty, err := tempPropertyName(property)
	if err != nil {
		return err
	}
	restored := false
	backupMsg := func(baseErr error) error {
		if restored {
			return baseErr
		}
		return fmt.Errorf("%w; values may be preserved in temporary property %s on blueprint %s", baseErr, tempProperty, id)
	}

	withTemp := cloneBlueprint(existing)
	ensureProperties(withTemp)[tempProperty] = currentSchema
	if _, err := u.client.UpdateBlueprint(ctx, id, stripSystemFieldsForErrorHandling(withTemp)); err != nil {
		return fmt.Errorf("failed to add temporary property %s: %w", tempProperty, err)
	}
	if err := u.copyPropertyValues(ctx, id, property, tempProperty); err != nil {
		return backupMsg(fmt.Errorf("failed to copy property %s values into %s: %w", property, tempProperty, err))
	}

	withoutOriginal := cloneBlueprint(withTemp)
	delete(ensureProperties(withoutOriginal), property)
	if _, err := u.client.UpdateBlueprint(ctx, id, stripSystemFieldsForErrorHandling(withoutOriginal)); err != nil {
		return backupMsg(fmt.Errorf("failed to remove original property %s: %w", property, err))
	}

	withDesired := cloneBlueprint(withoutOriginal)
	ensureProperties(withDesired)[property] = desiredSchema
	if _, err := u.client.UpdateBlueprint(ctx, id, stripSystemFieldsForErrorHandling(withDesired)); err != nil {
		return backupMsg(fmt.Errorf("failed to add recreated property %s: %w", property, err))
	}
	if err := u.copyPropertyValues(ctx, id, tempProperty, property); err != nil {
		return backupMsg(fmt.Errorf("failed to restore property %s values from %s: %w", property, tempProperty, err))
	}
	restored = true

	cleanup := cloneBlueprint(withDesired)
	delete(ensureProperties(cleanup), tempProperty)
	if _, err := u.client.UpdateBlueprint(ctx, id, stripSystemFieldsForErrorHandling(cleanup)); err != nil {
		u.warn(fmt.Sprintf("Blueprint %s: recreated property %s but failed to remove temporary property %s: %v", id, property, tempProperty, err))
	}

	if _, err := u.applyUpdate(ctx, id, blueprint, mode); err != nil {
		return err
	}
	u.warn(fmt.Sprintf("Blueprint %s: recreated property %s after forbidden_format_change", id, property))
	return nil
}

func (u *BlueprintUpdater) copyPropertyValues(ctx context.Context, blueprintID, sourceProperty, targetProperty string) error {
	migration, err := u.client.CreateMigration(ctx, api.MigrationRequest{
		SourceBlueprint: blueprintID,
		Mapping: map[string]interface{}{
			"blueprint": blueprintID,
			"entity": map[string]interface{}{
				"identifier": ".identifier",
				"title":      ".title",
				"properties": map[string]interface{}{
					targetProperty: jqProperty(sourceProperty),
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return u.waitForMigration(ctx, migration)
}

func (u *BlueprintUpdater) waitForMigration(ctx context.Context, migration api.Migration) error {
	status := migrationStatus(migration)
	if migrationStatusFailed(status) {
		return fmt.Errorf("migration %s failed with status %q", migrationIdentifier(migration), status)
	}
	if migrationStatusSucceeded(status) {
		return nil
	}

	identifier := migrationIdentifier(migration)
	if identifier == "" {
		return fmt.Errorf("migration response is missing identifier; cannot wait for completion")
	}

	waitTimeout := u.options.MigrationWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = defaultMigrationWaitTimeout
	}
	pollInterval := u.options.MigrationPollInterval
	if pollInterval <= 0 {
		pollInterval = defaultMigrationPollInterval
	}

	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for migration %s to complete: %w", identifier, waitCtx.Err())
		case <-timer.C:
			current, err := u.client.GetMigration(waitCtx, identifier)
			if err != nil {
				return fmt.Errorf("failed to get migration %s status: %w", identifier, err)
			}
			status := migrationStatus(current)
			if migrationStatusFailed(status) {
				return fmt.Errorf("migration %s failed with status %q", identifier, status)
			}
			if migrationStatusSucceeded(status) {
				return nil
			}
			timer.Reset(pollInterval)
		}
	}
}

func migrationIdentifier(migration api.Migration) string {
	for _, key := range []string{"identifier", "id"} {
		if value, ok := migration[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func migrationStatus(migration api.Migration) string {
	if value, ok := migration["status"].(string); ok && value != "" {
		return strings.ToUpper(strings.TrimSpace(value))
	}
	return ""
}

func migrationStatusSucceeded(status string) bool {
	return status == "COMPLETED"
}

func migrationStatusFailed(status string) bool {
	return status == "FAILURE" ||
		status == "CANCELLED" ||
		status == "PENDING_CANCELLATION"
}

func (u *BlueprintUpdater) warn(message string) {
	if u.options.AddWarning != nil {
		u.options.AddWarning(message)
	}
}

func apiErrorForCode(err error, code string) *api.APIError {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Code == code {
		return apiErr
	}
	return nil
}

func isSupportedForbiddenFormatChangeAction(action ErrorAction) bool {
	switch action {
	case ErrorActionFail, ErrorActionPrompt, ErrorActionIgnoreProperty, ErrorActionRecreateProperty:
		return true
	default:
		return false
	}
}

func cloneBlueprint(bp api.Blueprint) api.Blueprint {
	return api.Blueprint(cloneMap(map[string]interface{}(bp)))
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]interface{}); ok {
			out[k] = cloneMap(nested)
		} else {
			out[k] = v
		}
	}
	return out
}

func ensureProperties(bp api.Blueprint) map[string]interface{} {
	if props, ok := bp["properties"].(map[string]interface{}); ok {
		return props
	}
	props := make(map[string]interface{})
	bp["properties"] = props
	return props
}

func tempPropertyName(property string) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failed to generate temporary property name: %w", err)
	}
	return fmt.Sprintf("port_cli_tmp_%s_%s", sanitizeTempPropertyPart(property), hex.EncodeToString(b[:])), nil
}

func sanitizeTempPropertyPart(property string) string {
	var b strings.Builder
	for _, r := range property {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "property"
	}
	return b.String()
}

func jqProperty(property string) string {
	escaped := strings.ReplaceAll(property, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`.properties["%s"]`, escaped)
}

func stripSystemFieldsForErrorHandling(bp api.Blueprint) api.Blueprint {
	cleaned := cloneBlueprint(bp)
	for _, field := range []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"} {
		delete(cleaned, field)
	}
	return cleaned
}
