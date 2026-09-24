//go:build !js

package bldr_cli_compiler

import (
	"bytes"
	gast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// FactoryImport describes a discovered NewFactory in a Go package.
type FactoryImport struct {
	// Path is the import path of the package.
	Path string
	// Alias is the import alias for the package.
	Alias string
	// PassBus is true when NewFactory takes a single bus.Bus argument.
	// When false, NewFactory takes no arguments.
	PassBus bool
}

// composeAlias is the import alias of the project compose package.
const composeAlias = "project_compose"

// FormatCliEntrypoint formats the generated CLI entrypoint code.
//
// When composePackage is set, the generated main calls its Compose function
// once and appends the returned factories and commands to the discovered ones.
func FormatCliEntrypoint(
	appName string,
	projectID string,
	factoryImports map[string]FactoryImport,
	cliImports map[string]CliImport,
	composePackage string,
) ([]byte, error) {
	var allDecls []gast.Decl

	// merge and sort all dynamic imports
	allImports := make(map[string]string)
	for pkg, fi := range factoryImports {
		allImports[pkg] = fi.Alias
	}
	for pkg, ci := range cliImports {
		allImports[pkg] = ci.Alias
	}
	fixedImports := []struct{ alias, path string }{
		{"", "embed"},
		{"cli_entrypoint", "github.com/s4wave/spacewave/bldr/cli/entrypoint"},
		{"", "github.com/aperturerobotics/controllerbus/bus"},
		{"", "github.com/aperturerobotics/controllerbus/controller"},
	}
	if composePackage != "" {
		fixedImports = append(fixedImports, struct{ alias, path string }{composeAlias, composePackage})
	}
	for _, imp := range fixedImports {
		if existing, ok := allImports[imp.path]; ok && existing != imp.alias {
			return nil, errors.Errorf("import %s alias %q conflicts with generated alias %q", imp.path, imp.alias, existing)
		}
		allImports[imp.path] = imp.alias
	}
	importPkgs := make([]string, 0, len(allImports))
	for pkg := range allImports {
		importPkgs = append(importPkgs, pkg)
	}
	slices.Sort(importPkgs)

	// build single parenthesized import declaration
	var importSpecs []gast.Spec
	for _, pkg := range importPkgs {
		alias := allImports[pkg]
		var name *gast.Ident
		if alias != "" {
			name = gast.NewIdent(alias)
		}
		importSpecs = append(importSpecs, &gast.ImportSpec{
			Name: name,
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
	factories := make([]FactoryImport, 0, len(factoryImports))
	for _, fi := range factoryImports {
		factories = append(factories, fi)
	}
	slices.SortFunc(factories, func(a, b FactoryImport) int {
		return strings.Compare(a.Alias, b.Alias)
	})

	var factoryElts []gast.Expr
	for _, fi := range factories {
		call := &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   gast.NewIdent(fi.Alias),
				Sel: gast.NewIdent("NewFactory"),
			},
		}
		if fi.PassBus {
			call.Args = append(call.Args, gast.NewIdent("b"))
		}
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
							Elts: []gast.Expr{call},
						},
					},
				},
			}},
		})
	}

	// factories var
	allDecls = append(allDecls, &gast.GenDecl{
		Doc: commentGroup("// factories are the factories included in the binary.\n"),
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
		Doc: commentGroup("// configSets are the configuration sets to apply on startup.\n"),
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
	for _, ci := range cliImports {
		cliAliases = append(cliAliases, ci.Alias)
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
		Doc: commentGroup("// cliCommands are the CLI command builders.\n"),
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
	mainFn, err := mainDecl(appName, projectID, composePackage != "")
	if err != nil {
		return nil, err
	}
	allDecls = append(allDecls, mainFn)

	return formatFileWithSpacing(allDecls)
}

// commentGroup builds a doc comment group from comment lines.
func commentGroup(lines ...string) *gast.CommentGroup {
	list := make([]*gast.Comment, 0, len(lines))
	for _, line := range lines {
		list = append(list, &gast.Comment{Text: line})
	}
	return &gast.CommentGroup{List: list}
}

// mainDecl builds the main entrypoint. With a compose package it calls
// Compose once and appends its factories and commands.
func mainDecl(appName, projectID string, composed bool) (gast.Decl, error) {
	factoriesSrc, commandsSrc := "factories", "cliCommands"
	var stmts []gast.Stmt
	if composed {
		stmts = append(stmts, &gast.AssignStmt{
			Lhs: []gast.Expr{gast.NewIdent("composition")},
			Tok: token.DEFINE,
			Rhs: []gast.Expr{&gast.CallExpr{Fun: &gast.SelectorExpr{
				X:   gast.NewIdent(composeAlias),
				Sel: gast.NewIdent("Compose"),
			}}},
		})
		factoriesSrc = "append(factories, composition.Factories...)"
		commandsSrc = "append(cliCommands, composition.Commands...)"
	}

	mainCall, err := parser.ParseExpr("cli_entrypoint.Main(" +
		strconv.Quote(appName) + ", " +
		strconv.Quote(projectID) + ", " +
		factoriesSrc + ", configSets, " +
		commandsSrc + ")")
	if err != nil {
		return nil, errors.Wrap(err, "parse generated main call")
	}
	stmts = append(stmts, &gast.ExprStmt{X: mainCall})
	return &gast.FuncDecl{
		Doc:  commentGroup("// main is the main entrypoint.\n"),
		Name: gast.NewIdent("main"),
		Type: &gast.FuncType{Params: &gast.FieldList{}},
		Body: &gast.BlockStmt{List: stmts},
	}, nil
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
		return nil, errors.New("set line offsets for generated entrypoint")
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
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return bytes.Replace(formatted, []byte("import (\n\t\"embed\"\n\t"), []byte("import (\n\t\"embed\"\n\n\t"), 1), nil
}
