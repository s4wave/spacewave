package bldr_plugin_compiler_vardef

import (
	"bytes"
	"go/printer"
	"go/token"
	"testing"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
)

type testcase struct {
	pluginVar          *PluginVar
	expectedDevInfoRef string
	expectedGoValue    string
}

var testcases = []*testcase{{
	pluginVar: &PluginVar{
		PkgImportPath: "test/package",
		PkgVar:        "TestVariable",
		Body:          &PluginVar_StringValue{StringValue: "test-value"},
	},
	expectedDevInfoRef: `devInfo.LookupPluginDevVar("test/package", "TestVariable").GetStringValue()`,
	expectedGoValue:    `"test-value"`,
}, {
	pluginVar: &PluginVar{
		PkgImportPath: "other/package",
		PkgVar:        "OtherTestVar",
		Body: &PluginVar_EsbuildOutput{EsbuildOutput: &bldr_esbuild.EsbuildOutput{
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
		exp, err := tc.pluginVar.ToGoDevInfoRefAst(devInfoVarName)
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
		exp, err := tc.pluginVar.ToGoValueAst()
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
