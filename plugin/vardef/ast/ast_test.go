package bldr_plugin_vardef_ast

import (
	"bytes"
	"go/printer"
	"go/token"
	"testing"

	bldr_plugin_vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
)

type testcase struct {
	pluginVar          *bldr_plugin_vardef.PluginVar
	expectedDevInfoRef string
	expectedGoValue    string
}

var testcases = []*testcase{{
	pluginVar: &bldr_plugin_vardef.PluginVar{
		PkgImportPath: "test/package",
		PkgVar:        "TestVariable",
		Body:          &bldr_plugin_vardef.PluginVar_StringValue{StringValue: "test-value"},
	},
	expectedDevInfoRef: `devInfo.LookupPluginDevVar("test/package", "TestVariable").GetStringValue()`,
	expectedGoValue:    `"test-value"`,
}, {
	pluginVar: &bldr_plugin_vardef.PluginVar{
		PkgImportPath: "other/package",
		PkgVar:        "OtherTestVar",
		Body: &bldr_plugin_vardef.PluginVar_EsbuildOutput{EsbuildOutput: &bldr_esbuild.EsbuildOutput{
			EntrypointHref: "/p/plugin/entrypoint.js",
			CssHref:        "/p/plugin/entrypoint.css",
		}},
	},
	expectedDevInfoRef: `devInfo.LookupPluginDevVar("other/package", "OtherTestVar").GetEsbuildOutputValue()`,
	expectedGoValue:    `bldr_values.EsbuildOutput{EntrypointHref: "/p/plugin/entrypoint.js", CssHref: "/p/plugin/entrypoint.css"}`,
}}

var devInfoVarName = "devInfo"

// TestToGoDevInfoRefAst tests building the dev info ref ast.
func TestToGoDevInfoRefAst(t *testing.T) {
	for _, tc := range testcases {
		exp, err := ToGoDevInfoRefAst(tc.pluginVar, devInfoVarName)
		if err != nil {
			t.Fatal(err.Error())
		}
		fset := token.NewFileSet()
		var buf bytes.Buffer
		err = printer.Fprint(&buf, fset, exp)
		if err != nil {
			t.Fatal(err.Error())
		}
		genExpr := buf.String()
		t.Log(genExpr)
		if genExpr != tc.expectedDevInfoRef {
			t.Fatalf("expected: %s", tc.expectedDevInfoRef)
		}
	}
}

// TestToGoValueAst tests building the go value ast.
func TestToGoValueAst(t *testing.T) {
	for _, tc := range testcases {
		exp, err := ToGoValueAst(tc.pluginVar)
		if err != nil {
			t.Fatal(err.Error())
		}
		fset := token.NewFileSet()
		var buf bytes.Buffer
		err = printer.Fprint(&buf, fset, exp)
		if err != nil {
			t.Fatal(err.Error())
		}
		genExpr := buf.String()
		t.Log(genExpr)
		if genExpr != tc.expectedGoValue {
			t.Fatalf("expected: %s", tc.expectedDevInfoRef)
		}
	}
}
