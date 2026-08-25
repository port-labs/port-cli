package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// Blueprint represents a Port blueprint.
type Blueprint map[string]interface{}

// Entity represents a Port entity.
type Entity map[string]interface{}

// Scorecard represents a Port scorecard.
type Scorecard map[string]interface{}

// Action represents a Port action.
type Action map[string]interface{}

// Team represents a Port team.
type Team map[string]interface{}

// User represents a Port user.
type User map[string]interface{}

// Automation represents a Port automation.
type Automation map[string]interface{}

// Page represents a Port page.
type Page map[string]interface{}

// Folder represents a Port sidebar folder.
type Folder map[string]interface{}

// Integration represents a Port integration.
type Integration map[string]interface{}

// Permissions represents Port resource permissions.
type Permissions map[string]interface{}

// MigrationRequest represents a Port migration request.
type MigrationRequest struct {
	SourceBlueprint string                 `json:"sourceBlueprint"`
	Mapping         map[string]interface{} `json:"mapping"`
}

// Migration represents a Port migration job.
type Migration map[string]interface{}

type RequestParams struct {
	Method   string
	Endpoint string
	Data     any
	Params   map[string]string
}

func (c *Client) Request(ctx context.Context, params RequestParams) (any, error) {
	resp, err := c.request(ctx, params.Method, params.Endpoint, params.Data, params.Params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// GetBlueprints retrieves all blueprints.
func (c *Client) GetBlueprints(ctx context.Context) ([]Blueprint, error) {
	resp, err := c.request(ctx, "GET", "/blueprints", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Blueprints []Blueprint `json:"blueprints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprints: %w", err)
	}

	return result.Blueprints, nil
}

// GetBlueprint retrieves a specific blueprint.
func (c *Client) GetBlueprint(ctx context.Context, identifier string) (Blueprint, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s", identifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Blueprint Blueprint `json:"blueprint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprint: %w", err)
	}

	return result.Blueprint, nil
}

// CreateBlueprint creates a new blueprint.
func (c *Client) CreateBlueprint(ctx context.Context, blueprint Blueprint) (Blueprint, error) {
	resp, err := c.request(ctx, "POST", "/blueprints", blueprint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Blueprint Blueprint `json:"blueprint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprint: %w", err)
	}

	return result.Blueprint, nil
}

// UpdateBlueprint updates an existing blueprint.
func (c *Client) UpdateBlueprint(ctx context.Context, identifier string, blueprint Blueprint) (Blueprint, error) {
	resp, err := c.request(ctx, "PUT", fmt.Sprintf("/blueprints/%s", identifier), blueprint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Blueprint Blueprint `json:"blueprint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprint: %w", err)
	}

	return result.Blueprint, nil
}

// PatchBlueprint updates an existing blueprint with a partial payload (PATCH).
func (c *Client) PatchBlueprint(ctx context.Context, identifier string, blueprint Blueprint) (Blueprint, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/blueprints/%s", identifier), blueprint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Blueprint Blueprint `json:"blueprint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprint: %w", err)
	}

	return result.Blueprint, nil
}

// DeleteBlueprint deletes a blueprint.
func (c *Client) DeleteBlueprint(ctx context.Context, identifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/blueprints/%s", identifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func migrationFromResponse(result map[string]interface{}) Migration {
	if migration, ok := result["migration"].(map[string]interface{}); ok {
		return Migration(migration)
	}
	return Migration(result)
}

// CreateMigration starts a Port migration.
func (c *Client) CreateMigration(ctx context.Context, migration MigrationRequest) (Migration, error) {
	resp, err := c.request(ctx, "POST", "/migrations", migration, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode migration result: %w", err)
	}
	return migrationFromResponse(result), nil
}

// GetMigration retrieves a Port migration job.
func (c *Client) GetMigration(ctx context.Context, identifier string) (Migration, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/migrations/%s", identifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode migration result: %w", err)
	}
	return migrationFromResponse(result), nil
}

// GetEntities retrieves entities for a blueprint.
func (c *Client) GetEntities(ctx context.Context, blueprintIdentifier string, params map[string]string) ([]Entity, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s/entities", blueprintIdentifier), nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Entities []Entity `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entities: %w", err)
	}

	return result.Entities, nil
}

