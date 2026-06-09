package gocompiler

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	goscript_compiler "github.com/aperturerobotics/goscript/compiler"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func GoScriptStartupCacheEnvKeys() []string {
	return nil
}

// GoScriptCompileOptions configures one goscript compile invocation.
type GoScriptCompileOptions struct {
	WorkDir                   string
	OutputPath                string
	Packages                  []string
	BuildFlags                []string
	Env                       []string
	OverrideDirs              []string
	AllDependencies           bool
	ProtobufTypeScriptBinding bool
}

// GoListImportPath returns the import path for the package in workDir under the given build flags.
func GoListImportPath(ctx context.Context, workDir string, buildFlags []string, env ...string) (string, error) {
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
	cmd.Env = append(cmd.Env, env...)
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
	for _, env := range opts.Env {
		if strings.TrimSpace(env) == "" {
			return errors.New("goscript env cannot be empty")
		}
	}

	conf := &goscript_compiler.Config{
		Dir:                       opts.WorkDir,
		OutputPath:                opts.OutputPath,
		AllDependencies:           opts.AllDependencies,
		ProtobufTypeScriptBinding: opts.ProtobufTypeScriptBinding,
	}
	packages := make([]string, 0, len(opts.Packages))
	for _, pkg := range opts.Packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			return errors.New("goscript package cannot be empty")
		}
		packages = append(packages, pkg)
	}
	for _, flag := range opts.BuildFlags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			return errors.New("goscript build flag cannot be empty")
		}
		conf.BuildFlags = append(conf.BuildFlags, flag)
	}
	for _, dir := range opts.OverrideDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return errors.New("goscript override dir cannot be empty")
		}
		conf.OverrideDirs = append(conf.OverrideDirs, dir)
	}

	timeStart := time.Now()
	comp, err := goscript_compiler.NewCompiler(conf, le, nil)
	if err != nil {
		return err
	}
	if _, err := comp.CompilePackages(ctx, packages...); err != nil {
		return err
	}
	le.
		WithField("compiler", "goscript").
		WithField("dur", time.Since(timeStart).String()).
		Info("compiled plugin TypeScript package tree")
	return nil
}
