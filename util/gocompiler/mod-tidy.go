package gocompiler

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// RunGoModTidy runs go mod tidy unconditionally.
func RunGoModTidy(ctx context.Context, le *logrus.Entry, workDir string) error {
	cmd := NewGoCompilerCmd("mod", "tidy")
	cmd.Dir = workDir
	return ExecGoCompiler(le, cmd)
}

// MaybeRunGoModTidy conditionally runs go mod tidy if the go.mod has any
// unresolved non-canonical versions.
func MaybeRunGoModTidy(ctx context.Context, le *logrus.Entry, workDir string) error {
	baseGoModPath := filepath.Join(workDir, "go.mod")
	baseGoModData, err := os.ReadFile(baseGoModPath)
	if err != nil {
		return err
	}
	var anyNeedFixed bool
	_, err = modfile.Parse(
		baseGoModPath,
		baseGoModData,
		func(path, version string) (string, error) {
			if module.CanonicalVersion(version) != "" {
				return version, nil
			}
			anyNeedFixed = true
			return "v0.5.1+incompatible", nil
		},
	)
	if !anyNeedFixed && err != nil {
		return err
	}

	cmd := NewGoCompilerCmd("mod", "tidy")
	cmd.Dir = workDir
	return ExecGoCompiler(le, cmd)
}
