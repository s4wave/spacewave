package bldr_plugin_compiler

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bldr/util/gocompiler"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/tools/imports"
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
	goVarDefs []*GoVarDef,
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

		// If the module exists within the source repository:
		modPathAbs := path.Dir(mod.GoMod)
		if !strings.HasPrefix(modPathAbs, analysis.workDir) {
			m.le.
				WithField("mod-path", mod.Path).
				Debug("skipping replacing out-of-tree module")
			continue
		}

		// Add a replace to the relative path of the containing repo.
		//
		// Ex: github.com/my/package => ../../
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
		goVarDefs,
	)
	if err != nil {
		return err
	}
	pluginCodeData, err := formatCodeFile(analysis.fset, gfile)
	if err != nil {
		return err
	}
	// remove any unused imports
	pluginCodeData, err = imports.Process(outPluginCodeFilePath, pluginCodeData, nil)
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
	ecmd := gocompiler.NewGoCompilerCmd("mod", "tidy")
	ecmd.Dir = m.pluginCodegenPath
	return gocompiler.ExecGoCompiler(m.le, ecmd)
}

// CompilePlugin compiles the plugin to outFile.
// The module structure should have been built already.
func (m *ModuleCompiler) CompilePlugin(outFile string) error {
	// go mod tidy
	if err := m.GoModTidy(); err != nil {
		return err
	}

	args := []string{
		"build",
		"-v", "-trimpath",
		"-buildvcs=false",
		"-o",
		outFile,
	}

	// build path: .
	args = append(args, ".")

	// go build
	ecmd := gocompiler.NewGoCompilerCmd(args...)

	ecmd.Dir = m.pluginCodegenPath
	return gocompiler.ExecGoCompiler(m.le, ecmd)
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

	// add build flags for the target plugin binary
	goArgs := gocompiler.GetDefaultArgs()
	goArgs = append(goArgs, "-gcflags", "-N -l")
	goEnv := gocompiler.GetDefaultEnv()
	goEnv = append(goEnv, "GOOS=", "GOARCH=")
	devWrapperSrc = fmt.Sprintf(
		"%s\nfunc init() {\n\tBuildFlags = %#v\n\tBuildEnv = %#v\n}\n",
		devWrapperSrc,
		goArgs,
		goEnv,
	)
	if err := os.WriteFile(devSrcMain, []byte(devWrapperSrc), 0644); err != nil {
		return err
	}

	// go mod tidy
	if err := m.GoModTidy(); err != nil {
		return err
	}

	// go build the wrapper
	args := []string{
		"build",
		"-o", outFile,
		"-v", "-trimpath",
		"-buildvcs=false",
	}

	if dlvAddr != "" {
		if err := ValidateDelveAddr(dlvAddr); err != nil {
			return errors.Wrap(err, "dlv_addr")
		}
		args = append(args, "-ldflags", "-X 'main.DelveAddr="+dlvAddr+"'")
	}

	// build path: .
	args = append(args, ".")

	ecmd := gocompiler.NewGoCompilerCmd(args...)
	ecmd.Env = append(ecmd.Env, "GOOS=", "GOARCH=") // host
	ecmd.Dir = devSrcDir
	return gocompiler.ExecGoCompiler(m.le, ecmd)
}

func formatCodeFile(fset *token.FileSet, pkgCodeFile *ast.File) ([]byte, error) {
	var outBytes bytes.Buffer
	var printerConf printer.Config
	printerConf.Mode |= printer.SourcePos
	err := printer.Fprint(&outBytes, fset, pkgCodeFile)
	return outBytes.Bytes(), err
}