const entitySearchPaginationThreshold = 10000

// GetEntitiesCount retrieves the number of entities for a blueprint.
func (c *Client) GetEntitiesCount(ctx context.Context, blueprintIdentifier string) (int, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s/entities-count", blueprintIdentifier), nil, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode entities count: %w", err)
	}

	return result.Count, nil
}

// ForEachEntity retrieves all entities for a blueprint and calls yield with
// each returned batch. Blueprints above the threshold use the search endpoint
// because it supports cursor pagination; smaller blueprints use the canonical
// GET endpoint.
func (c *Client) ForEachEntity(ctx context.Context, blueprintIdentifier string, yield func([]Entity) error) error {
	count, err := c.GetEntitiesCount(ctx, blueprintIdentifier)
	if err != nil {
		return err
	}

	if count > entitySearchPaginationThreshold {
		return c.ForEachEntityPage(ctx, blueprintIdentifier, paginatedEntitySearchBody(), yield)
	}

	entities, err := c.GetEntities(ctx, blueprintIdentifier, nil)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}
	return yield(entities)
}

func paginatedEntitySearchBody() map[string]interface{} {
	return map[string]interface{}{
		"query": map[string]interface{}{
			"combinator": "and",
			"rules":      []interface{}{},
		},
		"limit": 1000,
	}
}

