package plugin_compiler

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"mvdan.cc/gofumpt/format"
)

// ModuleCompiler assembles a series of Go module files on disk to orchestrate
// "go build" commands and produce a plugin with unique import paths for the
// changed packages.
type ModuleCompiler struct {
	ctx context.Context
	le  *logrus.Entry

	pluginCodegenPath string
	pluginGoModule    string
}

// NewModuleCompiler constructs a new module compiler with paths.
//
// packagesLookupPath is the working directory for "go build."
func NewModuleCompiler(
	ctx context.Context,
	le *logrus.Entry,
	pluginCodegenPath string,
	pluginGoModule string,
) (*ModuleCompiler, error) {
	if pluginCodegenPath == "" {
		return nil, errors.New("codegen path cannot be empty")
	}
	pluginCodegenPath, err := filepath.Abs(pluginCodegenPath)
	if err != nil {
		return nil, err
	}
	return &ModuleCompiler{
		ctx: ctx,
		le:  le,

		pluginCodegenPath: pluginCodegenPath,
		pluginGoModule:    pluginGoModule,
	}, nil
}

// GenerateModule builds the module files in the codegen path.
//
// buildPrefix should be something like cbus-plugin-abcdef (no slash)
func (m *ModuleCompiler) GenerateModule(analysis *Analysis) error {
	// sanity checks
	if os.PathSeparator != '/' {
		// this is sort of hacky but we expect to generally use this on linux.
		return errors.New("can only work on systems where / is the path separator")
	}
	if _, err := os.Stat(m.pluginCodegenPath); err != nil {
		return err
	}

	loadedModules := analysis.GetImportedModules()
	if len(loadedModules) == 0 {
		return errors.New("must load at least one module")
	}

	codegenModuleDir := m.pluginCodegenPath
	codegenModulesPluginPath := codegenModuleDir
	if err := os.MkdirAll(codegenModulesPluginPath, 0755); err != nil {
		return err
	}

	// Create the output code plugin go.mod.
	outPluginModDir := codegenModulesPluginPath
	outPluginModFilePath := path.Join(outPluginModDir, "go.mod")
	outPluginCodeFilePath := path.Join(codegenModulesPluginPath, "plugin.go")

	// outPluginGoMod will contain the go.mod for the container plugin.
	// baseModule is used to inherit replace directives in go.mod
	outPluginGoMod := analysis.GetBaseModFile()
	// basePluginPath := outPluginGoMod.Module.Mod.Path

	// Relocate the go.mod references to the new go.mod path.
	if err := relocateGoModFile(outPluginGoMod, outPluginModFilePath); err != nil {
		return err
	}
	if err := outPluginGoMod.AddModuleStmt(m.pluginGoModule); err != nil {
		return err
	}

	// Also add the replacement to the final plugin go.mod.
	/*
		err = outPluginGoMod.AddReplace(srcMod.Path, "", modPathAbs, "")
		if err != nil {
			return err
		}
	*/

	for _, mod := range loadedModules {
		srcMod := mod
		for mod.Replace != nil {
			m.le.
				WithField("mod-curr-path", mod.Path).
				WithField("mod-next-path", mod.Replace.Path).
				Debug("module was replaced with another")
			mod = mod.Replace
		}

		// Add a replace to the relative path of the containing repo.
		//
		// Ex: github.com/my/package => ../../
		modPathAbs := path.Dir(mod.GoMod)
		modPathRel, err := filepath.Rel(outPluginModDir, modPathAbs)
		if err != nil {
			return err
		}

		err = outPluginGoMod.AddReplace(srcMod.Path, "", modPathRel, "")
		if err != nil {
			return err
		}
		outPluginGoMod.Cleanup()
	}

	outPluginGoMod.SortBlocks()
	outPluginGoMod.Cleanup()
	destGoMod, err := outPluginGoMod.Format()
	if err != nil {
		return err
	}
	if err := os.WriteFile(
		outPluginModFilePath,
		destGoMod,
		0644,
	); err != nil {
		return err
	}

	rewritePackagesImports := func(pkgCodeFile *ast.File) {
		for _, pkgCodeImport := range pkgCodeFile.Imports {
			pkgCodeImportPath := pkgCodeImport.Path.Value
			if len(pkgCodeImportPath) < 2 {
				continue
			}
			pkgCodeImportPath = pkgCodeImportPath[1:]
			pkgCodeImportPath = pkgCodeImportPath[:len(pkgCodeImportPath)-1]
			targetPkg, ok := analysis.packages[pkgCodeImportPath]
			if !ok {
				continue
			}
			replacedTargetPath := targetPkg.Types.Path()
			pkgCodeImport.Path.Value = fmt.Sprintf("%q", replacedTargetPath)
		}
	}

	pluginGoMod, err := outPluginGoMod.Format()
	if err != nil {
		return err
	}
	err = os.WriteFile(outPluginModFilePath, pluginGoMod, 0644)
	if err != nil {
		return err
	}

	// Build the actual plugin file itself.
	gfile, err := CodegenPluginWrapperFromAnalysis(
		m.le,
		analysis,
	)
	if err != nil {
		return err
	}
	// Format to output pass #1
	pluginCodeData, err := formatCodeFile(analysis.fset, gfile)
	if err != nil {
		return err
	}
	// we have to write it and then adjust paths, to populate fields in ast code.
	gfile, err = parser.ParseFile(
		analysis.fset,
		outPluginCodeFilePath,
		pluginCodeData,
		parser.ParseComments|parser.AllErrors,
	)
	if err != nil {
		return err
	}
	// Adjust the import paths.
	rewritePackagesImports(gfile)
	// Format to output pass #2
	pluginCodeData, err = formatCodeFile(analysis.fset, gfile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPluginCodeFilePath, pluginCodeData, 0644); err != nil {
		return err
	}

	return nil
}

