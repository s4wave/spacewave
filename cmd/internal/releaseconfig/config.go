// Package releaseconfig applies the release-owned public settings to native producers.
package releaseconfig

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// Prepare combines the source build definitions with the staging release export.
// Production retains bldr.yaml. Callers pass the returned path to every builder,
// including manifest-pack production, so the handoff label and runtime agree.
func Prepare(repoDir, environment string) (string, error) {
	// Resolve the producer environment before selecting any runtime configuration.
	ambient := os.Getenv("SPACEWAVE_RELEASE_ENV")
	if ambient == "production" {
		ambient = "prod"
	}
	if environment == "production" {
		environment = "prod"
	}
	if environment != "" && ambient != "" && environment != ambient {
		return "", errors.Errorf("release environment %q conflicts with SPACEWAVE_RELEASE_ENV=%q", environment, ambient)
	}
	if environment == "" {
		environment = ambient
	}
	switch environment {
	case "", "prod":
		return "bldr.yaml", nil
	case "staging":
	default:
		return "", errors.Errorf("unknown release environment %q", environment)
	}

	// The private release repository generates and verifies this public export.
	root, err := os.OpenRoot(repoDir)
	if err != nil {
		return "", err
	}
	defer root.Close()

	// Read the base bldr.star build definition and the staging overlay.
	source, err := root.ReadFile("bldr.star")
	if err != nil {
		return "", err
	}
	overrides, err := root.ReadFile("release-staging.star")
	if err != nil {
		return "", errors.Wrap(err, "read staging release overlay")
	}

	// Write the combined bldr.star and a staging bldr.yaml into the export directory.
	dir := filepath.Join(".tmp", "native-release-config", "staging")
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	content := append(append(source, '\n'), overrides...)
	if err := root.WriteFile(filepath.Join(dir, "bldr.star"), content, 0o644); err != nil {
		return "", err
	}

	// Write the staging bldr.yaml manifest and return its slash path.
	path := filepath.Join(dir, "bldr.yaml")
	if err := root.WriteFile(path, []byte("id: spacewave\n"), 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}
