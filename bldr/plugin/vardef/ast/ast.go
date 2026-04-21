package bldr_plugin_vardef_ast

import (
	gast "go/ast"
	"go/token"
	"strconv"

	bldr_plugin_vardef "github.com/s4wave/spacewave/bldr/plugin/vardef"
	"github.com/pkg/errors"
)

// ToGoDevInfoRefAst builds the Go ast for referencing the value on the dev info object.
func ToGoDevInfoRefAst(v *bldr_plugin_vardef.PluginVar, devInfoVarName string) (gast.Expr, error) {
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
	case *bldr_plugin_vardef.PluginVar_StringValue:
		return &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   pluginVarVal,
				Sel: &gast.Ident{Name: "GetStringValue"},
			},
		}, nil
	case *bldr_plugin_vardef.PluginVar_WebBundlerOutput:
		return &gast.CallExpr{
			Fun: &gast.SelectorExpr{
				X:   pluginVarVal,
				Sel: &gast.Ident{Name: "GetWebBundlerOutputValue"},
			},
		}, nil
	default:
		return nil, errors.Errorf("unexpected plugin var type: %s.%s", v.GetPkgImportPath(), v.GetPkgVar())
	}
}

// ToGoValueAst builds the Go ast for the value of the plugin variable.
func ToGoValueAst(v *bldr_plugin_vardef.PluginVar) (gast.Expr, error) {
	buildStringLit := func(lit string) *gast.BasicLit {
		return &gast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(lit),
		}
	}

	// varValue is the value for the go variable.
	switch val := v.GetBody().(type) {
	case *bldr_plugin_vardef.PluginVar_StringValue:
		return buildStringLit(val.StringValue), nil
	case *bldr_plugin_vardef.PluginVar_WebBundlerOutput:
		elts := make([]gast.Expr, 0, 2)
		output := val.WebBundlerOutput
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
				Sel: gast.NewIdent("WebBundlerOutput"),
				X:   gast.NewIdent("bldr_values"),
			},
		}, nil
	default:
		return nil, errors.Errorf("unknown target variable type: %s.%s", v.GetPkgImportPath(), v.GetPkgVar())
	}
}
