// Package config resolves where Becoute keeps its local data.
package config

import (
	"os"
	"path/filepath"
)

// DataDir returns the directory Becoute stores its library and progress in.
// Set BECOUTE_DATA_DIR to override it (useful for tests and portable installs).
func DataDir() (string, error) {
	if dir := os.Getenv("BECOUTE_DATA_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Becoute"), nil
}

// LibraryPath is the JSON file holding the book library.
func LibraryPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "library.json"), nil
}

// ProgressPath is the JSON file holding listening progress.
func ProgressPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "progress.json"), nil
}
