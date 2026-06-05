package gocompiler

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	GoScriptCommandEnv = "BLDR_GOSCRIPT"
	goScriptModule     = "github.com/aperturerobotics/goscript/cmd/goscript@b5c5464b34668c8ae2112a1f3b19eab0407e9542"
	goScriptNoSumDB    = "github.com/aperturerobotics/goscript"
)

func GoScriptStartupCacheEnvKeys() []string {
	return []string{GoScriptCommandEnv}
}

// GoScriptCompileOptions configures one goscript compile invocation.
type GoScriptCompileOptions struct {
	WorkDir                   string
	OutputPath                string
	Packages                  []string
	BuildFlags                []string
	OverrideDirs              []string
	AllDependencies           bool
	ProtobufTypeScriptBinding bool
}

// GoListImportPath returns the import path for the package in workDir under the given build flags.
func GoListImportPath(ctx context.Context, workDir string, buildFlags []string) (string, error) {
	args := []string{"list"}
	for _, flag := range buildFlags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			return "", errors.New("go list build flag cannot be empty")
		}
		args = append(args, flag)
	}
	args = append(args, "-f", "{{.ImportPath}}", ".")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Env = append(os.Environ(), GetDefaultEnv()...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.Errorf("go list import path failed: %s", strings.TrimSpace(string(out)))
		}
		return "", err
	}
	importPath := strings.TrimSpace(string(out))
	if importPath == "" {
		return "", errors.New("go list import path returned empty path")
	}
	return importPath, nil
}

// ExecGoScriptCompile compiles Go packages to a GoScript TypeScript package tree.
func ExecGoScriptCompile(ctx context.Context, le *logrus.Entry, opts GoScriptCompileOptions) error {
	if strings.TrimSpace(opts.WorkDir) == "" {
		return errors.New("goscript work dir cannot be empty")
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return errors.New("goscript output path cannot be empty")
	}
	if len(opts.Packages) == 0 {
		return errors.New("goscript packages cannot be empty")
	}

	args := []string{
		"compile",
		"--dir", opts.WorkDir,
		"--output", opts.OutputPath,
	}
	for _, pkg := range opts.Packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			return errors.New("goscript package cannot be empty")
		}
		args = append(args, "--package", pkg)
	}
	for _, flag := range opts.BuildFlags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			return errors.New("goscript build flag cannot be empty")
		}
		args = append(args, "--build-flags", flag)
	}
	for _, dir := range opts.OverrideDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return errors.New("goscript override dir cannot be empty")
		}
		args = append(args, "--gs-path", dir)
	}
	if opts.AllDependencies {
		args = append(args, "--all-dependencies")
	}
	if opts.ProtobufTypeScriptBinding {
		args = append(args, "--protobuf-ts-binding")
	}

	ecmd := newGoScriptCmd(ctx, args...)
	ecmd.Dir = opts.WorkDir

	timeStart := time.Now()
	if err := ExecGoCompiler(le, ecmd); err != nil {
		return err
	}
	le.
		WithField("compiler", "goscript").
		WithField("dur", time.Since(timeStart).String()).
		Info("compiled plugin TypeScript package tree")
	return nil
}

func newGoScriptCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmdName := os.Getenv(GoScriptCommandEnv)
	if strings.TrimSpace(cmdName) != "" {
		return NewGoCompilerCmd(ctx, cmdName, args...)
	}

	goArgs := append([]string{"run", goScriptModule}, args...)
	ecmd := NewGoCompilerCmd(ctx, "go", goArgs...)
	ecmd.Env = append(ecmd.Env, "GONOSUMDB="+goScriptNoSumDB)
	return ecmd
}
