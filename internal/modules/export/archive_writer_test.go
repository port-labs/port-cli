package export

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestArchiveWriter_StreamsEntitiesToJSONAndTar(t *testing.T) {
	tests := []struct {
		name   string
		format string
		file   string
		read   func(*testing.T, string) []api.Entity
	}{
		{
			name:   "json",
			format: "json",
			file:   "export.json",
			read: func(t *testing.T, path string) []api.Entity {
				t.Helper()
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read JSON export: %v", err)
				}
				var parsed struct {
					Blueprints []api.Blueprint `json:"blueprints"`
					Entities   []api.Entity    `json:"entities"`
				}
				if err := json.Unmarshal(content, &parsed); err != nil {
					t.Fatalf("decode JSON export: %v", err)
				}
				if len(parsed.Blueprints) != 1 {
					t.Fatalf("expected 1 blueprint, got %d", len(parsed.Blueprints))
				}
				return parsed.Entities
			},
		},
		{
			name:   "tar",
			format: "tar",
			file:   "export.tar.gz",
			read: func(t *testing.T, path string) []api.Entity {
				t.Helper()
				file, err := os.Open(path)
				if err != nil {
					t.Fatalf("open tar export: %v", err)
				}
				defer file.Close()
				gzr, err := gzip.NewReader(file)
				if err != nil {
					t.Fatalf("open gzip reader: %v", err)
				}
				defer gzr.Close()
				tr := tar.NewReader(gzr)
				for {
					header, err := tr.Next()
					if err == io.EOF {
						t.Fatal("entities.json was not written")
					}
					if err != nil {
						t.Fatalf("read tar entry: %v", err)
					}
					if header.Name != "entities.json" {
						continue
					}
					var entities []api.Entity
					if err := json.NewDecoder(tr).Decode(&entities); err != nil {
						t.Fatalf("decode entities tar entry: %v", err)
					}
					return entities
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), tt.file)
			writer, err := newArchiveWriter(tt.format, outputPath)
			if err != nil {
				t.Fatalf("newArchiveWriter error: %v", err)
			}
			if err := writer.WriteResource("blueprints", []api.Blueprint{{"identifier": "service"}}); err != nil {
				t.Fatalf("WriteResource error: %v", err)
			}
			if err := writer.WriteEntities(func(sink EntitySink) error {
				if err := sink.WriteEntity(api.Entity{"identifier": "svc-1", "blueprint": "service"}); err != nil {
					return err
				}
				return sink.WriteEntity(api.Entity{"identifier": "svc-2", "blueprint": "service"})
			}); err != nil {
				t.Fatalf("WriteEntities error: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close error: %v", err)
			}

			entities := tt.read(t, outputPath)
			if len(entities) != 2 {
				t.Fatalf("expected 2 entities, got %d", len(entities))
			}
			if entities[0]["identifier"] != "svc-1" || entities[1]["identifier"] != "svc-2" {
				t.Fatalf("unexpected entities: %v", entities)
			}
		})
	}
}