// SearchEntities queries entities for a blueprint using Port's search endpoint.
// Pages are fetched sequentially (each page's cursor depends on the previous
// response), so this cannot be parallelized client-side. For large blueprints
// this is still far better than GetEntities, which makes a single unbounded
// request that 504s above ~10k entities.
func (c *Client) SearchEntities(ctx context.Context, blueprintIdentifier string, body map[string]interface{}) ([]Entity, error) {
	// Pre-allocate a reasonable capacity to avoid repeated slice growth.
	all := make([]Entity, 0, 256)
	err := c.ForEachEntityPage(ctx, blueprintIdentifier, body, func(entities []Entity) error {
		all = append(all, entities...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}

// ForEachEntityPage queries entities for a blueprint using Port's search
// endpoint and calls yield once for each returned page.
func (c *Client) ForEachEntityPage(ctx context.Context, blueprintIdentifier string, body map[string]interface{}, yield func([]Entity) error) error {
	var from string
	for {
		pageBody := cloneBody(body)
		if from != "" {
			pageBody["from"] = from
		}
		resp, err := c.request(ctx, "POST", fmt.Sprintf("/blueprints/%s/entities/search", blueprintIdentifier), pageBody, nil)
		if err != nil {
			return err
		}

		var result struct {
			Entities []Entity `json:"entities"`
			Next     string   `json:"next"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode entities: %w", err)
		}
		resp.Body.Close()

		if len(result.Entities) > 0 {
			if err := yield(result.Entities); err != nil {
				return err
			}
		}
		if result.Next == "" {
			return nil
		}
		from = result.Next
	}
}

// cloneBody performs a shallow top-level copy of the request body map so that
// pagination can add a "from" key without mutating the original. Nested values
// (e.g. "query", "rules") are shared by reference; callers must not mutate
// them between pages.
func cloneBody(body map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(body)+1)
	for k, v := range body {
		cloned[k] = v
	}
	return cloned
}

// TopSearchEntities queries entities using Port's top-search endpoint, which
// supports server-side sorting.
func (c *Client) TopSearchEntities(ctx context.Context, blueprintIdentifier string, body map[string]interface{}) ([]Entity, error) {
	resp, err := c.request(ctx, "POST", fmt.Sprintf("/blueprints/%s/entities/top-search", blueprintIdentifier), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Entities []Entity `json:"entities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entities: %w", err)
	}

	return result.Entities, nil
}

// GetEntity retrieves a specific entity.
func (c *Client) GetEntity(ctx context.Context, blueprintIdentifier, entityIdentifier string) (Entity, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s/entities/%s", blueprintIdentifier, entityIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Entity Entity `json:"entity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entity: %w", err)
	}

	return result.Entity, nil
}

// CreateEntity creates a new entity.
func (c *Client) CreateEntity(ctx context.Context, blueprintIdentifier string, entity Entity) (Entity, error) {
	resp, err := c.request(ctx, "POST", fmt.Sprintf("/blueprints/%s/entities", blueprintIdentifier), entity, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Entity Entity `json:"entity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entity: %w", err)
	}

	return result.Entity, nil
}

// UpdateEntity updates an existing entity.
func (c *Client) UpdateEntity(ctx context.Context, blueprintIdentifier, entityIdentifier string, entity Entity) (Entity, error) {
	resp, err := c.request(ctx, "PUT", fmt.Sprintf("/blueprints/%s/entities/%s", blueprintIdentifier, entityIdentifier), entity, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Entity Entity `json:"entity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entity: %w", err)
	}

	return result.Entity, nil
}

// DeleteEntity deletes an entity.
func (c *Client) DeleteEntity(ctx context.Context, blueprintIdentifier, entityIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/blueprints/%s/entities/%s", blueprintIdentifier, entityIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// BulkDeleteEntities deletes multiple entities for a blueprint.
func (c *Client) BulkDeleteEntities(ctx context.Context, blueprintIdentifier string, entityIdentifiers []string, deleteDependents bool) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"entities": entityIdentifiers,
	}

	params := map[string]string{}
	if deleteDependents {
		params["delete_dependents"] = "true"
	} else {
		params["delete_dependents"] = "false"
	}

	resp, err := c.request(ctx, "POST", fmt.Sprintf("/blueprints/%s/bulk/entities/delete", blueprintIdentifier), payload, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetScorecards retrieves scorecards for a blueprint.
func (c *Client) GetScorecards(ctx context.Context, blueprintIdentifier string) ([]Scorecard, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s/scorecards", blueprintIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Scorecards []Scorecard `json:"scorecards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode scorecards: %w", err)
	}

	return result.Scorecards, nil
}

// GetAllScorecards retrieves all scorecards (organization-wide).
func (c *Client) GetAllScorecards(ctx context.Context) ([]Scorecard, error) {
	resp, err := c.request(ctx, "GET", "/scorecards", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Scorecards []Scorecard `json:"scorecards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode scorecards: %w", err)
	}

	return result.Scorecards, nil
}

// CreateScorecard creates a new scorecard for a blueprint.
func (c *Client) CreateScorecard(ctx context.Context, blueprintIdentifier string, scorecard Scorecard) (Scorecard, error) {
	resp, err := c.request(ctx, "POST", fmt.Sprintf("/blueprints/%s/scorecards", blueprintIdentifier), scorecard, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Scorecard Scorecard `json:"scorecard"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode scorecard: %w", err)
	}

	return result.Scorecard, nil
}

// UpdateScorecard updates an existing scorecard.
func (c *Client) UpdateScorecard(ctx context.Context, blueprintIdentifier, scorecardIdentifier string, scorecard Scorecard) (Scorecard, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/blueprints/%s/scorecards/%s", blueprintIdentifier, scorecardIdentifier), scorecard, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Scorecard Scorecard `json:"scorecard"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode scorecard: %w", err)
	}

	return result.Scorecard, nil
}

// UpdateScorecards updates multiple scorecards for a blueprint using bulk PUT endpoint.
// The API expects the array of scorecards directly (not wrapped in an object).
func (c *Client) UpdateScorecards(ctx context.Context, blueprintIdentifier string, scorecards []Scorecard) ([]Scorecard, error) {
	// Send array directly - API does not expect {"scorecards": [...]} wrapper
	resp, err := c.request(ctx, "PUT", fmt.Sprintf("/blueprints/%s/scorecards", blueprintIdentifier), scorecards, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Scorecards []Scorecard `json:"scorecards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode scorecards: %w", err)
	}

	return result.Scorecards, nil
}

// DeleteScorecard deletes a scorecard.
func (c *Client) DeleteScorecard(ctx context.Context, blueprintIdentifier, scorecardIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/blueprints/%s/scorecards/%s", blueprintIdentifier, scorecardIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetActions retrieves actions for a blueprint using the organization-wide
// actions endpoint and client-side blueprint filtering.
func (c *Client) GetActions(ctx context.Context, blueprintIdentifier string) ([]Action, error) {
	allActions, err := c.GetAllActions(ctx)
	if err != nil {
		return nil, err
	}

	actions := make([]Action, 0)
	for _, action := range allActions {
		if SelfServiceActionBlueprintID(action) == blueprintIdentifier {
			actions = append(actions, action)
		}
	}
	return actions, nil
}

// CreateAction creates a blueprint-level action using the organization-wide
// actions endpoint.
func (c *Client) CreateAction(ctx context.Context, blueprintIdentifier string, action Action) (Action, error) {
	action = ActionWithBlueprintIdentifier(action, blueprintIdentifier)
	resp, err := c.request(ctx, "POST", "/actions", action, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Action Action `json:"action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action: %w", err)
	}

	return result.Action, nil
}

