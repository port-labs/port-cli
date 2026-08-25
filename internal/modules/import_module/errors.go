package import_module

import (
	"fmt"
	"strings"
	"sync"
)

// ErrorCategory represents the type of error encountered during import.
type ErrorCategory string

const (
	// ErrDependency indicates a missing blueprint/entity reference.
	// These errors may resolve after dependencies are created.
	ErrDependency ErrorCategory = "DEPENDENCY"

	// ErrAuth indicates authentication or permission issues.
	ErrAuth ErrorCategory = "AUTH"

	// ErrBlueprintConfig indicates blueprint configuration prevents the operation.
	// E.g., inherited ownership enabled, protected blueprints, etc.
	ErrBlueprintConfig ErrorCategory = "BLUEPRINT_CONFIG"

	// ErrValidation indicates invalid data format or values.
	ErrValidation ErrorCategory = "VALIDATION"

	// ErrSchemaMismatch indicates entity data doesn't match blueprint schema.
	ErrSchemaMismatch ErrorCategory = "SCHEMA_MISMATCH"

	// ErrRateLimit indicates the API throttled the request.
	ErrRateLimit ErrorCategory = "RATE_LIMIT"

	// ErrNetwork indicates connection or network issues.
	ErrNetwork ErrorCategory = "NETWORK"

	// ErrConflict indicates the resource already exists.
	ErrConflict ErrorCategory = "CONFLICT"

	// ErrNotFound indicates the resource was not found.
	ErrNotFound ErrorCategory = "NOT_FOUND"

	// ErrServerError indicates the API returned a 5xx server-side error.
	ErrServerError ErrorCategory = "SERVER_ERROR"

	// ErrUnknown indicates an unexpected error.
	ErrUnknown ErrorCategory = "UNKNOWN"
)

// ImportError represents a categorized error from an import operation.
type ImportError struct {
	Category     ErrorCategory
	ResourceType string // "blueprint", "entity", "action", etc.
	ResourceID   string // identifier of the resource
	Message      string
	Cause        error
	Retryable    bool
}

func (e *ImportError) Error() string {
	if e.ResourceID != "" {
		return fmt.Sprintf("[%s] %s %s: %s", e.Category, e.ResourceType, e.ResourceID, e.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Category, e.ResourceType, e.Message)
}

// CategorizeError analyzes an error and returns an ImportError with appropriate category.
func CategorizeError(err error, resourceType, resourceID string) *ImportError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())
	ie := &ImportError{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Message:      err.Error(),
		Cause:        err,
	}

	// Check for blueprint configuration errors (inherited ownership, protected, etc.)
	if containsAny(errStr, []string{
		"inherited_ownership_enabled",
		"inherited ownership",
		"protected_resource",
		"protected resource",
		"protected_entity",
		"protected entity",
	}) {
		ie.Category = ErrBlueprintConfig
		ie.Retryable = false
		return ie
	}

	// Check for schema mismatch errors
	if containsAny(errStr, []string{
		"blueprint_schema_mismatch",
		"schema mismatch",
		"missing required property",
		"required property",
		"missing_property",
		"required_relation",
		"relation is required",
	}) {
		ie.Category = ErrSchemaMismatch
		ie.Retryable = false
		return ie
	}

	// Check for dependency errors (missing references)
	if containsAny(errStr, []string{
		"was not found",
		"not found",
		"not_found",
		"does not exist",
		"missing blueprint",
		"target blueprint",
		"relation target",
		"invalid relation",
		"blueprint with identifier",
		"after_item_not_in_parent",
		"is not in the parent folder",
	}) {
		ie.Category = ErrDependency
		ie.Retryable = true
		return ie
	}

	// Check for auth errors
	if containsAny(errStr, []string{
		"unauthorized",
		"forbidden",
		"authentication failed",
		"invalid credentials",
		"invalid_credentials",
		"access denied",
		"permission denied",
		"401",
		"403",
	}) {
		ie.Category = ErrAuth
		ie.Retryable = false
		return ie
	}

	// Check for rate limit errors
	if containsAny(errStr, []string{
		"rate limit",
		"too many requests",
		"throttle",
		"429",
	}) {
		ie.Category = ErrRateLimit
		ie.Retryable = true
		return ie
	}

	// Check for network errors
	if containsAny(errStr, []string{
		"connection refused",
		"connection reset",
		"timeout",
		"no such host",
		"network unreachable",
		"context canceled",
		"context deadline exceeded",
		"i/o timeout",
		"eof",
	}) {
		ie.Category = ErrNetwork
		ie.Retryable = true
		return ie
	}

	// Check for conflict errors
	if containsAny(errStr, []string{
		"conflict",
		"already exists",
		"duplicate",
		"409",
	}) {
		ie.Category = ErrConflict
		ie.Retryable = false
		return ie
	}

	// Check for validation errors
	if containsAny(errStr, []string{
		"validation",
		"invalid",
		"required field",
		"bad request",
		"malformed",
		"400",
	}) {
		ie.Category = ErrValidation
		ie.Retryable = false
		return ie
	}

	// Check for not found (different from dependency - resource itself not found)
	if containsAny(errStr, []string{"404"}) {
		ie.Category = ErrNotFound
		ie.Retryable = false
		return ie
	}

	// Check for server-side errors (5xx responses)
	if containsAny(errStr, []string{
		"internal_error",
		"internal server error",
		"500",
	}) {
		ie.Category = ErrServerError
		ie.Retryable = false
		return ie
	}

	// Default to unknown
	ie.Category = ErrUnknown
	ie.Retryable = false
	return ie
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ErrorCollector collects and categorizes errors during import.
type ErrorCollector struct {
	mu     sync.Mutex
	errors []*ImportError

	// Grouped views (populated on demand)
	byCategory map[ErrorCategory][]*ImportError
	byResource map[string][]*ImportError
}

