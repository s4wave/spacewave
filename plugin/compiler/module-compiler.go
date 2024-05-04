package bldr_plugin_compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
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
// if configSetBinary is set and len() != 0, will be embedded as a config set.
//
// devInfoFile will be loaded at runtime and used to populate variables init().
// if devInfoFile is empty, the values of the go variable defs are hardcoded into init().
// if devInfoFile is set, the file will be written at that path.
func (m *ModuleCompiler) GenerateModule(
	analysis *Analysis,
	pluginMeta *bldr_plugin.PluginMeta,
	configSetBinary []byte,
	goVarDefs []*vardef.PluginVar,
	devInfoFile string,
) (*vardef.PluginDevInfo, error) {
	if _, err := os.Stat(m.pluginCodegenPath); err != nil {
		return nil, err
	}

	loadedModules := analysis.GetImportedModules()
	if len(loadedModules) == 0 {
		return nil, errors.New("must load at least one module")
	}

	// Create the embedded config set file, if necessary.
	var configSetBinFiles []string
	if len(configSetBinary) != 0 {
		configSetBinFilename := "config-set.bin"
		outConfigSetBinPath := filepath.Join(m.pluginCodegenPath, configSetBinFilename)
		if err := os.WriteFile(outConfigSetBinPath, configSetBinary, 0o644); err != nil {
			return nil, err
		}
		configSetBinFiles = append(configSetBinFiles, configSetBinFilename)
	}

	// Create the dev info file if necessary.
	pluginDevInfo := &vardef.PluginDevInfo{PluginVars: goVarDefs}
	if len(devInfoFile) != 0 && len(goVarDefs) != 0 {
		outDevInfoFilePath := filepath.Join(m.pluginCodegenPath, devInfoFile)
		devInfoBin, err := (pluginDevInfo).MarshalVT()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(outDevInfoFilePath, devInfoBin, 0o644); err != nil {
			return nil, err
		}
	}

	// Build the plugin main() code file.
	gfile, err := CodegenPluginWrapperFromAnalysis(
		m.le,
		analysis,
		pluginMeta,
		configSetBinFiles,
		goVarDefs,
		devInfoFile,
	)
	if err != nil {
		return nil, err
	}
	pluginCodeData, err := gocompiler.FormatCodeFile(analysis.fset, gfile)
	if err != nil {
		return nil, err
	}
	// remove any unused imports
	outPluginCodeFilePath := filepath.Join(m.pluginCodegenPath, "plugin.go")
	pluginCodeData, err = imports.Process(outPluginCodeFilePath, pluginCodeData, nil)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(outPluginCodeFilePath, pluginCodeData, 0o644); err != nil {
		return nil, err
	}

	return pluginDevInfo, nil
}

// CompilePlugin compiles the plugin to outFile.
// The module structure should have been built already.
func (m *ModuleCompiler) CompilePlugin(
	ctx context.Context,
	le *logrus.Entry,
	outFile string,
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	enableCgo bool,
	useTinygo bool,
) error {
	workDir := m.pluginCodegenPath
	return gocompiler.ExecBuildEntrypoint(
		le,
		buildPlatform,
		buildType,
		workDir,
		outFile,
		enableCgo,
		useTinygo,
		nil,
		nil,
	)
}

// CompilePluginDevWrapper compiles a development wrapper for the plugin.
// The module structure should have been built already.
// If buildDevWrapper is set, build an entrypoint that runs the plugin.
// If buildDevWrapper is set, assumes paths: .bldr/build/myplugin/ and .bldr/dist/myplugin/
// NOTE: This wrapper is intended to be run on the build machine in native mode.
func (m *ModuleCompiler) CompilePluginDevWrapper(
	ctx context.Context,
	le *logrus.Entry,
	outFile,
	dlvAddr string,
	buildPlatform bldr_platform.Platform,
	buildType bldr_manifest.BuildType,
	enableCgo bool,
) error {
	// write the plugin dev wrapper entrypoint
	devSrcDir := filepath.Join(m.pluginCodegenPath, "dev")
	devSrcMain := filepath.Join(devSrcDir, "main.go")
	if err := os.MkdirAll(devSrcDir, 0o755); err != nil {
		return err
	}
	devWrapperSrc, err := GetDevWrapper()
	if err != nil {
		return err
	}

	// add build flags for the target plugin binary
	goArgs := gocompiler.GetDefaultArgs()

	// build tags
	buildTags := gocompiler.NewBuildTags(buildType, enableCgo)

	// add build tags to build args
	if len(buildTags) != 0 {
		goArgs = append(goArgs, "-tags="+strings.Join(buildTags, ","))
	}

	// note: no -trimpath here
	// disables inlining and optimizations for debugging purposes
	goArgs = append(goArgs, "-gcflags", "-N -l")

	goEnv := gocompiler.GetDefaultEnv()
	goEnv = append(goEnv, "GOOS=", "GOARCH=")
	if enableCgo {
		goEnv = append(goEnv, "CGO_ENABLED=1")
	} else {
		goEnv = append(goEnv, "CGO_ENABLED=0")
	}

	devWrapperSrc = fmt.Sprintf(
		"%s\nfunc init() {\n\tBuildFlags = %#v\n\tBuildEnv = %#v\n}\n",
		devWrapperSrc,
		goArgs,
		goEnv,
	)
	if err := os.WriteFile(devSrcMain, []byte(devWrapperSrc), 0o644); err != nil {
		return err
	}

	// go build the wrapper
	args := append([]string{"build", "-trimpath", "-o", outFile}, gocompiler.GetDefaultArgs()...)

	if dlvAddr != "" {
		if err := ValidateDelveAddr(dlvAddr); err != nil {
			return errors.Wrap(err, "dlv_addr")
		}
		args = append(args, "-ldflags", "-X 'main.DelveAddr="+dlvAddr+"'")
	}

	// build path: .
	args = append(args, ".")

	ecmd := gocompiler.NewGoCompilerCmd("go", args...)
	ecmd.Env = append(ecmd.Env, "GOOS=", "GOARCH=") // host, ignore cgo-enabled
	ecmd.Dir = devSrcDir
	return gocompiler.ExecGoCompiler(m.le, ecmd)
}