// UpdateAction updates an existing blueprint-level action using the
// organization-wide actions endpoint.
func (c *Client) UpdateAction(ctx context.Context, blueprintIdentifier, actionIdentifier string, action Action) (Action, error) {
	action = ActionWithBlueprintIdentifier(action, blueprintIdentifier)
	resp, err := c.request(ctx, "PUT", fmt.Sprintf("/actions/%s", actionIdentifier), action, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Action Action `json:"action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action: %w", err)
	}

	return result.Action, nil
}

// DeleteAction deletes a blueprint-level action using the organization-wide
// actions endpoint.
func (c *Client) DeleteAction(ctx context.Context, blueprintIdentifier, actionIdentifier string) error {
	return c.DeleteActionByID(ctx, actionIdentifier)
}

// DeleteActionByID deletes an action using the organization-wide actions endpoint.
func (c *Client) DeleteActionByID(ctx context.Context, actionIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/actions/%s", actionIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ActionBlueprintID extracts the blueprint identifier an action or automation
// references, if any. Self-service actions carry it at trigger.blueprintIdentifier;
// automations can carry it at trigger.event.blueprintIdentifier.
func ActionBlueprintID(action Action) string {
	trigger, ok := action["trigger"].(map[string]interface{})
	if !ok {
		return ""
	}
	if bpID, ok := trigger["blueprintIdentifier"].(string); ok && bpID != "" {
		return bpID
	}
	if event, ok := trigger["event"].(map[string]interface{}); ok {
		if bpID, ok := event["blueprintIdentifier"].(string); ok {
			return bpID
		}
	}
	return ""
}

// SelfServiceActionBlueprintID extracts the blueprint identifier from a
// non-automation action. Automations are excluded even when their event
// references a blueprint.
func SelfServiceActionBlueprintID(action Action) string {
	if IsAutomationAction(action) {
		return ""
	}
	trigger, ok := action["trigger"].(map[string]interface{})
	if !ok {
		return ""
	}
	bpID, _ := trigger["blueprintIdentifier"].(string)
	return bpID
}

// IsAutomationAction reports whether an action record is an automation.
func IsAutomationAction(action Action) bool {
	trigger, ok := action["trigger"].(map[string]interface{})
	if !ok {
		return false
	}
	triggerType, _ := trigger["type"].(string)
	return triggerType == "automation"
}

// ActionWithBlueprintIdentifier returns a shallow copy of action with
// trigger.blueprintIdentifier set.
func ActionWithBlueprintIdentifier(action Action, blueprintIdentifier string) Action {
	if blueprintIdentifier == "" {
		return action
	}

	out := make(Action, len(action)+1)
	for k, v := range action {
		out[k] = v
	}

	trigger, _ := out["trigger"].(map[string]interface{})
	clonedTrigger := make(map[string]interface{}, len(trigger)+1)
	for k, v := range trigger {
		clonedTrigger[k] = v
	}
	clonedTrigger["blueprintIdentifier"] = blueprintIdentifier
	out["trigger"] = clonedTrigger
	return out
}

// GetTeams retrieves all teams.
func (c *Client) GetTeams(ctx context.Context) ([]Team, error) {
	resp, err := c.request(ctx, "GET", "/teams", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Teams []Team `json:"teams"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode teams: %w", err)
	}

	return result.Teams, nil
}

