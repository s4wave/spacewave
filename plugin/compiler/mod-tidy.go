package plugin_compiler

import (
	"context"
	"os"
	"path"

	uexec "github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// MaybeRunGoModTidy conditionally runs go mod tidy if the go.mod has any
// unresolved non-canonical versions.
func MaybeRunGoModTidy(ctx context.Context, le *logrus.Entry, workDir string) error {
	baseGoModPath := path.Join(workDir, "go.mod")
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
	if anyNeedFixed {
		err = nil
	} else if err != nil {
		return err
	}

	cmd := uexec.ExecGoTidyModules()
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	return ExecGoCompiler(le, cmd)
}
