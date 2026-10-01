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

// Analysis contains target-selected source paths and discovered declarations.
type Analysis struct {
	// fset locates parsed discovery roots.
	fset *token.FileSet
	// packagePaths are the resolved root package paths.
	packagePaths []string
	// packagePathMappings resolve caller-relative package paths.
	packagePathMappings map[string]string
	// packages contains same-module package records keyed by import path.
	packages map[string]*Package
	// codeFiles contains syntax only for discovery roots.
	codeFiles map[string][]*ast.File
	// imports maps generated import paths to their explicit aliases.
	imports map[string]string
	// baseModFile contains source module policy.
	baseModFile *modfile.File
	// module contains modules supplying discovered factories.
	module map[string]*packages.Module
	// workDir is the source module directory.
	workDir string
	// controllerFactories indexes factory packages by generated import alias.
	controllerFactories map[string]*Package
	// typedPackages contains only packages requiring declaration type resolution.
	typedPackages map[string]*types.Package
	// webBundlerOutputType belongs to the same universe as typedPackages.
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
	// Read the source module policy before resolving relative roots.
	baseGoModPath := filepath.Join(workDir, "go.mod")
	baseGoModData, err := os.ReadFile(baseGoModPath)
	if err != nil {
		return nil, errors.Wrapf(err, "read base go.mod at %s", baseGoModPath)
	}
	baseModFile, err := modfile.Parse(baseGoModPath, baseGoModData, nil)
	if err != nil {
		return nil, err
	}

	// Acquire the build budget permit for the analysis work.
	budget, err := bldr_buildbudget.Default()
	if err != nil {
		return nil, err
	}
	permit, err := budget.Acquire(ctx, bldr_buildbudget.GoAnalysisWeight)
	if err != nil {
		return nil, err
	}
	defer permit.Release()

	// Resolve relative roots while retaining their caller-facing mappings.
	packagePaths, packagePathMappings := UpdateRelativeGoPackagePaths(packagePaths, baseModFile.Module.Mod.Path)

	// Initialize the analysis record with the resolved roots and imports.
	res := &Analysis{
		baseModFile:         baseModFile,
		packagePaths:        packagePaths,
		packagePathMappings: packagePathMappings,
		workDir:             workDir,
		imports: map[string]string{
			"embed":   "",
			"os":      "",
			"strings": "",

			"github.com/aperturerobotics/controllerbus/bus":        "",
			"github.com/aperturerobotics/controllerbus/controller": "",
			"github.com/s4wave/spacewave/bldr/values":              "bldr_values",
			"github.com/s4wave/spacewave/bldr/plugin/entrypoint":   "plugin_entrypoint",
			"github.com/sirupsen/logrus":                           "",
		},
		controllerFactories: make(map[string]*Package),
		packages:            make(map[string]*Package),
		module:              make(map[string]*packages.Module),
		codeFiles:           make(map[string][]*ast.File),
		typedPackages:       make(map[string]*types.Package),
	}

	// Normalize the target tags and enable analysis-only declarations.
	buildTags = append(slices.Clone(buildTags), "bldr_analyze")
	slices.Sort(buildTags)
	buildTags = slices.Compact(buildTags)

	// Load the target dependency graph without syntax or type checking.
	var conf packages.Config
	conf.Context = ctx
	conf.Fset = token.NewFileSet()
	conf.Mode = packages.NeedName | packages.NeedCompiledGoFiles |
		packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedModule
	conf.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	}

	// Point the loader at the source module and its build tags.
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

	// Load the roots together with the bundler output package.
	packagesToLoad := append([]string{EsbuildOutputPkgPath}, packagePaths...)

	// Load the target dependency graph without syntax or type checking.
	loadedPackages, err := packages.Load(&conf, packagesToLoad...)
	if err != nil {
		return nil, err
	}

	// Collect every package in the metadata graph for diagnostics.
	var metadataPackages []*packages.Package
	packages.Visit(loadedPackages, nil, func(pkg *packages.Package) {
		metadataPackages = append(metadataPackages, pkg)
	})

	// Fail the analysis when any package reports a load error.
	if err := packageLoadFailureError(metadataPackages, packagesToLoad, buildTags, goos, goarch, workDir); err != nil {
		return nil, err
	}

	// Keep the file set used to parse the discovery roots.
	res.fset = conf.Fset

	// Bound imported discovery and watched inputs to the explicit roots' modules.
	explicitFactoryPackagePaths := make(map[string]struct{}, len(packagePaths))
	for _, packagePath := range packagePaths {
		explicitFactoryPackagePaths[packagePath] = struct{}{}
	}

	// Identify the modules whose dependency paths belong to the program.
	programModulePaths := make(map[string]struct{}, len(packagePaths))
	for _, pkg := range loadedPackages {
		if pkg.Module == nil {
			continue
		}
		if _, ok := explicitFactoryPackagePaths[pkg.PkgPath]; ok {
			programModulePaths[pkg.Module.Path] = struct{}{}
		}
	}

	// Walk same-module imports without retaining their syntax trees.
	programPackages := make(map[string]*packages.Package)
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
		res.packages[pkg.PkgPath] = &Package{Path: pkg.PkgPath, Name: pkg.Name, SourceFiles: pkg.CompiledGoFiles}
		programPackages[pkg.PkgPath] = pkg

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

	// Require at least one analyzed package before continuing.
	le.Debugf("loaded %d init packages to analyze", len(res.packages))
	if len(res.packages) == 0 {
		return nil, errors.New("expected at least one package to be loaded")
	}

	// Parse only discovery roots; dependency filenames already came from metadata.
	typedPaths := make([]string, 0)
	for pkgPath, pkg := range res.packages {
		_, explicit := explicitFactoryPackagePaths[pkgPath]
		if !explicit && !enableImportedFactoryDiscovery {
			continue
		}
		for _, filename := range pkg.SourceFiles {
			file, err := parser.ParseFile(conf.Fset, filename, nil, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			res.codeFiles[pkgPath] = append(res.codeFiles[pkgPath], file)
		}
		if pkg.discoverConstructors(res.codeFiles[pkgPath]) {
			typedPaths = append(typedPaths, pkgPath)
		}
	}

	// Resolve annotation candidates and ambiguous constructors in one type universe.
	codeFiles := res.GetGoCodeFiles()
	for _, tag := range []string{EsbuildTag, ViteTag} {
		candidates, err := FindTagComments(tag, res.fset, codeFiles,
			func(values []string, _ *ast.ValueSpec) (bool, bool, error) {
				for _, value := range values {
					if _, found := TrimCommentArgs(tag, value); found {
						return true, true, nil
					}
				}
				return false, false, nil
			})
		if err != nil {
			return nil, err
		}
		for pkgPath := range candidates {
			typedPaths = append(typedPaths, pkgPath)
		}
	}

	// Load candidates together with the output reference for exact type identity.
	if len(typedPaths) != 0 {
		// Reload the candidate packages with type information enabled.

		typedPaths = append(typedPaths, EsbuildOutputPkgPath)
		slices.Sort(typedPaths)
		typedPaths = slices.Compact(typedPaths)
		conf.Mode = (conf.Mode &^ packages.NeedDeps) | packages.NeedTypes |
			packages.NeedTypesSizes | packages.NeedExportFile
		loaded, err := packages.Load(&conf, typedPaths...)
		if err != nil {
			return nil, err
		}
		if err := packageLoadFailureError(loaded, typedPaths, buildTags, goos, goarch, workDir); err != nil {
			return nil, err
		}
		for _, pkg := range loaded {
			res.typedPackages[pkg.PkgPath] = pkg.Types
			if pkg.PkgPath == EsbuildOutputPkgPath {
				if obj := pkg.Types.Scope().Lookup(EsbuildOutputTypeName); obj != nil {
					res.webBundlerOutputType = obj.Type()
				}
			}
			if len(res.codeFiles[pkg.PkgPath]) != 0 {
				res.packages[pkg.PkgPath].resolveConstructors(pkg.Types)
			}
		}
		if res.webBundlerOutputType == nil {
			return nil, errors.Errorf("could not find %s.%s type", EsbuildOutputPkgPath, EsbuildOutputTypeName)
		}
	}

	// Retain generated imports and source modules without retaining the package graph.
	for pkgPath, pkg := range res.packages {
		if pkg.Factory == nil {
			continue
		}
		res.controllerFactories[pkg.Name] = pkg
		res.imports[pkgPath] = pkg.Name
		if mod := programPackages[pkgPath].Module; mod != nil {
			res.module[mod.Path] = mod
		}
	}

	return res, nil
}

