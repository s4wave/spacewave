//go:build !js

// Package callpath finds static call paths between Go functions.
//
// It builds a class hierarchy analysis call graph, which counts every method
// of a matching type as a possible callee of an interface call. The paths are
// therefore a superset of the real ones: a path found may never run, but a call
// path missing from the output does not exist in the loaded code.
package callpath

import (
	"regexp"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Options configures a call path search.
type Options struct {
	// Dir is the directory the package patterns resolve from.
	Dir string
	// Patterns are the package patterns to load, as for go build.
	Patterns []string
	// Tests also loads the test files of the matched packages.
	Tests bool
	// From matches the full SSA names of the functions the paths start from.
	From *regexp.Regexp
	// To matches the full SSA names of the functions the paths end at.
	To *regexp.Regexp
	// Within matches the package paths a path may pass through. A target may
	// lie outside them. Nil keeps a path inside the modules of the loaded
	// packages.
	Within *regexp.Regexp
}

// Path is one call path, from its root to its target, as full SSA names.
type Path []string

// Result is the outcome of a search.
type Result struct {
	// Modules are the module paths of the loaded packages.
	Modules []string
	// Roots is the number of functions From matched.
	Roots int
	// Paths are the shortest path from each root to each target it reaches,
	// sorted.
	Paths []Path
}

// Find loads the packages and returns the call paths from each root to each
// target it reaches.
func Find(opts Options) (*Result, error) {
	// Both ends of a path are required.
	if opts.From == nil || opts.To == nil {
		return nil, errors.New("from and to patterns are required")
	}

	// Load the packages and build the call graph.
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule, Dir: opts.Dir, Tests: opts.Tests}
	pkgs, err := packages.Load(cfg, opts.Patterns...)
	if err != nil {
		return nil, err
	}
	if n := packages.PrintErrors(pkgs); n != 0 {
		return nil, errors.Errorf("%d errors loading packages", n)
	}
	if len(pkgs) == 0 {
		return nil, errors.New("no packages matched")
	}
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	cg := cha.CallGraph(prog)

	// Decide which packages a path may pass through.
	res := &Result{Modules: loadedModules(pkgs)}
	within := func(f *ssa.Function) bool {
		// A synthetic wrapper has no package. Pass through it to its callees.
		pkg := funcPkgPath(f)
		if pkg == "" {
			return f.Synthetic != ""
		}
		if opts.Within != nil {
			return opts.Within.MatchString(pkg)
		}
		for _, mod := range res.Modules {
			if pkg == mod || strings.HasPrefix(pkg, mod+"/") {
				return true
			}
		}
		return false
	}

	// Collect the roots in name order so the output is stable. A synthetic
	// wrapper only repeats the path of the method it wraps.
	var roots []*callgraph.Node
	for f, n := range cg.Nodes {
		if f != nil && f.Synthetic == "" && within(f) && opts.From.MatchString(f.String()) {
			roots = append(roots, n)
		}
	}
	slices.SortFunc(roots, func(a, b *callgraph.Node) int {
		return strings.Compare(a.Func.String(), b.Func.String())
	})
	res.Roots = len(roots)

	// Search breadth first from each root for its shortest path to each target.
	for _, root := range roots {
		res.Paths = append(res.Paths, searchRoot(root, opts.To, within)...)
	}
	slices.SortFunc(res.Paths, func(a, b Path) int {
		return slices.Compare(a, b)
	})
	res.Paths = slices.CompactFunc(res.Paths, func(a, b Path) bool {
		return slices.Equal(a, b)
	})
	return res, nil
}

// searchRoot returns the shortest path from root to each target it reaches.
// The search continues only through functions within allows. A target ends
// its path.
func searchRoot(root *callgraph.Node, to *regexp.Regexp, within func(*ssa.Function) bool) []Path {
	// Walk breadth first, recording how each node was first reached.
	var paths []Path
	prev := map[*callgraph.Node]*callgraph.Node{root: nil}
	queue := []*callgraph.Node{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range cur.Out {
			callee := e.Callee
			if callee.Func == nil {
				continue
			}
			if _, seen := prev[callee]; seen {
				continue
			}
			isTarget := to.MatchString(callee.Func.String())
			if !isTarget && !within(callee.Func) {
				continue
			}
			prev[callee] = cur
			if isTarget {
				paths = append(paths, pathTo(prev, callee))
				continue
			}
			queue = append(queue, callee)
		}
	}
	return paths
}

// pathTo walks prev back from n to the root.
func pathTo(prev map[*callgraph.Node]*callgraph.Node, n *callgraph.Node) Path {
	var path Path
	for p := n; p != nil; p = prev[p] {
		path = append(path, p.Func.String())
	}
	slices.Reverse(path)
	return path
}

// funcPkgPath returns the package path of f, following an instantiation or
// closure to the function it came from.
func funcPkgPath(f *ssa.Function) string {
	for f != nil {
		if f.Pkg != nil {
			return f.Pkg.Pkg.Path()
		}
		if o := f.Origin(); o != nil && o != f {
			f = o
			continue
		}
		f = f.Parent()
	}
	return ""
}

// loadedModules returns the sorted module paths of the loaded packages.
func loadedModules(pkgs []*packages.Package) []string {
	var mods []string
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			mods = append(mods, pkg.Module.Path)
		}
	}
	slices.Sort(mods)
	return slices.Compact(mods)
}