// CreateTeam creates a new team.
func (c *Client) CreateTeam(ctx context.Context, team Team) (Team, error) {
	resp, err := c.request(ctx, "POST", "/teams", team, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Team Team `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode team: %w", err)
	}

	return result.Team, nil
}

// UpdateTeam updates an existing team.
func (c *Client) UpdateTeam(ctx context.Context, teamName string, team Team) (Team, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/teams/%s", teamName), team, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Team Team `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode team: %w", err)
	}

	return result.Team, nil
}

// DeleteTeam deletes a team.
func (c *Client) DeleteTeam(ctx context.Context, teamName string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/teams/%s", teamName), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetUsers retrieves all users in the organization.
func (c *Client) GetUsers(ctx context.Context) ([]User, error) {
	resp, err := c.request(ctx, "GET", "/users", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Users []User `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode users: %w", err)
	}

	return result.Users, nil
}

// GetUser retrieves a specific user by email.
func (c *Client) GetUser(ctx context.Context, userEmail string) (User, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/users/%s", userEmail), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		User User `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}

	return result.User, nil
}

// BulkEntityError is a single per-entity failure returned by the bulk entity endpoint.
type BulkEntityError struct {
	Identifier string  `json:"identifier"`
	Index      float64 `json:"index"`
	StatusCode float64 `json:"statusCode"`
	Error      string  `json:"error"`
	Message    string  `json:"message"`
}

// CreateUserEntitiesBulk creates up to 20 _user blueprint entities in one call.
// Set upsert=true to overwrite existing entities; false returns 409 errors for conflicts.
func (c *Client) CreateUserEntitiesBulk(ctx context.Context, entities []Entity, upsert bool) ([]BulkEntityError, error) {
	payload := map[string]interface{}{
		"entities": entities,
	}
	path := fmt.Sprintf("/blueprints/_user/entities/bulk?upsert=%t", upsert)
	resp, err := c.request(ctx, "POST", path, payload, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Errors []BulkEntityError `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode bulk user create result: %w", err)
	}

	return result.Errors, nil
}

// BulkUpsertEntities upserts up to 20 entities for any blueprint in one call.
// Set upsert=true to overwrite existing entities; false returns 409 errors for conflicts.
func (c *Client) BulkUpsertEntities(ctx context.Context, blueprintID string, entities []Entity, upsert bool) ([]BulkEntityError, error) {
	payload := map[string]interface{}{
		"entities": entities,
	}
	path := fmt.Sprintf("/blueprints/%s/entities/bulk?upsert=%t", blueprintID, upsert)
	resp, err := c.request(ctx, "POST", path, payload, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Errors []BulkEntityError `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode bulk entity upsert result: %w", err)
	}

	return result.Errors, nil
}

// GetAllActions retrieves all actions and automations (organization-wide).
func (c *Client) GetAllActions(ctx context.Context) ([]Action, error) {
	resp, err := c.request(ctx, "GET", "/actions", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Actions []Action `json:"actions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode actions: %w", err)
	}

	return result.Actions, nil
}

// CreateAutomation creates a new automation (organization-wide action).
func (c *Client) CreateAutomation(ctx context.Context, automation Automation) (Automation, error) {
	resp, err := c.request(ctx, "POST", "/actions", automation, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Action Automation `json:"action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode automation: %w", err)
	}

	return result.Action, nil
}

