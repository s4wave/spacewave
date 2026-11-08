// Package quadscan provides a golangci-lint module plugin that flags a World
// graph quad that fixes the predicate but neither the subject nor the object.
// A lookup with such a quad scans every quad with that predicate in the World,
// so its cost grows with the World rather than with the result. Link each
// member to an index object and look up by the index object instead.
package quadscan

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("quadscan", New)
}

// worldPkg is the package that defines the graph quad constructors.
const worldPkg = "github.com/s4wave/spacewave/db/world"

// plugin is the quadscan linter.
type plugin struct{}

// New constructs the plugin.
func New(any) (register.LinterPlugin, error) {
	return &plugin{}, nil
}

// BuildAnalyzers returns the analysis passes.
func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

// GetLoadMode returns the load mode, which needs type information.
func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

// Analyzer flags predicate-only graph quads outside tests.
var Analyzer = &analysis.Analyzer{
	Name:      "quadscan",
	Doc:       "flags a graph quad that fixes only the predicate, which scans every quad with that predicate",
	Run:       run,
	FactTypes: []analysis.Fact{new(quadCtor)},
}

// quadCtor marks a function that returns a graph quad built from its
// parameters. Subj and Obj are the parameter indexes the quad's subject and
// object come from, or argEmpty when the function passes an empty string.
type quadCtor struct {
	// Subj is the subject's source.
	Subj int
	// Pred is the predicate's source.
	Pred int
	// Obj is the object's source.
	Obj int
}

// AFact marks quadCtor as a fact.
func (*quadCtor) AFact() {}

// String describes the fact.
func (*quadCtor) String() string { return "quadCtor" }

const (
	// argEmpty is a constant empty string.
	argEmpty = -1
	// argSet is any other value.
	argSet = -2
)

// baseCtors are the World's own constructors, with the subject at 0, the
// predicate at 1 and the object at 2.
var baseCtors = map[string]bool{"NewGraphQuad": true, "NewGraphQuadWithKeys": true}

func run(pass *analysis.Pass) (any, error) {
	// Export a fact for each function that wraps a quad constructor.
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || len(fd.Body.List) != 1 {
				continue
			}
			ret, ok := fd.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			call, ok := ret.Results[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			ctor := lookupCtor(pass, call)
			if ctor == nil {
				continue
			}
			fn, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
			if !ok {
				continue
			}
			params := paramIndexes(pass, fd)
			fact := &quadCtor{
				Subj: forward(pass, call, ctor.Subj, params),
				Pred: forward(pass, call, ctor.Pred, params),
				Obj:  forward(pass, call, ctor.Obj, params),
			}
			if fact.Subj != argSet && fact.Obj != argSet && fact.Pred != argEmpty {
				pass.ExportObjectFact(fn, fact)
			}
		}
	}

	// Report each construction that fixes the predicate alone.
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			// Find a quad constructor call.
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ctor := lookupCtor(pass, call)
			if ctor == nil {
				return true
			}
			if argValue(pass, call, ctor.Subj) == argEmpty &&
				argValue(pass, call, ctor.Obj) == argEmpty &&
				argValue(pass, call, ctor.Pred) != argEmpty {
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					Message: "graph quad fixes only the predicate, so a lookup with it scans every quad with that predicate in the World; link each member to an index object and look up by the index object",
				})
			}
			return true
		})
	}
	return nil, nil
}

// lookupCtor returns the quad constructor that call calls, or nil.
func lookupCtor(pass *analysis.Pass, call *ast.CallExpr) *quadCtor {
	// Resolve the called function.
	var id *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		id = fun
	case *ast.SelectorExpr:
		id = fun.Sel
	default:
		return nil
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return nil
	}
	if fn.Pkg().Path() == worldPkg && baseCtors[fn.Name()] {
		return &quadCtor{Subj: 0, Pred: 1, Obj: 2}
	}
	fact := new(quadCtor)
	if pass.ImportObjectFact(fn, fact) {
		return fact
	}
	return nil
}

// paramIndexes maps fd's parameter objects to their indexes.
func paramIndexes(pass *analysis.Pass, fd *ast.FuncDecl) map[types.Object]int {
	params := map[types.Object]int{}
	i := 0
	for _, field := range fd.Type.Params.List {
		for _, name := range field.Names {
			params[pass.TypesInfo.Defs[name]] = i
			i++
		}
		if len(field.Names) == 0 {
			i++
		}
	}
	return params
}

// forward returns where the wrapper's argument at src comes from: one of
// its parameters, an empty string, or another value.
func forward(pass *analysis.Pass, call *ast.CallExpr, src int, params map[types.Object]int) int {
	if src < 0 {
		return src
	}
	if src >= len(call.Args) {
		return argSet
	}
	if id, ok := call.Args[src].(*ast.Ident); ok {
		if i, ok := params[pass.TypesInfo.Uses[id]]; ok {
			return i
		}
	}
	return argValue(pass, call, src)
}

// argValue returns argEmpty when the argument at src is a constant empty
// string, and argSet otherwise.
func argValue(pass *analysis.Pass, call *ast.CallExpr, src int) int {
	// A wrapper that passes an empty string has no argument to read.
	if src == argEmpty {
		return argEmpty
	}
	if src < 0 || src >= len(call.Args) {
		return argSet
	}
	tv, ok := pass.TypesInfo.Types[call.Args[src]]
	if ok && tv.Value != nil && tv.Value.Kind() == constant.String && constant.StringVal(tv.Value) == "" {
		return argEmpty
	}
	return argSet
}
