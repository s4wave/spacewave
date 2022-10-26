package plugin_compiler

import (
	"bytes"
	"context"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

// NewModuleCompiler constructs a new module compiler.
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
// if configSetBinary is set and len() != 0, will be embedded as a config set.
func (m *ModuleCompiler) GenerateModule(
	analysis *Analysis,
	configSetBinary []byte,
) error {
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

	// Create the output code plugin go.mod.
	outPluginModFilePath := path.Join(m.pluginCodegenPath, "go.mod")
	outPluginCodeFilePath := path.Join(m.pluginCodegenPath, "plugin.go")

	// Create the embedded config set file, if necessary.
	var configSetBinFiles []string
	if len(configSetBinary) != 0 {
		configSetBinFilename := "config-set.bin"
		outConfigSetBinPath := path.Join(m.pluginCodegenPath, configSetBinFilename)
		if err := os.WriteFile(outConfigSetBinPath, configSetBinary, 0644); err != nil {
			return err
		}
		configSetBinFiles = append(configSetBinFiles, configSetBinFilename)
	}

	// outPluginGoMod will contain the go.mod for the container plugin.
	// baseModule is used to inherit replace directives in go.mod
	// Relocate the go.mod references to the new go.mod path.
	outPluginGoMod := analysis.GetBaseModFile()
	if err := relocateGoModFile(outPluginGoMod, outPluginModFilePath); err != nil {
		return err
	}
	if err := outPluginGoMod.AddModuleStmt(m.pluginGoModule); err != nil {
		return err
	}

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
		modPathRel, err := filepath.Rel(m.pluginCodegenPath, modPathAbs)
		if err != nil {
			return err
		}

		err = outPluginGoMod.AddReplace(srcMod.Path, "", modPathRel, "")
		if err != nil {
			return err
		}
	}

	// cleanup go mod file
	outPluginGoMod.SortBlocks()
	outPluginGoMod.Cleanup()

	// format & write go mod file
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

	pluginGoMod, err := outPluginGoMod.Format()
	if err != nil {
		return err
	}
	err = os.WriteFile(outPluginModFilePath, pluginGoMod, 0644)
	if err != nil {
		return err
	}

	// Build the plugin main() code file.
	gfile, err := CodegenPluginWrapperFromAnalysis(
		m.le,
		analysis,
		configSetBinFiles,
	)
	if err != nil {
		return err
	}
	pluginCodeData, err := formatCodeFile(analysis.fset, gfile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPluginCodeFilePath, pluginCodeData, 0644); err != nil {
		return err
	}

	return nil
}

// GoModTidy runs go mod tidy on the plugin.
// The module structure should have been built already.
func (m *ModuleCompiler) GoModTidy() error {
	// go mod tidy
	ecmd := NewGoCompilerCmd("mod", "tidy")
	ecmd.Dir = m.pluginCodegenPath
	return ExecGoCompiler(m.le, ecmd)
}

// CompilePlugin compiles the plugin to outFile.
// The module structure should have been built already.
func (m *ModuleCompiler) CompilePlugin(outFile string) error {
	// go mod tidy
	if err := m.GoModTidy(); err != nil {
		return err
	}

	// go build
	ecmd := NewGoCompilerCmd(
		"build", "-v", "-trimpath",
		"-buildvcs=false",
		"-o",
		outFile,
		".",
	)
	ecmd.Dir = m.pluginCodegenPath
	return ExecGoCompiler(m.le, ecmd)
}

// CompilePluginDevWrapper compiles a development wrapper for the plugin.
// The module structure should have been built already.
// If buildDevWrapper is set, build an entrypoint that runs the plugin.
// If buildDevWrapper is set, assumes paths: .bldr/build/myplugin/ and .bldr/dist/myplugin/
func (m *ModuleCompiler) CompilePluginDevWrapper(outFile, dlvAddr string) error {
	// write the plugin dev wrapper entrypoint
	devSrcDir := path.Join(m.pluginCodegenPath, "dev")
	devSrcMain := path.Join(devSrcDir, "main.go")
	if err := os.MkdirAll(devSrcDir, 0755); err != nil {
		return err
	}
	devWrapperSrc, err := GetDevWrapper()
	if err != nil {
		return err
	}
	if err := os.WriteFile(devSrcMain, []byte(devWrapperSrc), 0644); err != nil {
		return err
	}

	// go mod tidy
	if err := m.GoModTidy(); err != nil {
		return err
	}

	// go build
	compilerArgs := []string{
		"build",
		"-v", "-trimpath",
		"-buildvcs=false",
	}
	if dlvAddr != "" {
		if err := ValidateDelveAddr(dlvAddr); err != nil {
			return errors.Wrap(err, "dlv_addr")
		}
		compilerArgs = append(compilerArgs, "-ldflags", "-X 'main.DelveAddr="+dlvAddr+"'")
	}

	compilerArgs = append(compilerArgs, "-o", outFile)
	compilerArgs = append(compilerArgs, ".")

	ecmd := NewGoCompilerCmd(compilerArgs...)
	ecmd.Dir = devSrcDir
	return ExecGoCompiler(m.le, ecmd)
}

func formatCodeFile(fset *token.FileSet, pkgCodeFile *ast.File) ([]byte, error) {
	var outBytes bytes.Buffer
	var printerConf printer.Config
	printerConf.Mode |= printer.SourcePos
	err := printer.Fprint(&outBytes, fset, pkgCodeFile)
	return outBytes.Bytes(), err
}
