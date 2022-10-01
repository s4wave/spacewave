package plugin_compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	gast "go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/sirupsen/logrus"
)

// GeneratePluginWrapper generates a wrapper package for a list of packages
// containing controller factories.
func GeneratePluginWrapper(
	le *logrus.Entry,
	an *Analysis,
) (*gast.File, error) {
	// Build the plugin main package.
	return CodegenPluginWrapperFromAnalysis(le, an)
}

// FormatFile formats the output file.
func FormatFile(gf *gast.File) ([]byte, error) {
	var outDat bytes.Buffer
	outDat.WriteString("//go:build " + buildTag + "\n\n")
	// fset := prog.Fset
	mergeImports(gf)
	fset := token.NewFileSet()
	if err := format.Node(&outDat, fset, gf); err != nil {
		return nil, err
	}
	return outDat.Bytes(), nil
}

// BuildPackageName builds the unique name for the package.
func BuildPackageName(pkg *types.Package) string {
	// for now just use package name
	return pkg.Name()
}

// CodegenPluginWrapperFromAnalysis codegens a plugin wrapper from analysis.
func CodegenPluginWrapperFromAnalysis(
	le *logrus.Entry,
	a *Analysis,
) (*gast.File, error) {
	var allDecls []gast.Decl
	importStrs := make([]string, 0, len(a.imports))
	for impPkg := range a.imports {
		importStrs = append(importStrs, impPkg)
	}
	sort.Strings(importStrs)
	for _, impPath := range importStrs {
		impPkg := a.imports[impPath]
		// impPkg may be nil
		var impIdent *gast.Ident
		if impPkg != nil {
			impIdent = gast.NewIdent(BuildPackageName(impPkg))
		}
		allDecls = append(allDecls, &gast.GenDecl{
			Tok: token.IMPORT,
			Specs: []gast.Spec{
				&gast.ImportSpec{
					Name: impIdent,
					Path: &gast.BasicLit{
						Kind:  token.STRING,
						Value: `"` + impPath + `"`,
					},
				},
			},
		})
	}

	// Construct the elements of the slice to return from Factories.
	var buildControllersElts []gast.Expr
	for fpkg := range a.controllerFactories {
		buildControllersElts = append(buildControllersElts, &gast.CallExpr{
			Args: []gast.Expr{
				gast.NewIdent("b"),
			},
			Fun: &gast.SelectorExpr{
				Sel: gast.NewIdent("NewFactory"),
				X:   gast.NewIdent(fpkg),
			},
		})
	}

	// Factories are the factories included in the binary.
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// Factories are the factories included in the binary.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("Factories")},
				Values: []gast.Expr{
					&gast.CompositeLit{
						Type: &gast.ArrayType{
							Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("plugin_entrypoint"),
								Sel: gast.NewIdent("AddFactoryFunc"),
							},
						},
						Elts: []gast.Expr{
							&gast.FuncLit{
								Type: &gast.FuncType{
									Params: &gast.FieldList{
										List: []*gast.Field{{
											Names: []*gast.Ident{gast.NewIdent("b")},
											Type: &gast.SelectorExpr{
												X:   gast.NewIdent("bus"),
												Sel: gast.NewIdent("Bus"),
											},
										}},
									},
									Results: &gast.FieldList{List: []*gast.Field{
										{Type: &gast.ArrayType{Elt: &gast.SelectorExpr{
											X:   gast.NewIdent("controller"),
											Sel: gast.NewIdent("Factory"),
										}}},
									}},
								},
								Body: &gast.BlockStmt{List: []gast.Stmt{
									&gast.ReturnStmt{
										Results: []gast.Expr{
											&gast.CompositeLit{
												Elts: buildControllersElts,
												Type: &gast.ArrayType{Elt: &gast.SelectorExpr{
													X:   gast.NewIdent("controller"),
													Sel: gast.NewIdent("Factory"),
												}},
											},
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	})

	// ConfigSets are the configuration sets to apply on startup.
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// ConfigSets are the configuration sets to apply on startup.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("ConfigSets")},
				Type: &gast.ArrayType{
					Elt: &gast.SelectorExpr{
						X:   gast.NewIdent("plugin_entrypoint"),
						Sel: gast.NewIdent("BuildConfigSetFunc"),
					},
				},
			},
		},
	})

	// main runs the main process.
	allDecls = append(allDecls, &gast.FuncDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// main is the main entrypoint.\n",
			}},
		},
		Name: gast.NewIdent("main"),
		Type: &gast.FuncType{Params: &gast.FieldList{}},
		Body: &gast.BlockStmt{List: []gast.Stmt{
			&gast.ExprStmt{
				X: &gast.CallExpr{
					Fun: &gast.SelectorExpr{
						X:   gast.NewIdent("plugin_entrypoint"),
						Sel: gast.NewIdent("Main"),
					},
					Args: []gast.Expr{
						gast.NewIdent("Factories"),
						gast.NewIdent("ConfigSets"),
					},
				},
			},
		}},
	})

	return &gast.File{
		Name:    gast.NewIdent("main"),
		Package: 5, // Force after build tag.
		Decls:   allDecls,
	}, nil
}

// BuildPlugin builds a plugin using a temporary code-gen path.
//
// Automates the end-to-end build process with reasonable defaults.
// If codegenDir is empty, uses a tmpdir in the user .cache directory.
func BuildPlugin(ctx context.Context, le *logrus.Entry, packageSearchPath, outputPath, codegenDir string, packages []string) error {
	var err error
	packageSearchPath, err = filepath.Abs(packageSearchPath)
	if err != nil {
		return err
	}

	le.Infof("analyzing %d packages for plugin", len(packages))
	an, err := AnalyzePackages(ctx, le, packageSearchPath, packages)
	if err != nil {
		return err
	}

	// deterministic prefix gen
	var buildUid string
	{
		hs := sha256.New()
		for _, p := range packages {
			_, _ = hs.Write([]byte(p))
		}
		buildUid = b58.Encode(hs.Sum(nil))
	}

	if codegenDir != "" {
		codegenDir, err = filepath.Abs(codegenDir)
		if err != nil {
			return err
		}
	} else {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		codegenDir = filepath.Join(userCacheDir, "cbus", "codegen", buildUid)

		// remove codegen dir on exit
		le.Debugf("creating tmpdir for codegen: %s", codegenDir)
		defer func() {
			_ = os.RemoveAll(codegenDir)
		}()
	}

	if err := os.MkdirAll(codegenDir, 0755); err != nil {
		return err
	}

	buildPrefix := "cbus-plugin-" + (buildUid[:8])
	pluginID := buildPrefix
	le.
		WithField("build-prefix", buildPrefix).
		Infof("creating compiler for plugin with packages: %v", packages)
	mc, err := NewModuleCompiler(ctx, le, buildPrefix, codegenDir, pluginID)
	if err != nil {
		return err
	}

	err = mc.GenerateModules(an)
	if err != nil {
		return err
	}

	outputPath, err = filepath.Abs(outputPath)
	if err == nil {
		err = os.MkdirAll(path.Dir(outputPath), 0755)
	}
	if err != nil {
		return err
	}
	return mc.CompilePlugin(outputPath)
}
