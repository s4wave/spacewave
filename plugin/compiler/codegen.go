package plugin_compiler

import (
	"bytes"
	gast "go/ast"
	"go/format"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// GeneratePluginWrapper generates a wrapper package for a list of packages
// containing controller factories.
func GeneratePluginWrapper(
	le *logrus.Entry,
	an *Analysis,
	configSetFiles []string,
) (*gast.File, error) {
	// Build the plugin main package.
	return CodegenPluginWrapperFromAnalysis(le, an, configSetFiles)
}

// FormatFile formats the output file.
func FormatFile(gf *gast.File) ([]byte, error) {
	var outDat bytes.Buffer
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
//
// configSetFiles will be embedded in the binary and parsed as a ConfigSet.
func CodegenPluginWrapperFromAnalysis(
	le *logrus.Entry,
	a *Analysis,
	configSetFiles []string,
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

	// Build list of static files for assetFS
	var assetFSFiles []string
	assetFSFiles = append(assetFSFiles, configSetFiles...)

	// AssetFS: embed static files in the binary.
	var assetFSComment strings.Builder
	_, _ = assetFSComment.WriteString("// AssetFS contains embedded static assets.\n")
	if len(assetFSFiles) != 0 {
		_, _ = assetFSComment.WriteString("//\n")
	}
	for _, fileName := range assetFSFiles {
		_, _ = assetFSComment.WriteString("//go:embed ")
		_, _ = assetFSComment.WriteString(fileName)
		_, _ = assetFSComment.WriteString("\n")
	}
	allDecls = append(allDecls, &gast.GenDecl{
		Tok: token.VAR,
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: assetFSComment.String(),
			}},
		},
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("AssetFS")},
				Type: &gast.SelectorExpr{
					X:   gast.NewIdent("embed"),
					Sel: gast.NewIdent("FS"),
				},
			},
		},
	})

	// Construct the elements of the slice to return for Factories.
	var buildControllersElts []gast.Expr
	var controllerFactoriesPackages []string
	for fpkg := range a.controllerFactories {
		controllerFactoriesPackages = append(controllerFactoriesPackages, fpkg)
	}
	sort.Strings(controllerFactoriesPackages)
	for _, fpkg := range controllerFactoriesPackages {
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

	// Construct the elements of the slice to return for ConfigSsts.
	var buildConfigSetsElts []gast.Expr
	for _, fileName := range configSetFiles {
		buildConfigSetsElts = append(buildConfigSetsElts, &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   gast.NewIdent("plugin_entrypoint"),
				Sel: gast.NewIdent("ConfigSetFuncFromFS"),
			},
			Args: []gast.Expr{
				gast.NewIdent("AssetFS"),
				&gast.BasicLit{
					Kind:  token.STRING,
					Value: `"` + fileName + `"`,
				},
			},
		})
	}

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
				Values: []gast.Expr{
					&gast.CompositeLit{
						Type: &gast.ArrayType{
							Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("plugin_entrypoint"),
								Sel: gast.NewIdent("BuildConfigSetFunc"),
							},
						},
						Elts: buildConfigSetsElts,
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
		Package: 5, // fixes gofmt error
		Decls:   allDecls,
	}, nil
}
