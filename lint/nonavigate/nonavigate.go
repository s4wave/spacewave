// Package nonavigate provides a golangci-lint module plugin that forbids
// calls to Harness.Navigate in e2e/wasm test files. Navigate triggers a
// full page reload (Playwright page.Goto) which destroys the WASM process,
// all web workers, and all WebSocket connections. Any test state held in
// the plugin worker (traces, resource mounts) is lost.
package nonavigate

import (
	"go/ast"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("nonavigate", New)
}

// Settings holds optional configuration for the linter.
type Settings struct{}

type plugin struct{}

// New constructs the plugin.
func New(settings any) (register.LinterPlugin, error) {
	return &plugin{}, nil
}

// BuildAnalyzers returns the analysis passes.
func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "nonavigate",
		Doc:  "forbids Harness.Navigate calls that destroy the WASM process",
		Run:  run,
	}}, nil
}

// GetLoadMode returns the load mode (syntax only, no type info needed).
func (p *plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Navigate" {
				pass.Report(analysis.Diagnostic{
					Pos:     sel.Sel.Pos(),
					Message: "Navigate triggers a full page reload destroying the WASM process and all workers; use page.Evaluate with pushState for client-side routing instead",
				})
			}
			return true
		})
	}
	return nil, nil
}
