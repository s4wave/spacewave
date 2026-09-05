//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
	vardef "github.com/s4wave/spacewave/bldr/plugin/vardef"
	bldr_buildbudget "github.com/s4wave/spacewave/bldr/util/buildbudget"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// Analysis contains the result of code analysis.
type Analysis struct {
	// fset is the file set
	fset *token.FileSet
	// packagePaths are the resolved root package paths.
	packagePaths []string
	// packagePathMappings are mappings from the provided go pkg path to the resolved one.
	packagePathMappings map[string]string
	// packages are the imported packages
	// keyed by package path
	packages map[string]*packages.Package
	// imports contains the set of packages to import
	// keyed by import path
	imports map[string]*types.Package
	// baseModFile contains the base module file from the workDir.
	baseModFile *modfile.File
	// module contains all factory modules
	module map[string]*packages.Module
	// workDir is the working directory
	workDir string

	// controllerFactories contains the set of packages containing controllers
	controllerFactories map[string]*packages.Package

	// webBundlerOutputType is the type of EsbuildOutput and WebBundlerOutput
	webBundlerOutputType types.Type
}

// AnalyzePackages analyzes code packages using Go module package resolution.
//
// packagePaths can start with ./ to be relative to the root module path.
//
// goos and goarch select the build environment used to evaluate per-file
// build tags during analysis. Pass the target platform's GOOS/GOARCH so
// factories gated on platform-specific tags (e.g. "//go:build !js") match
// the target compile rather than the analysis host. Empty strings fall
// back to linux/amd64. When enableImportedFactoryDiscovery is false,
// factory discovery only uses the explicit packagePaths roots; imported
// packages are still loaded for dependency and source analysis.
func AnalyzePackages(
	ctx context.Context,
	le *logrus.Entry,
	workDir string,
	packagePaths []string,
	buildTags []string,
	goos, goarch string,
	enableImportedFactoryDiscovery bool,
) (*Analysis, error) {
	// expect go.mod go.sum in the work dir for base module
	baseGoModPath := filepath.Join(workDir, "go.mod")
	baseGoModData, err := os.ReadFile(baseGoModPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read base go.mod at %s", baseGoModPath)
	}
	baseModFile, err := modfile.Parse(baseGoModPath, baseGoModData, nil)
	if err != nil {
		return nil, err
	}
	budget, err := bldr_buildbudget.Default()
	if err != nil {
		return nil, err
	}
	permit, err := budget.Acquire(ctx, bldr_buildbudget.GoAnalysisWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()

	// update relative module paths (./)
	packagePaths, packagePathMappings := UpdateRelativeGoPackagePaths(packagePaths, baseModFile.Module.Mod.Path)

	res := &Analysis{
		baseModFile:         baseModFile,
		packagePaths:        packagePaths,
		packagePathMappings: packagePathMappings,
		workDir:             workDir,
		imports: map[string]*types.Package{
			// "context": nil,
			"embed":   nil,
			"os":      nil,
			"strings": nil,

			"github.com/aperturerobotics/controllerbus/bus":        nil,
			"github.com/aperturerobotics/controllerbus/controller": nil,
			"github.com/s4wave/spacewave/bldr/values":              types.NewPackage("github.com/s4wave/spacewave/bldr/values", "bldr_values"),
			"github.com/s4wave/spacewave/bldr/plugin/entrypoint":   types.NewPackage("github.com/s4wave/spacewave/bldr/plugin/entrypoint", "plugin_entrypoint"),
			"github.com/sirupsen/logrus":                           nil,
		},
		controllerFactories: make(map[string]*packages.Package),
		packages:            make(map[string]*packages.Package),
		module:              make(map[string]*packages.Module),
	}

	// build tags
	buildTags = append(slices.Clone(buildTags), "bldr_analyze")
	slices.Sort(buildTags)
	buildTags = slices.Compact(buildTags)

	var conf packages.Config
	conf.Context = ctx

	conf.Fset = token.NewFileSet()
	// Discover the program before loading syntax. Dependencies outside its modules
	// need export data, not complete syntax trees and expression type records.
	conf.Mode = packages.NeedName | packages.NeedCompiledGoFiles |
		packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedModule
	conf.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	}

	conf.Dir = workDir
	conf.Logf = func(format string, args ...any) {
		le.Debugf(format, args...)
	}
	conf.BuildFlags = append(conf.BuildFlags, "-mod=readonly")
	if len(buildTags) != 0 {
		conf.BuildFlags = append(conf.BuildFlags, "-tags="+strings.Join(buildTags, ","))
	}

	// Use the target platform's GOOS / GOARCH so build-tag gating during
	// analysis matches the target compile. Empty inputs fall back to
	// linux/amd64 for backwards compatibility with callers that have no
	// concrete target (e.g. unit tests).
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}
	conf.Env = append(os.Environ(), gocompiler.GetDefaultEnv()...)
	conf.Env = append(conf.Env, "GOOS="+goos, "GOARCH="+goarch)

	// Add values packages to the packages to load for type comparison
	packagesToLoad := append([]string{EsbuildOutputPkgPath}, packagePaths...)

	// Load the packages
	loadedPackages, err := packages.Load(&conf, packagesToLoad...)
	if err != nil {
		return nil, err
	}
	if err := packageLoadFailureError(loadedPackages, packagesToLoad, buildTags, goos, goarch, workDir); err != nil {
		return nil, err
	}
	res.fset = conf.Fset

	explicitFactoryPackagePaths := make(map[string]struct{}, len(packagePaths))
	for _, packagePath := range packagePaths {
		explicitFactoryPackagePaths[packagePath] = struct{}{}
	}

	programModulePaths := make(map[string]struct{}, len(packagePaths))
	for _, pkg := range loadedPackages {
		if pkg.Module == nil {
			continue
		}
		if _, ok := explicitFactoryPackagePaths[pkg.PkgPath]; ok {
			programModulePaths[pkg.Module.Path] = struct{}{}
		}
	}

	addPkgsStack := make([]*packages.Package, len(loadedPackages))
	copy(addPkgsStack, loadedPackages)
	for len(addPkgsStack) != 0 {
		pkg := addPkgsStack[len(addPkgsStack)-1]
		addPkgsStack = addPkgsStack[:len(addPkgsStack)-1]
		if _, ok := res.packages[pkg.PkgPath]; ok || pkg.Module == nil {
			continue
		}
		if _, ok := programModulePaths[pkg.Module.Path]; !ok {
			continue
		}
		res.packages[pkg.PkgPath] = pkg

		// add other packages from the same module as well
		for _, lpkg := range pkg.Imports {
			if _, ok := res.packages[lpkg.PkgPath]; ok || lpkg.Module == nil {
				continue
			}
			if lpkg.Module.Path == pkg.Module.Path {
				addPkgsStack = append(addPkgsStack, lpkg)
			}
		}
	}

	le.Debugf("loaded %d init packages to analyze", len(res.packages))
	if len(res.packages) == 0 {
		return nil, errors.New("expected at least one package to be loaded")
	}

	// Load one coherent type universe for the program packages and the output
	// type used by tagged variables. Imported dependencies use compiler exports.
	typedPaths := []string{EsbuildOutputPkgPath}
	for pkgPath := range res.packages {
		typedPaths = append(typedPaths, pkgPath)
	}
	slices.Sort(typedPaths)
	typedPaths = slices.Compact(typedPaths)
	conf.Mode = (conf.Mode &^ packages.NeedDeps) | packages.NeedTypes |
		packages.NeedSyntax | packages.NeedTypesSizes | packages.NeedExportFile
	loadedPackages, err = packages.Load(&conf, typedPaths...)
	if err != nil {
		return nil, err
	}
	if err := packageLoadFailureError(loadedPackages, typedPaths, buildTags, goos, goarch, workDir); err != nil {
		return nil, err
	}
	for _, pkg := range loadedPackages {
		if _, ok := res.packages[pkg.PkgPath]; ok {
			res.packages[pkg.PkgPath] = pkg
		}
	}

	// Find and store the web bundler output type
	for _, pkg := range loadedPackages {
		if pkg.PkgPath == EsbuildOutputPkgPath {
			if obj := pkg.Types.Scope().Lookup(EsbuildOutputTypeName); obj != nil {
				res.webBundlerOutputType = obj.Type()
			}
			break
		}
	}

	// If we couldn't find the type, return an error since we need it for type comparison
	if res.webBundlerOutputType == nil {
		return nil, errors.Errorf("could not find %s.%s type", EsbuildOutputPkgPath, EsbuildOutputTypeName)
	}

	// Find NewFactory() constructors.
	// Build a list of packages to import.
	factoryModules := res.module
	for _, pkg := range res.packages {
		le := le.WithField("pkg", pkg.Types.Path())

		if !enableImportedFactoryDiscovery {
			if _, ok := explicitFactoryPackagePaths[pkg.Types.Path()]; !ok {
				continue
			}
		}

		factoryCtorObj := pkg.Types.Scope().Lookup("NewFactory")
		if factoryCtorObj == nil {
			// le.Debug("no controller factories found in package")
			continue
		}

		le.Debugf("found factory ctor func: %s", factoryCtorObj.Type().String())
		res.controllerFactories[BuildPackageName(pkg.Types)] = pkg

		factoryPkgImportPath := pkg.Types.Path()
		if _, ok := res.imports[factoryPkgImportPath]; !ok {
			le.
				WithField("import-path", factoryPkgImportPath).
				WithField("import-type-name", pkg.Types.Name()).
				Debug("added package to plugin-file imports list")
			res.imports[factoryPkgImportPath] = pkg.Types
		}

		if pkg.Module == nil {
			le.Warn("no module was resolved for package")
			continue
		}

		factoryMod := pkg.Module
		if _, ok := factoryModules[factoryMod.Path]; !ok {
			le.
				WithField("import-path", factoryPkgImportPath).
				WithField("module-path", factoryMod.Path).
				WithField("module-version", factoryMod.Version).
				Debug("added module to modules list")
			factoryModules[factoryMod.Path] = factoryMod
		}
	}

	return res, nil
}

