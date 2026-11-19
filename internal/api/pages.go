package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PageListOptions sets the query parameters for GET /pages.
type PageListOptions struct {
	// Compact returns page metadata only, without widgets.
	Compact bool
	// IncludePrivate also returns pages with private visibility.
	IncludePrivate bool
}

// pageWriteResponse is the documented body of page PATCH, PUT and duplicate,
// and widget PATCH: {"ok": true, "identifier": "..."}.
type pageWriteResponse struct {
	Identifier string `json:"identifier"`
}

// decodeOptionalJSON decodes resp's body into out. An empty body is accepted
// because some Pages API routes document a 201 with no response schema.
func decodeOptionalJSON(resp *http.Response, out interface{}, what string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read %s response: %w", what, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to decode %s response: %w", what, err)
	}
	return nil
}

// GetPages retrieves all pages.
func (c *Client) GetPages(ctx context.Context, opts PageListOptions) ([]Page, error) {
	params := map[string]string{}
	if opts.Compact {
		params["compact"] = "true"
	}
	if opts.IncludePrivate {
		params["includePrivate"] = "true"
	}
	if len(params) == 0 {
		params = nil
	}

	resp, err := c.request(ctx, "GET", "/pages", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Pages []Page `json:"pages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode pages: %w", err)
	}

	return result.Pages, nil
}

// GetPage retrieves a single page by identifier.
func (c *Client) GetPage(ctx context.Context, pageIdentifier string) (Page, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/pages/%s", pageIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Page Page `json:"page"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode page: %w", err)
	}

	return result.Page, nil
}

// CreatePage creates a page and returns its identifier.
func (c *Client) CreatePage(ctx context.Context, page Page) (string, error) {
	resp, err := c.request(ctx, "POST", "/pages", page, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// The reference documents a 201 without a body schema, so accept either
	// {"page": {...}}, {"identifier": "..."} or an empty body.
	var result struct {
		Identifier string `json:"identifier"`
		Page       Page   `json:"page"`
	}
	if err := decodeOptionalJSON(resp, &result, "page create"); err != nil {
		return "", err
	}
	if id, _ := result.Page["identifier"].(string); id != "" {
		return id, nil
	}
	if result.Identifier != "" {
		return result.Identifier, nil
	}
	id, _ := page["identifier"].(string)
	return id, nil
}

// UpdatePage partially updates a page (PATCH) and returns its identifier.
func (c *Client) UpdatePage(ctx context.Context, pageIdentifier string, page Page) (string, error) {
	return c.writePage(ctx, "PATCH", fmt.Sprintf("/pages/%s", pageIdentifier), page, pageIdentifier, "page update")
}

// ReplacePage replaces a page (PUT) and returns its identifier.
func (c *Client) ReplacePage(ctx context.Context, pageIdentifier string, page Page) (string, error) {
	return c.writePage(ctx, "PUT", fmt.Sprintf("/pages/%s", pageIdentifier), page, pageIdentifier, "page replace")
}

// DuplicatePage copies a page, including its permissions, and returns the new page's identifier.
func (c *Client) DuplicatePage(ctx context.Context, pageIdentifier string) (string, error) {
	return c.writePage(ctx, "POST", fmt.Sprintf("/pages/%s/duplicate", pageIdentifier), nil, "", "page duplicate")
}

// writePage sends a write that responds with {"ok": true, "identifier": "..."}.
// If a successful response has no identifier, knownID is returned instead; an
// empty knownID (duplicate, where the new identifier is unknown) is an error.
func (c *Client) writePage(ctx context.Context, method, path string, body interface{}, knownID, what string) (string, error) {
	resp, err := c.request(ctx, method, path, body, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result pageWriteResponse
	if err := decodeOptionalJSON(resp, &result, what); err != nil {
		return "", err
	}
	if result.Identifier != "" {
		return result.Identifier, nil
	}
	if knownID == "" {
		return "", fmt.Errorf("%s response did not include an identifier", what)
	}
	return knownID, nil
}

// DeletePage deletes a page.
func (c *Client) DeletePage(ctx context.Context, pageIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/pages/%s", pageIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetPagesAPIVersion returns the organization's Pages API version: "beta" or "GA".
func (c *Client) GetPagesAPIVersion(ctx context.Context) (string, error) {
	resp, err := c.request(ctx, "GET", "/pages/version", nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode pages API version: %w", err)
	}
	return result.Version, nil
}

// GetPagePermissions retrieves permissions for a page.
func (c *Client) GetPagePermissions(ctx context.Context, pageIdentifier string) (Permissions, error) {
	return c.pagePermissions(ctx, "GET", fmt.Sprintf("/pages/%s/permissions", pageIdentifier), nil, "page permissions")
}

// UpdatePagePermissions updates permissions for a page.
func (c *Client) UpdatePagePermissions(ctx context.Context, pageIdentifier string, permissions Permissions) (Permissions, error) {
	return c.pagePermissions(ctx, "PATCH", fmt.Sprintf("/pages/%s/permissions", pageIdentifier), permissions, "updated page permissions")
}

// GetOrgPagePermissions retrieves organization-level page permissions (who can create pages).
func (c *Client) GetOrgPagePermissions(ctx context.Context) (Permissions, error) {
	return c.pagePermissions(ctx, "GET", "/pages/permissions", nil, "organization page permissions")
}

// UpdateOrgPagePermissions updates organization-level page permissions.
func (c *Client) UpdateOrgPagePermissions(ctx context.Context, permissions Permissions) (Permissions, error) {
	return c.pagePermissions(ctx, "PATCH", "/pages/permissions", permissions, "updated organization page permissions")
}

func (c *Client) pagePermissions(ctx context.Context, method, path string, body interface{}, what string) (Permissions, error) {
	resp, err := c.request(ctx, method, path, body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode %s: %w", what, err)
	}
	return result.Permissions, nil
}

// CreatePageWidget adds a widget to a page under parentWidgetID. The reference
// documents no response body, so the decoded body is returned as-is and may be empty.
func (c *Client) CreatePageWidget(ctx context.Context, pageIdentifier, parentWidgetID string, widget map[string]interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"parentWidgetId": parentWidgetID,
		"widget":         widget,
	}
	resp, err := c.request(ctx, "POST", fmt.Sprintf("/pages/%s/widgets", pageIdentifier), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := map[string]interface{}{}
	if err := decodeOptionalJSON(resp, &result, "widget create"); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdatePageWidget partially updates a widget. A null value clears that property.
func (c *Client) UpdatePageWidget(ctx context.Context, pageIdentifier, widgetID string, patch map[string]interface{}) (string, error) {
	return c.writePage(ctx, "PATCH", fmt.Sprintf("/pages/%s/widgets/%s", pageIdentifier, widgetID), patch, widgetID, "widget update")
}

// DeletePageWidget removes a widget from a page.
func (c *Client) DeletePageWidget(ctx context.Context, pageIdentifier, widgetID string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/pages/%s/widgets/%s", pageIdentifier, widgetID), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
