package config

import (
	"os"
	"path/filepath"
)

// EnvFileSource describes an env file Port CLI may load.
type EnvFileSource struct {
	Path   string
	Exists bool
}

// EnvFileSources returns the env files Port CLI checks, in load order.
func EnvFileSources() []EnvFileSource {
	sources := []EnvFileSource{{Path: ".env", Exists: fileExists(".env")}}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".port", ".env")
		sources = append(sources, EnvFileSource{Path: path, Exists: fileExists(path)})
	}
	return sources
}

// EnvFileLoadingDisabled reports whether env file loading is disabled.
func EnvFileLoadingDisabled() bool {
	return os.Getenv("TESTING") != "" || os.Getenv("PORT_NO_ENV_FILE") != ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