// CompilePlugin compiles the plugin once.
// The module structure should have been built already.
func (m *ModuleCompiler) CompilePlugin(outFile string) error {
	le := m.le
	codegenModuleDir, err := filepath.Abs(m.pluginCodegenPath)
	if err != nil {
		return err
	}
	// pluginBinDir := filepath.Join(codegenModuleDir, "bin")

	// build the intermediate output dir
	tmpName, err := os.MkdirTemp("", "controllerbus-hot-compiler-tmpdir")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpName)

	ecmd := exec.ExecGoTidyModules()
	ecmd.Dir = codegenModuleDir
	le.
		WithField("work-dir", ecmd.Dir).
		Debugf("running go mod tidy: %s", ecmd.String())
	if err := ecmd.Run(); err != nil {
		return err
	}

	// start the go compiler execution
	var stderrBuf bytes.Buffer
	ecmd = exec.ExecGoCompiler(
		"build", "-v", "-trimpath",
		"-buildvcs=false",
		"-o",
		outFile,
		".",
	)
	ecmd.Dir = codegenModuleDir
	ecmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	le.
		WithField("work-dir", ecmd.Dir).
		Debugf("running go compiler: %s", ecmd.String())
	err = ecmd.Run()
	if err != nil && strings.HasPrefix(err.Error(), "exit status") {
		stderrLines := strings.Split(stderrBuf.String(), "\n")
		errMsg := stderrLines[len(stderrLines)-1]
		if len(errMsg) == 0 && len(stderrLines) > 1 {
			errMsg = stderrLines[len(stderrLines)-2]
		}
		err = errors.New(errMsg)
	}
	return err
}

func formatCodeFile(fset *token.FileSet, pkgCodeFile *ast.File) ([]byte, error) {
	format.File(fset, pkgCodeFile, format.Options{LangVersion: "1.14"})
	var outBytes bytes.Buffer
	var printerConf printer.Config
	printerConf.Mode |= printer.SourcePos
	err := printer.Fprint(&outBytes, fset, pkgCodeFile)
	return outBytes.Bytes(), err
}