// packageLoadFailureError adds target and pattern context to package diagnostics.
func packageLoadFailureError(loadedPackages []*packages.Package, patterns []string, buildTags []string, goos, goarch, workDir string) error {
	// Collect package diagnostics into one error message.
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

// GetPackages returns the discovered package records keyed by import path.
func (a *Analysis) GetPackages() map[string]*Package {
	return a.packages
}

// GetGoCodeFiles returns syntax for explicitly configured annotation roots.
func (a *Analysis) GetGoCodeFiles() map[string][]*ast.File {
	res := make(map[string][]*ast.File)
	for _, pkgPath := range a.packagePaths {
		if files := a.codeFiles[pkgPath]; len(files) != 0 {
			res[pkgPath] = files
		}
	}
	return res
}

// GetProgramSourceFiles returns target-selected files for the same-module closure.
func (a *Analysis) GetProgramSourceFiles() map[string][]string {
	res := make(map[string][]string, len(a.packages))
	for pkgPath, pkg := range a.packages {
		res[pkgPath] = pkg.SourceFiles
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

// determineVarTypeWithReference distinguishes string-underlying values from the identical output type.
func determineVarTypeWithReference[V any](
	obj types.Object,
	refType types.Type,
	stringTypeValue, // Value to return if the type is a string
	refTypeValue V, // Value to return if the type matches the reference type
	errTag string, // Tag to include in error messages for context
) (V, error) {
	// Aliases share identity with their targets; unrelated named structs do not.
	var empty V
	if types.Identical(obj.Type(), refType) {
		return refTypeValue, nil
	}

	// String-underlying declarations retain the existing entrypoint-path behavior.
	switch t := obj.Type().Underlying().(type) {
	case *types.Basic:
		if t.Kind() == types.String {
			return stringTypeValue, nil
		}
		return empty, errors.Wrapf(ErrUnexpectedVarType, "%s basic type: %v", errTag, t)
	case *types.Struct:
		// Keep named-type diagnostics distinct from anonymous struct diagnostics.
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
		pkgPath := varDef.GetPkgImportPath()
		if pkgPath == "" {
			continue
		}
		if _, ok := a.imports[pkgPath]; ok {
			continue
		}

		// Use the discovered package name without constructing a synthetic type package.
		pkgName := a.packages[pkgPath].Name
		a.imports[pkgPath] = pkgName
		le.WithField("import-path", pkgPath).
			WithField("import-type-name", pkgName).
			Debug("added package to plugin-file imports list")
	}
}