// UpdateAutomation updates an existing automation.
func (c *Client) UpdateAutomation(ctx context.Context, automationIdentifier string, automation Automation) (Automation, error) {
	resp, err := c.request(ctx, "PUT", fmt.Sprintf("/actions/%s", automationIdentifier), automation, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Action Automation `json:"action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode automation: %w", err)
	}

	return result.Action, nil
}

// DeleteAutomation deletes an automation.
func (c *Client) DeleteAutomation(ctx context.Context, automationIdentifier string) error {
	return c.DeleteActionByID(ctx, automationIdentifier)
}

// GetPages retrieves all pages.
func (c *Client) GetPages(ctx context.Context) ([]Page, error) {
	resp, err := c.request(ctx, "GET", "/pages", nil, nil)
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

// CreatePage creates a new page.
func (c *Client) CreatePage(ctx context.Context, page Page) (Page, error) {
	resp, err := c.request(ctx, "POST", "/pages", page, nil)
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

// UpdatePage updates an existing page.
func (c *Client) UpdatePage(ctx context.Context, pageIdentifier string, page Page) (Page, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/pages/%s", pageIdentifier), page, nil)
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

// DeletePage deletes a page.
func (c *Client) DeletePage(ctx context.Context, pageIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/pages/%s", pageIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetFolders retrieves sidebar folders from the catalog sidebar.
func (c *Client) GetFolders(ctx context.Context) ([]Folder, error) {
	resp, err := c.request(ctx, "GET", "/sidebars/catalog", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode folders: %w", err)
	}

	var folders []Folder
	collectFoldersFromSidebarResponse(raw, &folders)

	seen := make(map[string]bool, len(folders))
	unique := make([]Folder, 0, len(folders))
	for _, folder := range folders {
		identifier, _ := folder["identifier"].(string)
		if identifier == "" || seen[identifier] {
			continue
		}
		seen[identifier] = true
		unique = append(unique, folder)
	}

	return unique, nil
}

func collectFoldersFromSidebarResponse(value interface{}, folders *[]Folder) {
	switch v := value.(type) {
	case map[string]interface{}:
		if sidebarType, ok := v["sidebarType"].(string); ok && sidebarType == "folder" {
			*folders = append(*folders, Folder(v))
		}
		for _, nested := range v {
			collectFoldersFromSidebarResponse(nested, folders)
		}
	case []interface{}:
		for _, item := range v {
			collectFoldersFromSidebarResponse(item, folders)
		}
	}
}

// CreateFolder creates a sidebar folder under the catalog sidebar.
func (c *Client) CreateFolder(ctx context.Context, folder Folder) error {
	resp, err := c.request(ctx, "POST", "/sidebars/catalog/folders", folder, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// DeleteFolder deletes a sidebar folder from the catalog sidebar.
func (c *Client) DeleteFolder(ctx context.Context, folderIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/sidebars/catalog/folders/%s", folderIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetIntegrations retrieves all integrations.
func (c *Client) GetIntegrations(ctx context.Context) ([]Integration, error) {
	resp, err := c.request(ctx, "GET", "/integration", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Integrations []Integration `json:"integrations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode integrations: %w", err)
	}

	return result.Integrations, nil
}

// UpdateIntegrationConfig updates an integration's configuration.
func (c *Client) UpdateIntegrationConfig(ctx context.Context, integrationIdentifier string, config map[string]interface{}) (Integration, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/integration/%s/config", integrationIdentifier), config, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Integration Integration `json:"integration"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode integration: %w", err)
	}

	return result.Integration, nil
}

// DeleteIntegration deletes an integration.
func (c *Client) DeleteIntegration(ctx context.Context, integrationIdentifier string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/integration/%s", integrationIdentifier), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetBlueprintPermissions retrieves permissions for a blueprint.
func (c *Client) GetBlueprintPermissions(ctx context.Context, blueprintIdentifier string) (Permissions, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/blueprints/%s/permissions", blueprintIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode blueprint permissions: %w", err)
	}

	return result.Permissions, nil
}

// UpdateBlueprintPermissions updates permissions for a blueprint.
func (c *Client) UpdateBlueprintPermissions(ctx context.Context, blueprintIdentifier string, permissions Permissions) (Permissions, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/blueprints/%s/permissions", blueprintIdentifier), permissions, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode updated blueprint permissions: %w", err)
	}

	return result.Permissions, nil
}

// GetActionPermissions retrieves permissions for an action.
func (c *Client) GetActionPermissions(ctx context.Context, actionIdentifier string) (Permissions, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/actions/%s/permissions", actionIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action permissions: %w", err)
	}

	return result.Permissions, nil
}

// UpdateActionPermissions updates permissions for an action.
func (c *Client) UpdateActionPermissions(ctx context.Context, actionIdentifier string, permissions Permissions) (Permissions, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/actions/%s/permissions", actionIdentifier), permissions, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode updated action permissions: %w", err)
	}

	return result.Permissions, nil
}

