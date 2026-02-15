//go:build !js

package bldr_plugin_compiler_go

import (
	"bytes"
	gast "go/ast"
	"go/format"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	bldr_plugin_vardef_ast "github.com/aperturerobotics/bldr/plugin/vardef/ast"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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
//
// devInfoFile will be loaded at runtime and used to populate variables init().
// if devInfoFile is empty, the values of the go variable defs are hardcoded into init().
func CodegenPluginWrapperFromAnalysis(
	le *logrus.Entry,
	a *Analysis,
	pluginMeta *bldr_plugin.PluginMeta,
	configSetFiles []string,
	goVarDefs []*vardef.PluginVar,
	devInfoFile string,
) (*gast.File, error) {
	var allDecls []gast.Decl
	importStrs := make([]string, 0, len(a.imports))
	for impPkg := range a.imports {
		importStrs = append(importStrs, impPkg)
	}
	slices.Sort(importStrs)
	importStrs = slices.Compact(importStrs)

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
						Value: strconv.Quote(impPath),
					},
				},
			},
		})
	}

	// Build list of static files for StaticFS
	var staticFSFiles []string
	staticFSFiles = append(staticFSFiles, configSetFiles...)

	// StaticFS: embed static files in the binary.
	var assetFSComment strings.Builder
	_, _ = assetFSComment.WriteString("// StaticFS contains embedded static assets.\n")
	if len(staticFSFiles) != 0 {
		_, _ = assetFSComment.WriteString("//\n")
	}
	for _, fileName := range staticFSFiles {
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
				Names: []*gast.Ident{gast.NewIdent("StaticFS")},
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
	slices.Sort(controllerFactoriesPackages)
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

	// PluginStartInfo contains the plugin instance id from the environment.
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// PluginStartInfo contains the json-base64 encoded startup information.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("PluginStartInfo")},
				Values: []gast.Expr{
					&gast.CallExpr{
						Fun: &gast.SelectorExpr{
							X:   gast.NewIdent("strings"),
							Sel: gast.NewIdent("TrimSpace"),
						},
						Args: []gast.Expr{
							&gast.CallExpr{
								Fun: &gast.SelectorExpr{
									X:   gast.NewIdent("os"),
									Sel: gast.NewIdent("Getenv"),
								},
								Args: []gast.Expr{
									&gast.BasicLit{
										Kind:  token.STRING,
										Value: `"BLDR_PLUGIN_START_INFO"`,
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// PluginMeta contains the b58 encoded plugin metadata.
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// PluginMeta contains the b58 encoded plugin metadata.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("PluginMeta")},
				Values: []gast.Expr{
					&gast.BasicLit{
						Kind:  token.STRING,
						Value: strconv.Quote(pluginMeta.MarshalB58()),
					},
				},
			},
		},
	})

	// LogLevel is the default logging level.
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// LogLevel is the default program log level.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("LogLevel")},
				Values: []gast.Expr{
					&gast.SelectorExpr{
						X:   gast.NewIdent("logrus"),
						Sel: gast.NewIdent("DebugLevel"),
					},
				},
			},
		},
	})

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
				gast.NewIdent("StaticFS"),
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

	// check that all imports are defined for the var defs
	for _, varDef := range goVarDefs {
		_, impOk := a.imports[varDef.GetPkgImportPath()]
		if !impOk {
			return nil, errors.Errorf("variable defined for unimported package: %s", varDef.GetPkgImportPath())
		}
	}

	// init initializes any defined variables
	if len(goVarDefs) != 0 {
		var initBody []gast.Stmt

		// if the dev info file is set, load from a file.
		devInfoVarName := "devInfo"
		if devInfoFile != "" {
			initBody = append(initBody,
				// devInfo, err := plugin_entrypoint.PluginDevInfoFromFile("dev-info.bin")
				&gast.AssignStmt{
					Lhs: []gast.Expr{
						&gast.Ident{Name: devInfoVarName},
						&gast.Ident{Name: "err"},
					},
					Tok: token.DEFINE, // :=
					Rhs: []gast.Expr{
						&gast.CallExpr{
							Fun: &gast.SelectorExpr{
								X:   &gast.Ident{Name: "plugin_entrypoint"},
								Sel: &gast.Ident{Name: "PluginDevInfoFromFile"},
							},
							Args: []gast.Expr{
								&gast.BasicLit{Kind: token.STRING, Value: `"dev-info.bin"`},
							},
						},
					},
				},
				// if err != nil { panic(err) }
				&gast.IfStmt{
					Cond: &gast.BinaryExpr{
						X:  &gast.Ident{Name: "err"},
						Op: token.NEQ,
						Y:  &gast.Ident{Name: "nil"},
					},
					Body: &gast.BlockStmt{
						List: []gast.Stmt{
							&gast.ExprStmt{
								X: &gast.CallExpr{
									Fun:  &gast.Ident{Name: "panic"},
									Args: []gast.Expr{&gast.Ident{Name: "err"}},
								},
							},
						},
					},
				},
			)
		}

		// set each of the variables
		for _, varDef := range goVarDefs {
			imp := a.imports[varDef.GetPkgImportPath()]
			pkgName := BuildPackageName(imp)
			var rhs []gast.Expr

			// if the dev info file is set, use it instead of hardcoding the value.
			if devInfoFile != "" {
				exp, err := bldr_plugin_vardef_ast.ToGoDevInfoRefAst(varDef, devInfoVarName)
				if err != nil {
					return nil, err
				}
				rhs = []gast.Expr{exp}
			} else {
				expr, err := bldr_plugin_vardef_ast.ToGoValueAst(varDef)
				if err != nil {
					return nil, err
				}
				rhs = []gast.Expr{expr}
			}

			initBody = append(initBody, &gast.AssignStmt{
				Lhs: []gast.Expr{
					&gast.SelectorExpr{
						X:   gast.NewIdent(pkgName),
						Sel: gast.NewIdent(varDef.GetPkgVar()),
					},
				},
				Tok: token.ASSIGN,
				Rhs: rhs,
			})
		}

		allDecls = append(allDecls, &gast.FuncDecl{
			Doc: &gast.CommentGroup{
				List: []*gast.Comment{{
					Text: "// init sets variables at init time\n",
				}},
			},
			Name: gast.NewIdent("init"),
			Type: &gast.FuncType{Params: &gast.FieldList{}},
			Body: &gast.BlockStmt{List: initBody},
		})
	}

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
						gast.NewIdent("PluginStartInfo"),
						gast.NewIdent("PluginMeta"),
						gast.NewIdent("LogLevel"),
						gast.NewIdent("Factories"),
						gast.NewIdent("ConfigSets"),
					},
				},
			},
		}},
	})

	// _ ensures that at least one line references bldr_values
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Slash: token.NoPos,
				Text:  "// _ ensures that at least one reference to bldr_values is present.",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{
					gast.NewIdent("_"),
				},
				Type: &gast.SelectorExpr{
					X:   gast.NewIdent("bldr_values"),
					Sel: gast.NewIdent("VoidOutput"),
				},
			},
		},
	})

	return &gast.File{
		Name:    gast.NewIdent("main"),
		Package: 5, // fixes gofmt error
		Decls:   allDecls,
	}, nil
}
