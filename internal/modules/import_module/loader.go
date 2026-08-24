package import_module

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
)

// Loader loads data from tar.gz or JSON files.
type Loader struct{}

// NewLoader creates a new loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadData loads data from a file (tar.gz or JSON).
func (l *Loader) LoadData(inputPath string) (*export.Data, error) {
	// Check if file exists
	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("input file does not exist: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(inputPath))
	suffixes := strings.Split(strings.ToLower(inputPath), ".")

	isTar := ext == ".gz" || strings.Contains(strings.Join(suffixes, "."), ".tar")
	isJSON := ext == ".json"

	if isTar {
		return l.loadTar(inputPath)
	} else if isJSON {
		return l.loadJSON(inputPath)
	}

	return nil, fmt.Errorf("unsupported file format: %s (expected .json or .tar.gz)", ext)
}

// loadTar loads data from a tar.gz file.
func (l *Loader) loadTar(tarPath string) (*export.Data, error) {
	file, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tar file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	data := &export.Data{
		Blueprints:   []api.Blueprint{},
		Entities:     []api.Entity{},
		Scorecards:   []api.Scorecard{},
		Actions:      []api.Action{},
		Teams:        []api.Team{},
		Users:        []api.User{},
		Folders:      []api.Folder{},
		Pages:        []api.Page{},
		Integrations: []api.Integration{},
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if !strings.HasSuffix(header.Name, ".json") {
			continue
		}

		// Determine data type from filename
		dataType := strings.TrimSuffix(header.Name, ".json")

		// Parse JSON and assign to appropriate field
		dec := json.NewDecoder(tr)
		switch dataType {
		case "blueprints":
			var items []api.Blueprint
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse blueprints: %w", err)
			}
			data.Blueprints = items

		case "entities":
			var items []api.Entity
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse entities: %w", err)
			}
			data.Entities = items

		case "scorecards":
			var items []api.Scorecard
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse scorecards: %w", err)
			}
			data.Scorecards = items

		case "actions":
			var items []api.Action
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse actions: %w", err)
			}
			data.Actions = items

		case "teams":
			var items []api.Team
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse teams: %w", err)
			}
			data.Teams = items

		case "users":
			var items []api.User
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse users: %w", err)
			}
			data.Users = items

		case "automations":
			// Backward compatibility: merge automations into actions
			var items []api.Action
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse automations: %w", err)
			}
			data.Actions = append(data.Actions, items...)

		case "pages":
			var items []api.Page
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse pages: %w", err)
			}
			data.Pages = items

		case "_folders":
			var items []api.Folder
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse folders: %w", err)
			}
			data.Folders = items

		case "integrations":
			var items []api.Integration
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse integrations: %w", err)
			}
			data.Integrations = items

		case "blueprint_permissions":
			var items map[string]api.Permissions
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse blueprint permissions: %w", err)
			}
			data.BlueprintPermissions = items

		case "action_permissions":
			var items map[string]api.Permissions
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse action permissions: %w", err)
			}
			data.ActionPermissions = items

		case "page_permissions":
			var items map[string]api.Permissions
			if err := dec.Decode(&items); err != nil {
				return nil, fmt.Errorf("failed to parse page permissions: %w", err)
			}
			data.PagePermissions = items
		}
	}

	return data, nil
}

