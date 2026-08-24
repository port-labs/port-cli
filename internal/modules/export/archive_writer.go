package export

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/port-labs/port-cli/internal/api"
)

// EntitySink accepts entities one at a time while an export archive is being
// written.
type EntitySink interface {
	WriteEntity(api.Entity) error
}

// ArchiveWriter writes export resources section by section.
type ArchiveWriter interface {
	WriteResource(name string, value interface{}) error
	WriteEntities(func(EntitySink) error) error
	Close() error
}

type jsonArchiveWriter struct {
	file      *os.File
	encoder   *json.Encoder
	wroteAny  bool
	closeOnce bool
}

func newJSONArchiveWriter(outputPath string) (*jsonArchiveWriter, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	w := &jsonArchiveWriter{
		file:    file,
		encoder: json.NewEncoder(file),
	}
	w.encoder.SetIndent("", "  ")
	if _, err := io.WriteString(file, "{\n"); err != nil {
		file.Close()
		return nil, err
	}
	return w, nil
}

func (w *jsonArchiveWriter) writeFieldPrefix(name string) error {
	if w.wroteAny {
		if _, err := io.WriteString(w.file, ",\n"); err != nil {
			return err
		}
	}
	w.wroteAny = true
	_, err := fmt.Fprintf(w.file, "  %q: ", name)
	return err
}

func (w *jsonArchiveWriter) WriteResource(name string, value interface{}) error {
	if err := w.writeFieldPrefix(name); err != nil {
		return err
	}
	return w.encoder.Encode(value)
}

func (w *jsonArchiveWriter) WriteEntities(write func(EntitySink) error) error {
	if err := w.writeFieldPrefix("entities"); err != nil {
		return err
	}
	if _, err := io.WriteString(w.file, "[\n"); err != nil {
		return err
	}
	sink := &jsonEntityArraySink{w: w.file, encoder: json.NewEncoder(w.file)}
	sink.encoder.SetIndent("    ", "  ")
	if err := write(sink); err != nil {
		return err
	}
	if sink.count > 0 {
		if _, err := io.WriteString(w.file, "\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w.file, "  ]\n")
	return err
}

func (w *jsonArchiveWriter) Close() error {
	if w.closeOnce {
		return nil
	}
	w.closeOnce = true
	if _, err := io.WriteString(w.file, "}\n"); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

type jsonEntityArraySink struct {
	w       io.Writer
	encoder *json.Encoder
	count   int
}

func (s *jsonEntityArraySink) WriteEntity(entity api.Entity) error {
	if s.count > 0 {
		if _, err := io.WriteString(s.w, ",\n"); err != nil {
			return err
		}
	}
	s.count++
	return s.encoder.Encode(entity)
}

type tarArchiveWriter struct {
	file    *os.File
	gzw     *gzip.Writer
	tw      *tar.Writer
	tempDir string
	closed  bool
}

func newTarArchiveWriter(outputPath string) (*tarArchiveWriter, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	tempDir, err := os.MkdirTemp("", "port-cli-export-*")
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create export temp directory: %w", err)
	}
	gzw := gzip.NewWriter(file)
	return &tarArchiveWriter{
		file:    file,
		gzw:     gzw,
		tw:      tar.NewWriter(gzw),
		tempDir: tempDir,
	}, nil
}

func (w *tarArchiveWriter) WriteResource(name string, value interface{}) error {
	return w.spoolAndWrite(name, func(tmp *os.File) error {
		encoder := json.NewEncoder(tmp)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	})
}

func (w *tarArchiveWriter) WriteEntities(write func(EntitySink) error) error {
	return w.spoolAndWrite("entities", func(tmp *os.File) error {
		if _, err := io.WriteString(tmp, "[\n"); err != nil {
			return err
		}
		sink := &jsonEntityArraySink{w: tmp, encoder: json.NewEncoder(tmp)}
		sink.encoder.SetIndent("  ", "  ")
		if err := write(sink); err != nil {
			return err
		}
		if sink.count > 0 {
			if _, err := io.WriteString(tmp, "\n"); err != nil {
				return err
			}
		}
		_, err := io.WriteString(tmp, "]\n")
		return err
	})
}

func (w *tarArchiveWriter) spoolAndWrite(name string, write func(*os.File) error) error {
	tmp, err := os.CreateTemp(w.tempDir, name+"-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp export entry for %s: %w", name, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := write(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name: fmt.Sprintf("%s.json", name),
		Size: info.Size(),
		Mode: 0o644,
	}
	if err := w.tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for %s: %w", name, err)
	}
	in, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := io.Copy(w.tw, in); err != nil {
		return fmt.Errorf("failed to write %s to tar: %w", name, err)
	}
	return nil
}

func (w *tarArchiveWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	var err error
	if closeErr := w.tw.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if closeErr := w.gzw.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if closeErr := w.file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if removeErr := os.RemoveAll(w.tempDir); removeErr != nil && err == nil {
		err = removeErr
	}
	return err
}

func newArchiveWriter(formatType, outputPath string) (ArchiveWriter, error) {
	if formatType == "tar" {
		return newTarArchiveWriter(outputPath)
	}
	return newJSONArchiveWriter(outputPath)
}
