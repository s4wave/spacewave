package bldr_plugin_compiler

import (
	"strings"

	"golang.org/x/exp/slices"
)

// WebPkgRefConfigSlice is a slice of WebPkgRefConfig.
type WebPkgRefConfigSlice []*WebPkgRefConfig

// AppendWebPkgRef appends a web pkg ref to the slice.
// Merges with any existing definition for that web pkg id.
//
// Returns true if any changes were made.
func (sl WebPkgRefConfigSlice) AppendWebPkgRefConfig(addConf *WebPkgRefConfig) (WebPkgRefConfigSlice, bool) {
	if addConf.GetId() == "" {
		return sl, false
	}

	// check if the ref already exists
	var ref *WebPkgRefConfig
	var dirty bool
	for _, sref := range sl {
		if sref.GetId() == addConf.GetId() {
			ref = sref
			break
		}
	}

	if ref == nil {
		sl = append(sl, addConf.CloneVT())
		slices.SortFunc(sl, func(a, b *WebPkgRefConfig) int {
			return strings.Compare(a.GetId(), b.GetId())
		})
		dirty = true
	} else {
		if addConf.GetExclude() && !ref.GetExclude() {
			dirty = true
			ref.Exclude = true
		}
		for _, addImport := range addConf.GetImports() {
			if !slices.Contains(ref.Imports, addImport) {
				dirty = true
				ref.Imports = append(ref.Imports, addImport)
			}
		}
		if dirty {
			slices.Sort(ref.Imports)
		}
	}

	return sl, dirty
}

// ToIdList converts the list to the esbuild externalize list.
func (sl WebPkgRefConfigSlice) ToIdList() []string {
	out := make([]string, 0, len(sl))
	for _, v := range sl {
		out = append(out, v.GetId())
	}
	slices.Sort(out)
	out = slices.Compact(out)
	return out
}

// SortWebPkgRefConfigs sorts the list of ref configs by web pkg id.
func SortWebPkgRefConfigs(refs []*WebPkgRefConfig) {
	slices.SortStableFunc(refs, func(a, b *WebPkgRefConfig) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
}
