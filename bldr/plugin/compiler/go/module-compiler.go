//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	// le records compiler operations.
	le *logrus.Entry

	// pluginCodegenPath contains the generated module and its source files.
	pluginCodegenPath string
	// pluginGoModule identifies the generated plugin module.
	pluginGoModule string
}

// NewModuleCompiler constructs a new module compiler.
func NewModuleCompiler(
	le *logrus.Entry,
	pluginCodegenPath string,
	pluginGoModule string,
) (*ModuleCompiler, error) {
	// Resolve the generated directory once for all compiler operations.
	if pluginCodegenPath == "" {
		return nil, errors.New("codegen path cannot be empty")
	}
	pluginCodegenPath, err := filepath.Abs(pluginCodegenPath)
	if err != nil {
		return nil, err
	}

	// Retain the target and logging dependency without starting a build.
	return &ModuleCompiler{
		le: le,

		pluginCodegenPath: pluginCodegenPath,
		pluginGoModule:    pluginGoModule,
	}, nil
}

// GenerateModule builds the module files in the codegen path.
//
// A nonempty configSetBinary is embedded as a config set.
//
// devInfoFile will be loaded at runtime and used to populate variables init().
// If devInfoFile is empty, variable values are embedded in init(). Otherwise,
// the generated development information is written below the codegen directory.
func (m *ModuleCompiler) GenerateModule(
	ctx context.Context,
	analysis *Analysis,
	pluginMeta *bldr_plugin.PluginMeta,
	configSetBinary []byte,
	goVarDefs []*vardef.PluginVar,
	devInfoFile string,
) (*vardef.PluginDevInfo, error) {
	// Keep generated writes inside the compiler's directory, including through links.
	root, err := os.OpenRoot(m.pluginCodegenPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	// Compose the generated module from the loaded source modules.
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
		if err := root.WriteFile(configSetBinFilename, configSetBinary, 0o644); err != nil {
			return nil, err
		}
		configSetBinFiles = append(configSetBinFiles, configSetBinFilename)
	}

	// Create the dev info file if necessary.
	pluginDevInfo := &vardef.PluginDevInfo{PluginVars: goVarDefs}
	if len(devInfoFile) != 0 && len(goVarDefs) != 0 {
		devInfoBin, err := pluginDevInfo.MarshalVT()
		if err != nil {
			return nil, err
		}
		if err := root.WriteFile(devInfoFile, devInfoBin, 0o644); err != nil {
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

	// Remove unused imports before writing the generated entrypoint.
	outPluginCodeFilePath := filepath.Join(m.pluginCodegenPath, "plugin.go")
	pluginCodeData, err = imports.Process(outPluginCodeFilePath, pluginCodeData, nil)
	if err != nil {
		return nil, err
	}
	if err := root.WriteFile("plugin.go", pluginCodeData, 0o644); err != nil {
		return nil, err
	}

	// Resolve the generated module's dependencies before compilation.
	if err := gocompiler.RunGoModTidy(ctx, m.le, m.pluginCodegenPath); err != nil {
		return nil, err
	}

	return pluginDevInfo, nil
}

// writeModuleFiles preserves source dependencies and rewrites local replacements.
func (m *ModuleCompiler) writeModuleFiles(analysis *Analysis) error {
	// Read source dependency policy before creating the generated module.
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

	// Give the plugin its own module path while resolving the source tree locally.
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

	// Confine dependency-file writes to the existing generated directory.
	root, err := os.OpenRoot(m.pluginCodegenPath)
	if err != nil {
		return err
	}
	defer root.Close()
	modFile.Cleanup()
	pluginGoModData, err := modFile.Format()
	if err != nil {
		return err
	}
	if err := root.WriteFile("go.mod", pluginGoModData, 0o644); err != nil {
		return err
	}

	// Preserve the source's dependency checksums when they exist.
	sourceGoSumPath := filepath.Join(analysis.workDir, "go.sum")
	sourceGoSumData, err := os.ReadFile(sourceGoSumPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "read source go.sum at %s", sourceGoSumPath)
	}
	return root.WriteFile("go.sum", sourceGoSumData, 0o644)
}

// absolutizeModuleReplaces keeps local replacements relative to the source module.
func absolutizeModuleReplaces(modFile *modfile.File, sourceDir string) error {
	// Resolve replacements against the original module's absolute directory.
	absSourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return err
	}

	// Versioned and already absolute replacements retain their original meaning.
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

// generatedPluginModulePath maps a plugin ID into the generated-module namespace.
func generatedPluginModulePath(moduleID string) (string, error) {
	// Require a nonempty identifier before deriving its import-path component.
	moduleID = strings.TrimSpace(moduleID)
	if moduleID == "" {
		return "", errors.New("plugin module id cannot be empty")
	}

	// Replace characters that cannot occur in the generated module's final component.
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
	deferredFunctions []string,
) (string, error) {
	// Resolve the generated main package and its JavaScript binding roots.
	mainPackagePath, err := gocompiler.GoListImportPath(ctx, m.pluginCodegenPath, buildFlags, "GOOS=js", "GOARCH=wasm")
	if err != nil {
		return "", err
	}
	bindingRoots, err := gocompiler.GoScriptBindingRoots(ctx, m.pluginCodegenPath, "GOOS=js", "GOARCH=wasm")
	if err != nil {
		return "", err
	}

	// Compile the generated module with its dependencies and binding policy.
	if err := gocompiler.ExecGoScriptCompile(ctx, le, gocompiler.GoScriptCompileOptions{
		WorkDir:                   m.pluginCodegenPath,
		OutputPath:                outPath,
		CacheRoot:                 cacheRoot,
		Packages:                  []string{"."},
		BuildFlags:                buildFlags,
		OverrideDirs:              overrideDirs,
		BindingRoots:              bindingRoots,
		DeferredFunctions:         deferredFunctions,
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
	// Create the host-only development wrapper below the generated directory.
	devSrcDir := filepath.Join(m.pluginCodegenPath, "dev")
	devSrcMain := filepath.Join(devSrcDir, "main.go")
	if err := os.MkdirAll(devSrcDir, 0o755); err != nil {
		return err
	}
	devWrapperSrc, err := GetDevWrapper()
	if err != nil {
		return err
	}

	// Carry compiler and target flags into the wrapper's runtime build command.
	goArgs := gocompiler.GetDefaultArgs()

	buildTags := gocompiler.NewBuildTags(buildType)
	if len(buildTags) != 0 {
		goArgs = append(goArgs, "-tags="+strings.Join(buildTags, ","))
	}

	// Retain source paths and disable optimizations for debugger attachment.
	goArgs = append(goArgs, "-gcflags", "-N -l")

	// The development wrapper runs on the build host.
	goEnv := gocompiler.GetDefaultEnv()
	goEnv = append(goEnv, "GOOS=", "GOARCH=")
	if enableCgo {
		goEnv = append(goEnv, "CGO_ENABLED=1")
	} else {
		goEnv = append(goEnv, "CGO_ENABLED=0")
	}

	// Quote runtime arguments as Go string literals without interpreting their contents.
	var source strings.Builder
	source.WriteString(devWrapperSrc)
	source.WriteString("\nfunc init() {\n")
	for _, binding := range []struct {
		name   string
		values []string
	}{{"BuildFlags", goArgs}, {"BuildEnv", goEnv}} {
		source.WriteString(binding.name + " = []string{")
		for _, value := range binding.values {
			source.WriteString(strconv.Quote(value) + ",")
		}
		source.WriteString("}\n")
	}
	source.WriteString("}\n")
	if err := os.WriteFile(devSrcMain, []byte(source.String()), 0o644); err != nil {
		return err
	}

	// Compile the wrapper with an optional validated debugger address.
	args := append([]string{"build", "-trimpath", "-o", outFile}, gocompiler.GetDefaultArgs()...)

	if dlvAddr != "" {
		if err := ValidateDelveAddr(dlvAddr); err != nil {
			return errors.Wrap(err, "dlv_addr")
		}
		args = append(args, "-ldflags", "-X 'main.DelveAddr="+dlvAddr+"'")
	}

	// Run in the generated wrapper module and retain the host's environment.
	args = append(args, ".")

	ecmd := gocompiler.NewGoCompilerCmd(ctx, "go", args...)
	ecmd.Env = append(ecmd.Env, "GOOS=", "GOARCH=") // host, ignore cgo-enabled
	ecmd.Dir = devSrcDir
	return gocompiler.ExecGoCompiler(m.le, ecmd)
}