// GetPagePermissions retrieves permissions for a page.
func (c *Client) GetPagePermissions(ctx context.Context, pageIdentifier string) (Permissions, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/pages/%s/permissions", pageIdentifier), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode page permissions: %w", err)
	}

	return result.Permissions, nil
}

// UpdatePagePermissions updates permissions for a page.
func (c *Client) UpdatePagePermissions(ctx context.Context, pageIdentifier string, permissions Permissions) (Permissions, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/pages/%s/permissions", pageIdentifier), permissions, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode updated page permissions: %w", err)
	}

	return result.Permissions, nil
}

// ActionRun represents a Port action run.
type ActionRun map[string]interface{}

// GetActionRuns retrieves all action runs.
func (c *Client) GetActionRuns(ctx context.Context) ([]ActionRun, error) {
	resp, err := c.request(ctx, "GET", "/actions/runs", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Runs []ActionRun `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action runs: %w", err)
	}

	return result.Runs, nil
}

// GetActionRun retrieves a specific action run.
func (c *Client) GetActionRun(ctx context.Context, runID string) (ActionRun, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/actions/runs/%s", runID), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Run ActionRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action run: %w", err)
	}

	return result.Run, nil
}

// UpdateActionRun updates an action run (set status, message, link, logs).
func (c *Client) UpdateActionRun(ctx context.Context, runID string, body map[string]interface{}) (ActionRun, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/actions/runs/%s", runID), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Run ActionRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action run: %w", err)
	}

	return result.Run, nil
}

// ApproveActionRun approves or declines an action run.
func (c *Client) ApproveActionRun(ctx context.Context, runID string, body map[string]interface{}) (ActionRun, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/actions/runs/%s/approval", runID), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Run ActionRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action run: %w", err)
	}

	return result.Run, nil
}

// ExecuteAction creates a new action run for the given action identifier.
func (c *Client) ExecuteAction(ctx context.Context, actionID string, body map[string]interface{}) (ActionRun, error) {
	resp, err := c.request(ctx, "POST", fmt.Sprintf("/actions/%s/runs", actionID), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Run ActionRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode action run: %w", err)
	}

	return result.Run, nil
}

// Webhook represents a Port webhook.
type Webhook map[string]interface{}

// GetWebhooks retrieves all webhooks.
func (c *Client) GetWebhooks(ctx context.Context) ([]Webhook, error) {
	resp, err := c.request(ctx, "GET", "/webhooks", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Webhooks []Webhook `json:"webhooks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode webhooks: %w", err)
	}

	return result.Webhooks, nil
}

// GetWebhook retrieves a specific webhook.
func (c *Client) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	resp, err := c.request(ctx, "GET", fmt.Sprintf("/webhooks/%s", id), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Webhook Webhook `json:"webhook"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode webhook: %w", err)
	}

	return result.Webhook, nil
}

// CreateWebhook creates a new webhook.
func (c *Client) CreateWebhook(ctx context.Context, body map[string]interface{}) (Webhook, error) {
	resp, err := c.request(ctx, "POST", "/webhooks", body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Webhook Webhook `json:"webhook"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode webhook: %w", err)
	}

	return result.Webhook, nil
}

// UpdateWebhook updates an existing webhook.
func (c *Client) UpdateWebhook(ctx context.Context, id string, body map[string]interface{}) (Webhook, error) {
	resp, err := c.request(ctx, "PATCH", fmt.Sprintf("/webhooks/%s", id), body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Webhook Webhook `json:"webhook"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode webhook: %w", err)
	}

	return result.Webhook, nil
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	resp, err := c.request(ctx, "DELETE", fmt.Sprintf("/webhooks/%s", id), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// AuditLog represents a Port audit log entry.
type AuditLog map[string]interface{}

// GetAuditLogs retrieves the organization audit log.
func (c *Client) GetAuditLogs(ctx context.Context) ([]AuditLog, error) {
	resp, err := c.request(ctx, "GET", "/audit-log", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Audits []AuditLog `json:"audits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode audit logs: %w", err)
	}

	return result.Audits, nil
}
