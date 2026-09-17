package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/changelog"
)

func main() {
	if err := run(); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run() error {
	rootDir, err := repoDir(os.Args[1:])
	if err != nil {
		return err
	}

	orgPath := filepath.Join(rootDir, "CHANGELOG.org")
	// #nosec G703 -- rootDir is the operator-provided repository directory.
	orgData, err := os.ReadFile(orgPath)
	if err != nil {
		return errors.Wrap(err, "read CHANGELOG.org")
	}

	cl, err := changelog.ParseOrgChangelog(orgData)
	if err != nil {
		return errors.Wrap(err, "parse CHANGELOG.org")
	}

	binData, err := cl.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal changelog binary")
	}
	// #nosec G703 -- rootDir is the operator-provided repository directory.
	if err := os.WriteFile(
		filepath.Join(rootDir, "core", "changelog", "changelog.bin"),
		binData,
		0o644,
	); err != nil {
		return errors.Wrap(err, "write changelog.bin")
	}

	return nil
}

func repoDir(args []string) (string, error) {
	switch len(args) {
	case 0:
		return os.Getwd()
	case 2:
		if args[0] != "--repo" {
			return "", errors.New("usage: changelog-gen [--repo /path/to/spacewave]")
		}
		return filepath.Clean(args[1]), nil // #nosec G602 -- len(args)==2 is checked by this case.
	default:
		return "", errors.New("usage: changelog-gen [--repo /path/to/spacewave]")
	}
}