// NewErrorCollector creates a new error collector.
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{
		errors:     make([]*ImportError, 0),
		byCategory: make(map[ErrorCategory][]*ImportError),
		byResource: make(map[string][]*ImportError),
	}
}

// Add adds an error to the collector.
func (ec *ErrorCollector) Add(err error, resourceType, resourceID string) {
	if err == nil {
		return
	}

	ie := CategorizeError(err, resourceType, resourceID)
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.errors = append(ec.errors, ie)
	ec.byCategory[ie.Category] = append(ec.byCategory[ie.Category], ie)
	ec.byResource[resourceType] = append(ec.byResource[resourceType], ie)
}

// AddImportError adds a pre-categorized ImportError.
func (ec *ErrorCollector) AddImportError(ie *ImportError) {
	if ie == nil {
		return
	}

	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.errors = append(ec.errors, ie)
	ec.byCategory[ie.Category] = append(ec.byCategory[ie.Category], ie)
	ec.byResource[ie.ResourceType] = append(ec.byResource[ie.ResourceType], ie)
}

// HasErrors returns true if any errors were collected.
func (ec *ErrorCollector) HasErrors() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return len(ec.errors) > 0
}

// Count returns the total number of errors.
func (ec *ErrorCollector) Count() int {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return len(ec.errors)
}

// CountByCategory returns the count of errors for a category.
func (ec *ErrorCollector) CountByCategory(cat ErrorCategory) int {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return len(ec.byCategory[cat])
}

// GetRetryable returns all errors that are retryable.
func (ec *ErrorCollector) GetRetryable() []*ImportError {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	retryable := make([]*ImportError, 0)
	for _, e := range ec.errors {
		if e.Retryable {
			retryable = append(retryable, e)
		}
	}
	return retryable
}

// GetByCategory returns errors for a specific category.
func (ec *ErrorCollector) GetByCategory(cat ErrorCategory) []*ImportError {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.byCategory[cat]
}

// GetByResource returns errors for a specific resource type.
func (ec *ErrorCollector) GetByResource(resourceType string) []*ImportError {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.byResource[resourceType]
}

// All returns all collected errors.
func (ec *ErrorCollector) All() []*ImportError {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	result := make([]*ImportError, len(ec.errors))
	copy(result, ec.errors)
	return result
}

// Summary returns a human-readable summary of errors.
// Shows count + first N examples per category.
func (ec *ErrorCollector) Summary(maxExamplesPerCategory int) string {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if len(ec.errors) == 0 {
		return "No errors"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Total errors: %d\n\n", len(ec.errors))

	// Order categories for consistent output
	categories := []ErrorCategory{
		ErrBlueprintConfig,
		ErrSchemaMismatch,
		ErrDependency,
		ErrAuth,
		ErrValidation,
		ErrRateLimit,
		ErrNetwork,
		ErrConflict,
		ErrNotFound,
		ErrServerError,
		ErrUnknown,
	}

	for _, cat := range categories {
		errs := ec.byCategory[cat]
		if len(errs) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "%s (%d):\n", cat, len(errs))

		shown := maxExamplesPerCategory
		if shown > len(errs) {
			shown = len(errs)
		}

		for i := 0; i < shown; i++ {
			e := errs[i]
			fmt.Fprintf(&sb, "  - %s %s: %s\n", e.ResourceType, e.ResourceID, truncate(e.Message, 100))
		}

		if len(errs) > shown {
			fmt.Fprintf(&sb, "  ... and %d more\n", len(errs)-shown)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToStringSlice converts errors to a simple string slice (for backward compatibility).
func (ec *ErrorCollector) ToStringSlice() []string {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	result := make([]string, len(ec.errors))
	for i, e := range ec.errors {
		result[i] = e.Error()
	}
	return result
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Clear removes all collected errors.
func (ec *ErrorCollector) Clear() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.errors = make([]*ImportError, 0)
	ec.byCategory = make(map[ErrorCategory][]*ImportError)
	ec.byResource = make(map[string][]*ImportError)
}
