//go:build !js

package bldr_cli_compiler

import (
	"bytes"
	"fmt"
	gast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
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

// CliImport describes a discovered NewCliCommands in a Go package.
type CliImport struct {
	// Alias is the import alias for the package.
	Alias string
	// TakesYieldBroker is true when NewCliCommands declares a second yield
	// broker parameter. Generated wrappers pass the process-shared broker
	// and guard the first bus access with a handoff.
	TakesYieldBroker bool
}

// brokerConsumerFactories maps factory import paths whose controller
// consumes the shared listener brokers to the options the generated
// factory wrapper passes.
var brokerConsumerFactories = map[string][]brokerFactoryOption{
	"github.com/s4wave/spacewave/core/resource/listener": {
		{selector: "WithYieldBroker", brokerField: "yield"},
		{selector: "WithStatusBroker", brokerField: "status"},
	},
	"github.com/s4wave/spacewave/core/resource/root/controller": {
		{selector: "WithYieldBroker", brokerField: "yield"},
		{selector: "WithListenerStatusBroker", brokerField: "status"},
	},
}

// brokerFactoryOption is one broker option passed to a consumer factory.
type brokerFactoryOption struct {
	// selector is the option constructor in the factory package.
	selector string
	// brokerField is the listenerBrokers field passed to it.
	brokerField string
}

// FormatCliEntrypoint formats the generated CLI entrypoint code.
//
// When any discovered factory consumes the shared listener brokers or any
// CLI command builder declares a yield broker parameter, the generated
// composition root constructs one listenerBrokers pair and wires it into
// the broker-consuming factories and the CLI command builders. Otherwise
// the generated entrypoint stays free of broker plumbing.
func FormatCliEntrypoint(
	appName string,
	projectID string,
	factoryImports map[string]FactoryImport,
	cliImports map[string]CliImport,
) ([]byte, error) {
	var allDecls []gast.Decl

	anyCliBroker := false
	for _, ci := range cliImports {
		if ci.TakesYieldBroker {
			anyCliBroker = true
			break
		}
	}
	anyBrokerFactory := false
	for pkgPath := range factoryImports {
		if _, ok := brokerConsumerFactories[pkgPath]; ok {
			anyBrokerFactory = true
			break
		}
	}
	brokersNeeded := anyCliBroker || anyBrokerFactory

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
	if brokersNeeded {
		fixedImports = append(fixedImports,
			struct{ alias, path string }{"resource_listener", "github.com/s4wave/spacewave/core/resource/listener"},
			struct{ alias, path string }{"yield_policy", "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"},
		)
		if anyCliBroker {
			fixedImports = append(fixedImports,
				struct{ alias, path string }{"aperture_cli", "github.com/aperturerobotics/cli"},
			)
		}
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

	if brokersNeeded {
		allDecls = append(allDecls, listenerBrokersDecls()...)
	}

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
		for _, opt := range brokerConsumerFactories[fi.Path] {
			call.Args = append(call.Args, &gast.CallExpr{
				Fun: &gast.SelectorExpr{
					X:   gast.NewIdent(fi.Alias),
					Sel: gast.NewIdent(opt.selector),
				},
				Args: []gast.Expr{&gast.SelectorExpr{
					X:   gast.NewIdent("brokers"),
					Sel: gast.NewIdent(opt.brokerField),
				}},
			})
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

	if brokersNeeded {
		allDecls = append(allDecls, buildFactoriesDecl(factoryElts))
	} else {
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
	}

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

	var cliCommandsExpr gast.Expr
	if anyCliBroker {
		cliCommandsDecl, err := buildCliCommandsDecl(appName, cliImports)
		if err != nil {
			return nil, err
		}
		allDecls = append(allDecls, cliCommandsDecl)
		cliCommandsExpr = &gast.CallExpr{
			Fun:  gast.NewIdent("buildCliCommands"),
			Args: []gast.Expr{gast.NewIdent("brokers")},
		}
	} else {
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
		cliCommandsExpr = gast.NewIdent("cliCommands")
	}

	// main function
	allDecls = append(allDecls, mainDecl(appName, projectID, brokersNeeded, cliCommandsExpr))

	return formatFileWithSpacing(allDecls)
}

// cliCommandsNeedsYieldBroker reports whether a NewCliCommands signature
// takes the shared yield broker as a second parameter.
func cliCommandsNeedsYieldBroker(pkgPath string, sig *types.Signature) (bool, error) {
	switch sig.Params().Len() {
	case 1:
		return false, nil
	case 2:
		return true, nil
	default:
		return false, errors.Errorf("package %s NewCliCommands has unsupported arity %d", pkgPath, sig.Params().Len())
	}
}

// commentGroup builds a doc comment group from comment lines.
func commentGroup(lines ...string) *gast.CommentGroup {
	list := make([]*gast.Comment, 0, len(lines))
	for _, line := range lines {
		list = append(list, &gast.Comment{Text: line})
	}
	return &gast.CommentGroup{List: list}
}

// listenerBrokersDecls builds the shared broker pair type and constructor.
func listenerBrokersDecls() []gast.Decl {
	return []gast.Decl{
		&gast.GenDecl{
			Doc: commentGroup(
				"// listenerBrokers are the shared yield and listener-status brokers wired at\n",
				"// this composition root into the resource listener controller, the root\n",
				"// resource controller, and the CLI commands.\n",
			),
			Tok: token.TYPE,
			Specs: []gast.Spec{
				&gast.TypeSpec{
					Name: gast.NewIdent("listenerBrokers"),
					Type: &gast.StructType{
						Fields: &gast.FieldList{List: []*gast.Field{
							{
								Names: []*gast.Ident{gast.NewIdent("yield")},
								Type: &gast.StarExpr{X: &gast.SelectorExpr{
									X:   gast.NewIdent("yield_policy"),
									Sel: gast.NewIdent("Broker"),
								}},
							},
							{
								Names: []*gast.Ident{gast.NewIdent("status")},
								Type: &gast.StarExpr{X: &gast.SelectorExpr{
									X:   gast.NewIdent("resource_listener"),
									Sel: gast.NewIdent("StatusBroker"),
								}},
							},
						}},
					},
				},
			},
		},
		&gast.FuncDecl{
			Doc:  commentGroup("// newListenerBrokers constructs the process-shared broker pair.\n"),
			Name: gast.NewIdent("newListenerBrokers"),
			Type: &gast.FuncType{
				Params: &gast.FieldList{},
				Results: &gast.FieldList{List: []*gast.Field{
					{Type: &gast.StarExpr{X: gast.NewIdent("listenerBrokers")}},
				}},
			},
			Body: &gast.BlockStmt{List: []gast.Stmt{
				&gast.ReturnStmt{
					Results: []gast.Expr{
						&gast.UnaryExpr{
							Op: token.AND,
							X: &gast.CompositeLit{
								Type: gast.NewIdent("listenerBrokers"),
								Elts: []gast.Expr{
									&gast.KeyValueExpr{
										Key:   gast.NewIdent("yield"),
										Value: &gast.CallExpr{Fun: &gast.SelectorExpr{X: gast.NewIdent("yield_policy"), Sel: gast.NewIdent("NewBroker")}},
									},
									&gast.KeyValueExpr{
										Key:   gast.NewIdent("status"),
										Value: &gast.CallExpr{Fun: &gast.SelectorExpr{X: gast.NewIdent("resource_listener"), Sel: gast.NewIdent("NewStatusBroker")}},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}

// buildFactoriesDecl builds the factory wiring function. Broker-consuming
// factories receive the shared listener brokers through their options.
func buildFactoriesDecl(factoryElts []gast.Expr) gast.Decl {
	return &gast.FuncDecl{
		Doc:  commentGroup("// buildFactories wires the shared listener brokers into every consumer.\n"),
		Name: gast.NewIdent("buildFactories"),
		Type: &gast.FuncType{
			Params: &gast.FieldList{List: []*gast.Field{
				{
					Names: []*gast.Ident{gast.NewIdent("brokers")},
					Type:  &gast.StarExpr{X: gast.NewIdent("listenerBrokers")},
				},
			}},
			Results: &gast.FieldList{List: []*gast.Field{
				{Type: &gast.ArrayType{Elt: &gast.SelectorExpr{
					X:   gast.NewIdent("cli_entrypoint"),
					Sel: gast.NewIdent("AddFactoryFunc"),
				}}},
			}},
		},
		Body: &gast.BlockStmt{List: []gast.Stmt{
			&gast.ReturnStmt{
				Results: []gast.Expr{
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
		}},
	}
}

// buildCliCommandsDecl builds the CLI command wiring function. Each command
// builder receives the shared yield broker and guards the first bus access
// with a handoff so a command process never displaces the foreground serve
// process on the listener socket.
func buildCliCommandsDecl(appName string, cliImports map[string]CliImport) (gast.Decl, error) {
	aliases := make([]string, 0, len(cliImports))
	for _, ci := range cliImports {
		if ci.TakesYieldBroker {
			aliases = append(aliases, ci.Alias)
		}
	}
	slices.Sort(aliases)

	var cliElts []gast.Expr
	for _, alias := range aliases {
		wrapperSrc := fmt.Sprintf(`func(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
	handedOff := false
	protectedGetBus := func() cli_entrypoint.CliBus {
		if !handedOff {
			brokers.yield.BeginHandoff(%q, "")
			handedOff = true
		}
		return getBus()
	}
	return %s.NewCliCommands(protectedGetBus, brokers.yield)
}`, appName+" CLI", alias)
		wrapperExpr, err := parser.ParseExpr(wrapperSrc)
		if err != nil {
			return nil, errors.Wrapf(err, "parse generated cli command wrapper for %s", alias)
		}
		cliElts = append(cliElts, wrapperExpr)
	}

	return &gast.FuncDecl{
		Doc: commentGroup(
			"// buildCliCommands builds the CLI command builders. Each command process\n",
			"// owns a private broker pair: it never shares listener state with the\n",
			"// daemon, and its bus's configured resource listener must not displace the\n",
			"// foreground serve process; serve binds the socket explicitly after\n",
			"// installing its own handoff guard.\n",
		),
		Name: gast.NewIdent("buildCliCommands"),
		Type: &gast.FuncType{
			Params: &gast.FieldList{List: []*gast.Field{
				{
					Names: []*gast.Ident{gast.NewIdent("brokers")},
					Type:  &gast.StarExpr{X: gast.NewIdent("listenerBrokers")},
				},
			}},
			Results: &gast.FieldList{List: []*gast.Field{
				{Type: &gast.ArrayType{Elt: &gast.SelectorExpr{
					X:   gast.NewIdent("cli_entrypoint"),
					Sel: gast.NewIdent("BuildCommandsFunc"),
				}}},
			}},
		},
		Body: &gast.BlockStmt{List: []gast.Stmt{
			&gast.ReturnStmt{
				Results: []gast.Expr{
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
		}},
	}, nil
}

// mainDecl builds the main entrypoint. When brokers are needed it constructs
// the shared pair first and passes the wiring functions through.
func mainDecl(appName, projectID string, brokersNeeded bool, cliCommandsExpr gast.Expr) gast.Decl {
	var factoriesExpr gast.Expr = gast.NewIdent("factories")
	var stmts []gast.Stmt
	if brokersNeeded {
		stmts = append(stmts, &gast.AssignStmt{
			Lhs: []gast.Expr{gast.NewIdent("brokers")},
			Tok: token.DEFINE,
			Rhs: []gast.Expr{&gast.CallExpr{Fun: gast.NewIdent("newListenerBrokers")}},
		})
		factoriesExpr = &gast.CallExpr{
			Fun:  gast.NewIdent("buildFactories"),
			Args: []gast.Expr{gast.NewIdent("brokers")},
		}
	}
	stmts = append(stmts, &gast.ExprStmt{X: &gast.CallExpr{
		Fun: &gast.SelectorExpr{
			X:   gast.NewIdent("cli_entrypoint"),
			Sel: gast.NewIdent("Main"),
		},
		Args: []gast.Expr{
			&gast.BasicLit{Kind: token.STRING, Value: strconv.Quote(appName)},
			&gast.BasicLit{Kind: token.STRING, Value: strconv.Quote(projectID)},
			factoriesExpr,
			gast.NewIdent("configSets"),
			cliCommandsExpr,
		},
	}})
	return &gast.FuncDecl{
		Doc:  commentGroup("// main is the main entrypoint.\n"),
		Name: gast.NewIdent("main"),
		Type: &gast.FuncType{Params: &gast.FieldList{}},
		Body: &gast.BlockStmt{List: stmts},
	}
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
