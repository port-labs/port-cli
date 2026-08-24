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

// StreamLoader reads export archives without materializing large entity arrays.
type StreamLoader struct{}

func NewStreamLoader() *StreamLoader {
	return &StreamLoader{}
}

func (l *StreamLoader) LoadDataWithoutEntities(inputPath string) (*export.Data, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("input file does not exist: %w", err)
	}
	if isTarPath(inputPath) {
		return l.loadTarMetadata(inputPath)
	}
	if strings.ToLower(filepath.Ext(inputPath)) == ".json" {
		return l.loadJSONMetadata(inputPath)
	}
	return nil, fmt.Errorf("unsupported file format: %s (expected .json or .tar.gz)", filepath.Ext(inputPath))
}

func (l *StreamLoader) ForEachEntity(inputPath string, yield func(api.Entity) error) error {
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("input file does not exist: %w", err)
	}
	if isTarPath(inputPath) {
		return l.forEachTarEntity(inputPath, yield)
	}
	if strings.ToLower(filepath.Ext(inputPath)) == ".json" {
		return l.forEachJSONEntity(inputPath, yield)
	}
	return fmt.Errorf("unsupported file format: %s (expected .json or .tar.gz)", filepath.Ext(inputPath))
}

func isTarPath(inputPath string) bool {
	ext := strings.ToLower(filepath.Ext(inputPath))
	suffixes := strings.Split(strings.ToLower(inputPath), ".")
	return ext == ".gz" || strings.Contains(strings.Join(suffixes, "."), ".tar")
}

func emptyExportData() *export.Data {
	return &export.Data{
		Blueprints:           []api.Blueprint{},
		Entities:             []api.Entity{},
		Scorecards:           []api.Scorecard{},
		Actions:              []api.Action{},
		Teams:                []api.Team{},
		Users:                []api.User{},
		Folders:              []api.Folder{},
		Pages:                []api.Page{},
		Integrations:         []api.Integration{},
		BlueprintPermissions: make(map[string]api.Permissions),
		ActionPermissions:    make(map[string]api.Permissions),
		PagePermissions:      make(map[string]api.Permissions),
	}
}

func (l *StreamLoader) loadTarMetadata(tarPath string) (*export.Data, error) {
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
	data := emptyExportData()
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".json") {
			continue
		}
		dataType := strings.TrimSuffix(header.Name, ".json")
		if dataType == "entities" {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, err
			}
			continue
		}
		if err := decodeDataSection(json.NewDecoder(tr), dataType, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (l *StreamLoader) loadJSONMetadata(jsonPath string) (*export.Data, error) {
	file, err := os.Open(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSON file: %w", err)
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	data := emptyExportData()
	if err := readJSONObject(dec, func(key string) error {
		if key == "entities" {
			return skipJSONValue(dec)
		}
		return decodeDataSection(dec, key, data)
	}); err != nil {
		return nil, err
	}
	return data, nil
}

func (l *StreamLoader) forEachTarEntity(tarPath string, yield func(api.Entity) error) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer file.Close()
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg || strings.TrimSuffix(header.Name, ".json") != "entities" {
			continue
		}
		return decodeEntityArray(json.NewDecoder(tr), yield)
	}
}

func (l *StreamLoader) forEachJSONEntity(jsonPath string, yield func(api.Entity) error) error {
	file, err := os.Open(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to open JSON file: %w", err)
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	return readJSONObject(dec, func(key string) error {
		if key == "entities" {
			return decodeEntityArray(dec, yield)
		}
		return skipJSONValue(dec)
	})
}

func readJSONObject(dec *json.Decoder, handle func(string) error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("expected JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("expected JSON object key")
		}
		if err := handle(key); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func decodeEntityArray(dec *json.Decoder, yield func(api.Entity) error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected entities array")
	}
	for dec.More() {
		var entity api.Entity
		if err := dec.Decode(&entity); err != nil {
			return err
		}
		if err := yield(entity); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func decodeDataSection(dec *json.Decoder, key string, data *export.Data) error {
	switch key {
	case "blueprints":
		return dec.Decode(&data.Blueprints)
	case "scorecards":
		return dec.Decode(&data.Scorecards)
	case "actions":
		return dec.Decode(&data.Actions)
	case "teams":
		return dec.Decode(&data.Teams)
	case "users":
		return dec.Decode(&data.Users)
	case "automations":
		var items []api.Action
		if err := dec.Decode(&items); err != nil {
			return err
		}
		data.Actions = append(data.Actions, items...)
		return nil
	case "pages":
		return dec.Decode(&data.Pages)
	case "_folders":
		return dec.Decode(&data.Folders)
	case "integrations":
		return dec.Decode(&data.Integrations)
	case "BlueprintPermissions", "blueprint_permissions":
		return dec.Decode(&data.BlueprintPermissions)
	case "ActionPermissions", "action_permissions":
		return dec.Decode(&data.ActionPermissions)
	case "PagePermissions", "page_permissions":
		return dec.Decode(&data.PagePermissions)
	default:
		return skipJSONValue(dec)
	}
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			for dec.More() {
				if _, err := dec.Token(); err != nil {
					return err
				}
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		}
	}
	return nil
}
