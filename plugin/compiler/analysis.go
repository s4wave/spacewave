package plugin_compiler

import (
	"context"
	"go/ast"
	"go/build"
	"os"
	"path"
	"strings"

	// "go/parser"
	"go/token"
	"go/types"

	"github.com/pkg/errors"
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

	// controllerFactories contains the set of packages containing controllers
	controllerFactories map[string]*packages.Package
}

// AnalyzePackages analyzes code packages using Go module package resolution.
//
// packagePaths can start with ./ to be relative to the root module path.
func AnalyzePackages(
	ctx context.Context,
	le *logrus.Entry,
	workDir string,
	packagePaths []string,
) (*Analysis, error) {
	// expect go.mod go.sum in the work dir for base module
	baseGoModPath := path.Join(workDir, "go.mod")
	baseGoModData, err := os.ReadFile(baseGoModPath)
	if err != nil {
		return nil, err
	}
	baseModFile, err := modfile.Parse(baseGoModPath, baseGoModData, nil)
	if err != nil {
		return nil, err
	}

	// update relative module paths (./)
	packagePaths = UpdateRelativeGoPackagePaths(packagePaths, baseModFile.Module.Mod.Path)

	res := &Analysis{
		baseModFile:  baseModFile,
		packagePaths: packagePaths,
		imports: map[string]*types.Package{
			// "context": nil,
			"embed": nil,
			"github.com/aperturerobotics/controllerbus/bus":        nil,
			"github.com/aperturerobotics/controllerbus/controller": nil,
			"github.com/aperturerobotics/bldr/plugin/entrypoint":   nil,
		},
		controllerFactories: make(map[string]*packages.Package),
		packages:            make(map[string]*packages.Package),
		module:              make(map[string]*packages.Module),
	}

	builderCtx := build.Default
	builderCtx.BuildTags = append(builderCtx.BuildTags, "bldr_analyze")

	var conf packages.Config
	conf.Context = ctx

	conf.Fset = token.NewFileSet()
	conf.Mode = conf.Mode |
		// NeedName adds Name and PkgPath.
		packages.NeedName |
		// NeedFiles adds GoFiles and OtherFiles.
		packages.NeedFiles |
		// NeedImports adds Imports. If NeedDeps is not set, the Imports field will contain
		// "placeholder" Packages with only the ID set.
		packages.NeedImports |
		// NeedDeps adds the fields requested by the LoadMode in the packages in Imports.
		packages.NeedDeps |
		// NeedExportFile adds ExportFile.
		packages.NeedExportFile |
		// NeedTypes adds Types, Fset, and IllTyped.
		packages.NeedTypes |
		// NeedSyntax adds Syntax.
		packages.NeedSyntax |
		// NeedTypesInfo adds TypesInfo.
		packages.NeedTypesInfo |
		// NeedModule adds Module.
		packages.NeedModule

	conf.Dir = workDir
	conf.Logf = func(format string, args ...interface{}) {
		le.Debugf(format, args...)
	}
	conf.BuildFlags = append(conf.BuildFlags, "-mod=readonly")

	loadedPackages, err := packages.Load(&conf, packagePaths...)
	if err != nil {
		return nil, err
	}
	res.fset = conf.Fset
	for _, pkg := range loadedPackages {
		res.packages[pkg.PkgPath] = pkg
	}

	le.Debugf("loaded %d init packages to analyze", len(loadedPackages))
	if len(loadedPackages) == 0 {
		return nil, errors.New("expected at least one package to be loaded")
	}
	// initPkg := loadedPackages[0]

	factoryModules := res.module

	// Find NewFactory() constructors.
	// Build a list of packages to import.
	for _, pkg := range loadedPackages {
		le := le.WithField("pkg", pkg.Types.Path())

		factoryCtorObj := pkg.Types.Scope().Lookup("NewFactory")
		factoryPkgImportPath := pkg.Types.Path()
		if factoryCtorObj != nil {
			le.Debugf("found factory ctor func: %s", factoryCtorObj.Type().String())
			res.controllerFactories[BuildPackageName(pkg.Types)] = pkg
			if _, ok := res.imports[factoryPkgImportPath]; !ok {
				le.
					WithField("import-path", factoryPkgImportPath).
					WithField("import-name", pkg.Types.Name).
					Debug("added package to plugin-file imports list")
				res.imports[factoryPkgImportPath] = pkg.Types
			}
		} else {
			le.Warn("no factory constructors found")
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

// GetLoadedPackages returns the loaded packages.
func (a *Analysis) GetLoadedPackages() map[string]*packages.Package {
	return a.packages
}

// ParseEsbuildComments searches for bldr:esbuild comments.
func (a *Analysis) ParseEsbuildComments(codeFiles map[string][]*ast.File) (map[string](map[string]*EsbuildArgs), error) {
	esbuildPackagesMap := make(map[string](map[string]*EsbuildArgs))
	getPackageMap := func(pkg string) map[string]*EsbuildArgs {
		m := esbuildPackagesMap[pkg]
		if m == nil {
			m = make(map[string]*EsbuildArgs)
		}
		esbuildPackagesMap[pkg] = m
		return m
	}

	for pkgImportPath, pkgCodeFile := range codeFiles {
		for _, codeFile := range pkgCodeFile {
			cmap := ast.NewCommentMap(a.fset, codeFile, codeFile.Comments)
			for nod, comments := range cmap {
				for _, comment := range comments {
					posErr := func(err error) error {
						pos := a.fset.Position(nod.Pos()).String()
						return errors.Wrap(err, pos)
					}
					declErr := func() error {
						return posErr(errors.Errorf("%s tag must be associated with a single declaration", EsbuildTag))
					}
					var commentPts []string
					for _, commentElem := range comment.List {
						commentTxt, hadPrefix := TrimEsbuildArgs(commentElem.Text)
						if hadPrefix {
							commentPts = append(commentPts, commentTxt)
						}
					}
					if len(commentPts) != 0 {
						fullComment := strings.Join(commentPts, " ")
						decl, declOk := nod.(*ast.GenDecl)
						if !declOk || len(decl.Specs) != 1 {
							return nil, declErr()
						}
						pkgMap := getPackageMap(pkgImportPath)
						spec := decl.Specs[0]
						valueSpec, ok := spec.(*ast.ValueSpec)
						if !ok || len(valueSpec.Names) != 1 || len(valueSpec.Names[0].Name) == 0 {
							return nil, declErr()
						}
						args, err := ParseEsbuildArgs(fullComment)
						if err != nil {
							return nil, posErr(err)
						}
						pkgMap[valueSpec.Names[0].Name] = args
					}
				}
			}
		}
	}
	return esbuildPackagesMap, nil
}

// GetGoCodeFiles returns file paths for packages in the program.
func (a *Analysis) GetGoCodeFiles() map[string][]*ast.File {
	packagePaths := a.packagePaths
	res := make(map[string][]*ast.File)
	addFile := func(pakImportPath string, astFile *ast.File) {
		res[pakImportPath] = append(res[pakImportPath], astFile)
	}

	// collect go files to watch
	for _, pak := range a.packages {
		for i := range pak.Syntax {
			pakImportPath := pak.PkgPath
			if len(packagePaths) != 0 {
				var found bool
				for _, ex := range packagePaths {
					if ex == pakImportPath {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			addFile(pakImportPath, pak.Syntax[i])
		}
	}

	return res
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
