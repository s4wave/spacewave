//go:build !js

package bldr_cli_compiler

import (
	"bytes"
	gast "go/ast"
	"go/format"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// FormatCliEntrypoint formats the generated CLI entrypoint code.
func FormatCliEntrypoint(
	appName string,
	factoryImports map[string]string,
	cliImports map[string]string,
) ([]byte, error) {
	var allDecls []gast.Decl

	// merge and sort all dynamic imports
	allImports := make(map[string]string)
	maps.Copy(allImports, factoryImports)
	maps.Copy(allImports, cliImports)

	importPkgs := make([]string, 0, len(allImports))
	for pkg := range allImports {
		importPkgs = append(importPkgs, pkg)
	}
	slices.Sort(importPkgs)

	// build single parenthesized import declaration
	var importSpecs []gast.Spec
	fixedImports := []struct{ alias, path string }{
		{"", "embed"},
		{"cli_entrypoint", "github.com/s4wave/spacewave/bldr/cli/entrypoint"},
		{"", "github.com/aperturerobotics/controllerbus/bus"},
		{"", "github.com/aperturerobotics/controllerbus/controller"},
	}
	for _, imp := range fixedImports {
		var name *gast.Ident
		if imp.alias != "" {
			name = gast.NewIdent(imp.alias)
		}
		importSpecs = append(importSpecs, &gast.ImportSpec{
			Name: name,
			Path: &gast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(imp.path),
			},
		})
	}
	for _, pkg := range importPkgs {
		importSpecs = append(importSpecs, &gast.ImportSpec{
			Name: gast.NewIdent(allImports[pkg]),
			Path: &gast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(pkg),
			},
		})
	}
	allDecls = append(allDecls, &gast.GenDecl{
		Tok:   token.IMPORT,
		Specs: importSpecs,
	})

	// configSetFS: embed configset.bin
	var embedComment strings.Builder
	embedComment.WriteString("// configSetFS contains the embedded configset.\n")
	embedComment.WriteString("//\n")
	embedComment.WriteString("//go:embed configset.bin\n")
	allDecls = append(allDecls, &gast.GenDecl{
		Tok: token.VAR,
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: embedComment.String(),
			}},
		},
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("configSetFS")},
				Type: &gast.SelectorExpr{
					X:   gast.NewIdent("embed"),
					Sel: gast.NewIdent("FS"),
				},
			},
		},
	})

	// build factory func lit elements
	factoryAliases := make([]string, 0, len(factoryImports))
	for _, alias := range factoryImports {
		factoryAliases = append(factoryAliases, alias)
	}
	slices.Sort(factoryAliases)

	var factoryElts []gast.Expr
	for _, alias := range factoryAliases {
		factoryElts = append(factoryElts, &gast.FuncLit{
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
							Type: &gast.ArrayType{Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("controller"),
								Sel: gast.NewIdent("Factory"),
							}},
							Elts: []gast.Expr{
								&gast.CallExpr{
									Fun: &gast.SelectorExpr{
										X:   gast.NewIdent(alias),
										Sel: gast.NewIdent("NewFactory"),
									},
									Args: []gast.Expr{gast.NewIdent("b")},
								},
							},
						},
					},
				},
			}},
		})
	}

	// factories var
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// factories are the factories included in the binary.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("factories")},
				Values: []gast.Expr{
					&gast.CompositeLit{
						Type: &gast.ArrayType{
							Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("cli_entrypoint"),
								Sel: gast.NewIdent("AddFactoryFunc"),
							},
						},
						Elts: factoryElts,
					},
				},
			},
		},
	})

	// configSets var
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// configSets are the configuration sets to apply on startup.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("configSets")},
				Values: []gast.Expr{
					&gast.CompositeLit{
						Type: &gast.ArrayType{
							Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("cli_entrypoint"),
								Sel: gast.NewIdent("BuildConfigSetFunc"),
							},
						},
						Elts: []gast.Expr{
							&gast.CallExpr{
								Fun: &gast.SelectorExpr{
									X:   gast.NewIdent("cli_entrypoint"),
									Sel: gast.NewIdent("ConfigSetFuncFromFS"),
								},
								Args: []gast.Expr{
									gast.NewIdent("configSetFS"),
									&gast.BasicLit{
										Kind:  token.STRING,
										Value: `"configset.bin"`,
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// build cli command elements
	cliAliases := make([]string, 0, len(cliImports))
	for _, alias := range cliImports {
		cliAliases = append(cliAliases, alias)
	}
	slices.Sort(cliAliases)

	var cliElts []gast.Expr
	for _, alias := range cliAliases {
		cliElts = append(cliElts, &gast.SelectorExpr{
			X:   gast.NewIdent(alias),
			Sel: gast.NewIdent("NewCliCommands"),
		})
	}

	// cliCommands var
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: &gast.CommentGroup{
			List: []*gast.Comment{{
				Text: "// cliCommands are the CLI command builders.\n",
			}},
		},
		Tok: token.VAR,
		Specs: []gast.Spec{
			&gast.ValueSpec{
				Names: []*gast.Ident{gast.NewIdent("cliCommands")},
				Values: []gast.Expr{
					&gast.CompositeLit{
						Type: &gast.ArrayType{
							Elt: &gast.SelectorExpr{
								X:   gast.NewIdent("cli_entrypoint"),
								Sel: gast.NewIdent("BuildCommandsFunc"),
							},
						},
						Elts: cliElts,
					},
				},
			},
		},
	})

	// main function
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
						X:   gast.NewIdent("cli_entrypoint"),
						Sel: gast.NewIdent("Main"),
					},
					Args: []gast.Expr{
						&gast.BasicLit{
							Kind:  token.STRING,
							Value: strconv.Quote(appName),
						},
						gast.NewIdent("factories"),
						gast.NewIdent("configSets"),
						gast.NewIdent("cliCommands"),
					},
				},
			},
		}},
	})

	return formatFileWithSpacing(allDecls)
}

// formatFileWithSpacing formats an AST file with blank lines between top-level declarations.
//
// It creates a FileSet with line position info and assigns positions to each
// declaration so that go/format sees line gaps and inserts blank lines.
func formatFileWithSpacing(decls []gast.Decl) ([]byte, error) {
	const lineWidth = 1000
	const lineGap = 10
	totalLines := 3 + len(decls)*lineGap + 10
	totalSize := totalLines * lineWidth

	fset := token.NewFileSet()
	tokFile := fset.AddFile("main.go", -1, totalSize)
	offsets := make([]int, totalLines)
	for i := range offsets {
		offsets[i] = i * lineWidth
	}
	if !tokFile.SetLines(offsets) {
		panic("failed to set lines")
	}

	base := tokFile.Base()
	for i, d := range decls {
		line := 3 + i*lineGap
		pos := token.Pos(base + (line-1)*lineWidth)
		switch decl := d.(type) {
		case *gast.GenDecl:
			decl.TokPos = pos
			if decl.Tok == token.IMPORT {
				decl.Lparen = pos + 10
			}
		case *gast.FuncDecl:
			decl.Type.Func = pos
		}
	}

	astFile := &gast.File{
		Name:    gast.NewIdent("main"),
		Package: token.Pos(base),
		Decls:   decls,
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, astFile); err != nil {
		return nil, err
	}
	return format.Source(buf.Bytes())
}
