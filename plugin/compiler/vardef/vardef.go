package bldr_plugin_compiler_vardef

import (
	gast "go/ast"
	"go/token"
	"strconv"
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// NewPluginVar constructs a new plugin var.
func NewPluginVar(pkgImportPath, pkgVar string, body isPluginVar_Body) *PluginVar {
	return &PluginVar{PkgImportPath: pkgImportPath, PkgVar: pkgVar, Body: body}
}

// SortPluginVars sorts a slice of plugin vars.
func SortPluginVars(vars []*PluginVar) {
	slices.SortFunc(vars, func(a, b *PluginVar) int {
		return strings.Compare(a.GetVariablePath(), b.GetVariablePath())
	})
}

// LookupPluginDevVar looks up a plugin dev var from the set of vars.
func (v *PluginDevInfo) LookupPluginDevVar(pkgImportPath, varName string) *PluginVar {
	for _, pv := range v.GetPluginVars() {
		if pv.GetPkgVar() == varName && pv.GetPkgImportPath() == pkgImportPath {
			return pv
		}
	}
	return nil
}

// GetVariablePath returns the package/path.VariableName.
func (v *PluginVar) GetVariablePath() string {
	return v.GetPkgImportPath() + "." + v.GetPkgVar()
}

// GetEsbuildOutputValue returns the dereferenced value of EsbuildOutput or empty if unset.
func (v *PluginVar) GetEsbuildOutputValue() bldr_esbuild.EsbuildOutput {
	val := v.GetEsbuildOutput()
	if val == nil {
		return bldr_esbuild.EsbuildOutput{}
	}
	return *(val.CloneVT())
}

// ToGoDevInfoRefAst builds the Go ast for referencing the value on the dev info object.
func (v *PluginVar) ToGoDevInfoRefAst(devInfoVarName string) (gast.Expr, error) {
	pluginVarVal := &gast.CallExpr{
		Fun: &gast.SelectorExpr{
			X:   &gast.Ident{Name: devInfoVarName},
			Sel: &gast.Ident{Name: "LookupPluginDevVar"},
		},
		Args: []gast.Expr{
			&gast.BasicLit{Kind: token.STRING, Value: strconv.Quote(v.GetPkgImportPath())},
			&gast.BasicLit{Kind: token.STRING, Value: strconv.Quote(v.GetPkgVar())},
		},
	}

	switch v.GetBody().(type) {
	case *PluginVar_StringValue:
		return &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   pluginVarVal,
				Sel: &gast.Ident{Name: "GetStringValue"},
			},
		}, nil
	case *PluginVar_EsbuildOutput:
		return &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   pluginVarVal,
				Sel: &gast.Ident{Name: "GetEsbuildOutputValue"},
			},
		}, nil
	default:
		return nil, errors.Errorf("unexpected plugin var type: %s.%s", v.GetPkgImportPath(), v.GetPkgVar())
	}
}

// ToGoValueAst builds the Go ast for the value of the plugin variable.
func (v *PluginVar) ToGoValueAst() (gast.Expr, error) {
	buildStringLit := func(lit string) *gast.BasicLit {
		return &gast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(lit),
		}
	}

	// varValue is the value for the go variable.
	switch val := v.GetBody().(type) {
	case *PluginVar_StringValue:
		return buildStringLit(val.StringValue), nil
	case *PluginVar_EsbuildOutput:
		elts := make([]gast.Expr, 0, 2)
		output := val.EsbuildOutput
		if entrypointHref := output.GetEntrypointHref(); entrypointHref != "" {
			elts = append(elts, &gast.KeyValueExpr{
				Key:   gast.NewIdent("EntrypointHref"),
				Value: buildStringLit(entrypointHref),
			})
		}
		if cssHref := output.GetCssHref(); cssHref != "" {
			elts = append(elts, &gast.KeyValueExpr{
				Key:   gast.NewIdent("CssHref"),
				Value: buildStringLit(cssHref),
			})
		}
		return &gast.CompositeLit{
			Elts: elts,
			Type: &gast.SelectorExpr{
				Sel: gast.NewIdent("EsbuildOutput"),
				X:   gast.NewIdent("bldr_values"),
			},
		}, nil
	default:
		return nil, errors.Errorf("unknown target variable type: %s.%s", v.GetPkgImportPath(), v.GetPkgVar())
	}
}
