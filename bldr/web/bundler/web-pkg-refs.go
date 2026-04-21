package bldr_web_bundler

import (
	"slices"
	"strings"
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

// ExcludedWebPkgIDs returns the set of web pkg IDs that have exclude=true.
func ExcludedWebPkgIDs(refs []*WebPkgRefConfig) map[string]struct{} {
	out := make(map[string]struct{})
	for _, ref := range refs {
		if ref.GetExclude() {
			out[ref.GetId()] = struct{}{}
		}
	}
	return out
}

// SortWebPkgRefConfigs sorts the list of ref configs by web pkg id.
func SortWebPkgRefConfigs(refs []*WebPkgRefConfig) {
	slices.SortStableFunc(refs, func(a, b *WebPkgRefConfig) int {
		return strings.Compare(a.GetId(), b.GetId())
	})
}

// CompactWebPkgRefConfigs compacts the list of ref configs by web pkg id.
// Merges together entries with the same ID in-place.
func CompactWebPkgRefConfigs(refs []*WebPkgRefConfig) []*WebPkgRefConfig {
	if len(refs) <= 1 {
		return refs
	}

	// Sort by ID first to ensure we process duplicates together
	SortWebPkgRefConfigs(refs)

	// Iterate through the sorted slice, merging duplicates
	writeIdx := 0
	for readIdx := range refs {
		// Skip empty IDs
		if refs[readIdx].GetId() == "" {
			continue
		}

		// If this is the first element or has a different ID than the previous one
		if writeIdx == 0 || refs[writeIdx-1].GetId() != refs[readIdx].GetId() {
			// If we're not at the same position, copy the current element
			if writeIdx != readIdx {
				refs[writeIdx] = refs[readIdx]
			}
			writeIdx++
		} else {
			// Same ID as previous element, merge with it
			prev := refs[writeIdx-1]
			curr := refs[readIdx]

			// Merge exclude flag
			if curr.GetExclude() {
				prev.Exclude = true
			}

			// Merge imports
			for _, imp := range curr.GetImports() {
				if !slices.Contains(prev.Imports, imp) {
					prev.Imports = append(prev.Imports, imp)
				}
			}

			// Sort imports for consistency
			slices.Sort(prev.Imports)
		}
	}

	// Return the compacted slice
	return refs[:writeIdx]
}
