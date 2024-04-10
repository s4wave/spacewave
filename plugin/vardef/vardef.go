package bldr_plugin_vardef

import (
	"strings"

	bldr_esbuild "github.com/aperturerobotics/bldr/web/esbuild"
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