func packageLoadFailureError(loadedPackages []*packages.Package, patterns []string, buildTags []string, goos, goarch, workDir string) error {
	var details strings.Builder
	if len(loadedPackages) == 0 {
		details.WriteString("no packages loaded")
	}
	for _, pkg := range loadedPackages {
		for _, pkgErr := range pkg.Errors {
			if details.Len() != 0 {
				details.WriteString("; ")
			}
			pkgName := pkg.PkgPath
			if pkgName == "" {
				pkgName = pkg.ID
			}
			if pkgName == "" {
				pkgName = "<unknown>"
			}
			details.WriteString(pkgName)
			details.WriteString(": ")
			details.WriteString(pkgErr.Error())
		}
	}
	if details.Len() == 0 {
		return nil
	}
	return errors.Errorf(
		"package load failed: %s (patterns=%s; tags=%s; GOOS=%s; GOARCH=%s; workDir=%s)",
		details.String(),
		strings.Join(patterns, ","),
		strings.Join(buildTags, ","),
		goos,
		goarch,
		workDir,
	)
}

// GetPackagePaths returns the resolved root package paths.
func (a *Analysis) GetPackagePaths() []string {
	return a.packagePaths
}

// GetPackagePathMappings returns the mappings from the provided go pkg path to the resolved one.
func (a *Analysis) GetPackagePathMappings() map[string]string {
	return a.packagePathMappings
}