// loadJSON loads data from a JSON file.
func (l *Loader) loadJSON(jsonPath string) (*export.Data, error) {
	file, err := os.Open(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON file: %w", err)
	}
	defer file.Close()

	var rawData map[string]interface{}
	if err := json.NewDecoder(file).Decode(&rawData); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	data := &export.Data{
		Blueprints:   []api.Blueprint{},
		Entities:     []api.Entity{},
		Scorecards:   []api.Scorecard{},
		Actions:      []api.Action{},
		Teams:        []api.Team{},
		Users:        []api.User{},
		Folders:      []api.Folder{},
		Pages:        []api.Page{},
		Integrations: []api.Integration{},
	}

	// Convert map[string]interface{} to typed slices
	if blueprints, ok := rawData["blueprints"].([]interface{}); ok {
		for _, bp := range blueprints {
			if bpMap, ok := bp.(map[string]interface{}); ok {
				data.Blueprints = append(data.Blueprints, api.Blueprint(bpMap))
			}
		}
	}

	if entities, ok := rawData["entities"].([]interface{}); ok {
		for _, e := range entities {
			if eMap, ok := e.(map[string]interface{}); ok {
				data.Entities = append(data.Entities, api.Entity(eMap))
			}
		}
	}

	if scorecards, ok := rawData["scorecards"].([]interface{}); ok {
		for _, sc := range scorecards {
			if scMap, ok := sc.(map[string]interface{}); ok {
				data.Scorecards = append(data.Scorecards, api.Scorecard(scMap))
			}
		}
	}

	if actions, ok := rawData["actions"].([]interface{}); ok {
		for _, a := range actions {
			if aMap, ok := a.(map[string]interface{}); ok {
				data.Actions = append(data.Actions, api.Action(aMap))
			}
		}
	}

	if teams, ok := rawData["teams"].([]interface{}); ok {
		for _, t := range teams {
			if tMap, ok := t.(map[string]interface{}); ok {
				data.Teams = append(data.Teams, api.Team(tMap))
			}
		}
	}

	if users, ok := rawData["users"].([]interface{}); ok {
		for _, u := range users {
			if uMap, ok := u.(map[string]interface{}); ok {
				data.Users = append(data.Users, api.User(uMap))
			}
		}
	}

	// Backward compatibility: merge automations into actions
	if automations, ok := rawData["automations"].([]interface{}); ok {
		for _, a := range automations {
			if aMap, ok := a.(map[string]interface{}); ok {
				data.Actions = append(data.Actions, api.Action(aMap))
			}
		}
	}

	if pages, ok := rawData["pages"].([]interface{}); ok {
		for _, p := range pages {
			if pMap, ok := p.(map[string]interface{}); ok {
				data.Pages = append(data.Pages, api.Page(pMap))
			}
		}
	}

	if folders, ok := rawData["_folders"].([]interface{}); ok {
		for _, f := range folders {
			if fMap, ok := f.(map[string]interface{}); ok {
				data.Folders = append(data.Folders, api.Folder(fMap))
			}
		}
	}

	if integrations, ok := rawData["integrations"].([]interface{}); ok {
		for _, i := range integrations {
			if iMap, ok := i.(map[string]interface{}); ok {
				data.Integrations = append(data.Integrations, api.Integration(iMap))
			}
		}
	}

	for _, key := range []string{"BlueprintPermissions", "blueprint_permissions"} {
		if perms, ok := rawData[key].(map[string]interface{}); ok {
			data.BlueprintPermissions = make(map[string]api.Permissions)
			for id, p := range perms {
				if pMap, ok := p.(map[string]interface{}); ok {
					data.BlueprintPermissions[id] = api.Permissions(pMap)
				}
			}
			break
		}
	}

	for _, key := range []string{"ActionPermissions", "action_permissions"} {
		if perms, ok := rawData[key].(map[string]interface{}); ok {
			data.ActionPermissions = make(map[string]api.Permissions)
			for id, p := range perms {
				if pMap, ok := p.(map[string]interface{}); ok {
					data.ActionPermissions[id] = api.Permissions(pMap)
				}
			}
			break
		}
	}

	for _, key := range []string{"PagePermissions", "page_permissions"} {
		if perms, ok := rawData[key].(map[string]interface{}); ok {
			data.PagePermissions = make(map[string]api.Permissions)
			for id, p := range perms {
				if pMap, ok := p.(map[string]interface{}); ok {
					data.PagePermissions[id] = api.Permissions(pMap)
				}
			}
			break
		}
	}

	return data, nil
}

// ValidateData validates the loaded data structure.
// When includeResources is non-empty, blueprints are only required if
// blueprints (or blueprint-dependent types like entities/scorecards) are
// being imported. Org-level resources (pages, integrations, teams, users)
// can be imported without blueprints in the file.
func (l *Loader) ValidateData(data *export.Data, includeResources []string) error {
	if len(includeResources) > 0 {
		blueprintsNeeded := false
		for _, r := range includeResources {
			switch r {
			case "blueprints", "entities", "scorecards", "blueprint-permissions":
				blueprintsNeeded = true
			}
		}
		if blueprintsNeeded && len(data.Blueprints) == 0 {
			return fmt.Errorf("missing required data: blueprints")
		}
		return nil
	}
	if len(data.Blueprints) == 0 {
		return fmt.Errorf("missing required data: blueprints")
	}
	return nil
}
