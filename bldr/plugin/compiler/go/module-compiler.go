//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	vardef "github.com/s4wave/spacewave/bldr/plugin/vardef"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/imports"
)

// ModuleCompiler assembles a series of Go module files on disk to orchestrate
// "go build" commands and produce a plugin with unique import paths for the
// changed packages.
type ModuleCompiler struct {
	le *logrus.Entry

	pluginCodegenPath string
	pluginGoModule    string
}

// NewModuleCompiler constructs a new module compiler.
func NewModuleCompiler(
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
		le: le,

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
	ctx context.Context,
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
	if err := m.writeModuleFiles(analysis); err != nil {
		return nil, err
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
	if err := gocompiler.RunGoModTidy(ctx, m.le, m.pluginCodegenPath); err != nil {
		return nil, err
	}

	return pluginDevInfo, nil
}

func (m *ModuleCompiler) writeModuleFiles(analysis *Analysis) error {
	sourceGoModPath := filepath.Join(analysis.workDir, "go.mod")
	sourceGoModData, err := os.ReadFile(sourceGoModPath)
	if err != nil {
		return errors.Wrapf(err, "read source go.mod at %s", sourceGoModPath)
	}
	modFile, err := modfile.Parse(sourceGoModPath, sourceGoModData, nil)
	if err != nil {
		return err
	}
	if err := absolutizeModuleReplaces(modFile, analysis.workDir); err != nil {
		return err
	}
	sourceModulePath := modFile.Module.Mod.Path
	pluginModulePath, err := generatedPluginModulePath(m.pluginGoModule)
	if err != nil {
		return err
	}
	if err := modFile.AddModuleStmt(pluginModulePath); err != nil {
		return err
	}
	if err := modFile.AddRequire(sourceModulePath, "v0.0.0"); err != nil {
		return err
	}
	sourceModuleDir, err := filepath.Abs(analysis.workDir)
	if err != nil {
		return err
	}
	if err := modFile.AddReplace(sourceModulePath, "", sourceModuleDir, ""); err != nil {
		return err
	}
	modFile.Cleanup()
	pluginGoModData, err := modFile.Format()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(m.pluginCodegenPath, "go.mod"), pluginGoModData, 0o644); err != nil {
		return err
	}

	sourceGoSumPath := filepath.Join(analysis.workDir, "go.sum")
	sourceGoSumData, err := os.ReadFile(sourceGoSumPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "read source go.sum at %s", sourceGoSumPath)
	}
	return os.WriteFile(filepath.Join(m.pluginCodegenPath, "go.sum"), sourceGoSumData, 0o644)
}

func absolutizeModuleReplaces(modFile *modfile.File, sourceDir string) error {
	absSourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return err
	}
	for _, replace := range modFile.Replace {
		if replace.New.Version != "" {
			continue
		}
		replacePath := replace.New.Path
		if filepath.IsAbs(replacePath) {
			continue
		}
		if !strings.HasPrefix(replacePath, ".") {
			continue
		}
		absReplacePath := filepath.Clean(filepath.Join(absSourceDir, replacePath))
		replace.New.Path = absReplacePath
		if replace.Syntax != nil && len(replace.Syntax.Token) > 0 {
			replace.Syntax.Token[len(replace.Syntax.Token)-1] = absReplacePath
		}
	}
	return nil
}

func generatedPluginModulePath(moduleID string) (string, error) {
	moduleID = strings.TrimSpace(moduleID)
	if moduleID == "" {
		return "", errors.New("plugin module id cannot be empty")
	}
	var b strings.Builder
	for _, r := range strings.ToLower(moduleID) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-_")
	if slug == "" {
		return "", errors.Errorf("plugin module id %q has no path characters", moduleID)
	}
	return "github.com/s4wave/spacewave/bldr/plugin/generated/" + slug, nil
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
		ctx,
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

// CompilePluginGoScript compiles the generated plugin module to TypeScript.
func (m *ModuleCompiler) CompilePluginGoScript(
	ctx context.Context,
	le *logrus.Entry,
	outPath string,
	cacheRoot string,
	buildFlags []string,
	overrideDirs []string,
) (string, error) {
	mainPackagePath, err := gocompiler.GoListImportPath(ctx, m.pluginCodegenPath, buildFlags, "GOOS=js", "GOARCH=wasm")
	if err != nil {
		return "", err
	}
	if err := gocompiler.ExecGoScriptCompile(ctx, le, gocompiler.GoScriptCompileOptions{
		WorkDir:                   m.pluginCodegenPath,
		OutputPath:                outPath,
		CacheRoot:                 cacheRoot,
		Packages:                  []string{"."},
		BuildFlags:                buildFlags,
		OverrideDirs:              overrideDirs,
		AllDependencies:           true,
		ProtobufTypeScriptBinding: true,
	}); err != nil {
		return "", err
	}
	return mainPackagePath, nil
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

	ecmd := gocompiler.NewGoCompilerCmd(ctx, "go", args...)
	ecmd.Env = append(ecmd.Env, "GOOS=", "GOARCH=") // host, ignore cgo-enabled
	ecmd.Dir = devSrcDir
	return gocompiler.ExecGoCompiler(m.le, ecmd)
}