// GetLoadedPackages returns the loaded packages.
func (a *Analysis) GetLoadedPackages() map[string]*packages.Package {
	return a.packages
}

// GetGoCodeFiles returns file paths for explicitly configured packages.
func (a *Analysis) GetGoCodeFiles() map[string][]*ast.File {
	packagePaths := make(map[string]struct{}, len(a.packagePaths))
	for _, packagePath := range a.packagePaths {
		packagePaths[packagePath] = struct{}{}
	}
	return a.getGoCodeFiles(packagePaths)
}

// GetProgramGoCodeFiles returns Go files for all same-module packages loaded into the program.
func (a *Analysis) GetProgramGoCodeFiles() map[string][]*ast.File {
	return a.getGoCodeFiles(nil)
}

func (a *Analysis) getGoCodeFiles(packagePaths map[string]struct{}) map[string][]*ast.File {
	res := make(map[string][]*ast.File)
	addFile := func(pakImportPath string, astFile *ast.File) {
		res[pakImportPath] = append(res[pakImportPath], astFile)
	}

	// collect go files to watch
	for _, pak := range a.packages {
		for i := range pak.Syntax {
			pakImportPath := pak.PkgPath
			if len(packagePaths) != 0 {
				if _, ok := packagePaths[pakImportPath]; !ok {
					continue
				}
			}
			addFile(pakImportPath, pak.Syntax[i])
		}
	}

	return res
}

