package output

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/port-labs/port-cli/internal/modules/export"
)

// WriteTar writes data to a tar.gz file.
func WriteTar(data *export.Data, outputPath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Write each data type to separate JSON files in the tar
	dataTypes := map[string]interface{}{
		"blueprints":   data.Blueprints,
		"entities":     data.Entities,
		"scorecards":   data.Scorecards,
		"actions":      data.Actions,
		"teams":        data.Teams,
		"_folders":     data.Folders,
		"pages":        data.Pages,
		"integrations": data.Integrations,
	}

	for dataType, items := range dataTypes {
		jsonData, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal %s: %w", dataType, err)
		}

		// Create tar header
		header := &tar.Header{
			Name: fmt.Sprintf("%s.json", dataType),
			Size: int64(len(jsonData)),
			Mode: 0o644,
		}

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", dataType, err)
		}

		if _, err := tw.Write(jsonData); err != nil {
			return fmt.Errorf("failed to write %s to tar: %w", dataType, err)
		}
	}

	return nil
}

// WriteJSON writes data to a JSON file.
func WriteJSON(data *export.Data, outputPath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	output := map[string]interface{}{
		"blueprints":   data.Blueprints,
		"entities":     data.Entities,
		"scorecards":   data.Scorecards,
		"actions":      data.Actions,
		"teams":        data.Teams,
		"_folders":     data.Folders,
		"pages":        data.Pages,
		"integrations": data.Integrations,
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