// GetFileSet returns the token file set.
func (a *Analysis) GetFileSet() *token.FileSet {
	return a.fset
}

// GetFileToken returns the file corresponding to the syntax object.
func (a *Analysis) GetFileToken(syn *ast.File) *token.File {
	return a.fset.File(syn.Pos())
}

// GetBaseModFile returns the parsed ModFile from the working dir.
func (a *Analysis) GetBaseModFile() *modfile.File {
	return a.baseModFile
}

// GetImportedModules returns the list of modules imported in the packages.
func (a *Analysis) GetImportedModules() map[string]*packages.Module {
	return a.module
}

// isTypeIdentical checks if a type is identical to a reference type
func (a *Analysis) isTypeIdentical(t types.Type, refType types.Type) bool {
	if refType == nil {
		return false
	}
	return types.Identical(t, refType)
}

// determineVarTypeWithReference determines the variable type by comparing with a reference type
// and handling common type patterns
func determineVarTypeWithReference[V any](
	a *Analysis,
	obj types.Object,
	refType types.Type,
	stringTypeValue, // Value to return if the type is a string
	refTypeValue V, // Value to return if the type matches the reference type
	errTag string, // Tag to include in error messages for context
) (V, error) {
	var empty V
	// First check if it's directly the reference type
	if a.isTypeIdentical(obj.Type(), refType) {
		return refTypeValue, nil
	}

	// Check the underlying type
	switch t := obj.Type().Underlying().(type) {
	case *types.Basic:
		if t.Kind() == types.String {
			return stringTypeValue, nil // Return string value for string types
		}
		return empty, errors.Wrapf(ErrUnexpectedVarType, "%s basic type: %v", errTag, t)
	case *types.Named, *types.Struct:
		// For named types and struct types, check if the original type matches reference
		if a.isTypeIdentical(obj.Type(), refType) {
			return refTypeValue, nil
		}

		// Get a descriptive name for error reporting
		if named, ok := obj.Type().(*types.Named); ok && named.Obj().Pkg() != nil {
			return empty, errors.Wrapf(ErrUnexpectedVarType, "%s named type: %v.%v",
				errTag, named.Obj().Pkg().Path(), named.Obj().Name())
		}

		return empty, errors.Wrapf(ErrUnexpectedVarType, "%s struct type", errTag)
	default:
		return empty, errors.Wrapf(ErrUnexpectedVarType, "%s type: %T", errTag, t)
	}
}

// AddVariableDefImports adds imports for the given variable defs.
func (a *Analysis) AddVariableDefImports(le *logrus.Entry, varDefs []*vardef.PluginVar) {
	for _, varDef := range varDefs {
		if pkgPath := varDef.GetPkgImportPath(); pkgPath != "" {
			_, ok := a.imports[pkgPath]
			if !ok {
				pkg := a.packages[pkgPath]
				pkgPath := pkg.Types.Path()
				pkgName := pkg.Types.Name()
				a.imports[pkgPath] = types.NewPackage(pkgPath, pkgName)
				le.
					WithField("import-path", pkgPath).
					WithField("import-type-name", pkgName).
					Debug("added package to plugin-file imports list")
			}
		}
	}
}
