package web_pkg

import (
	"slices"
	"strings"
)

// WebPkgRefSlice is a slice of WebPkgRef.
type WebPkgRefSlice []*WebPkgRef

// AppendWebPkgRef appends a web pkg ref to the slice.
// Merges with any existing definition for that web pkg id.
//
// Returns true if any changes were made.
func (sl WebPkgRefSlice) AppendWebPkgRef(webPkgID, webPkgRoot, importPath string) (WebPkgRefSlice, bool) {
	// check if the ref already exists
	var ref *WebPkgRef
	var dirty bool
	for _, sref := range sl {
		if sref.WebPkgId == webPkgID {
			ref = sref
			break
		}
	}

	if ref == nil {
		ref = &WebPkgRef{
			WebPkgId:   webPkgID,
			WebPkgRoot: webPkgRoot,
			Imports:    []string{importPath},
		}
		sl = append(sl, ref)
		slices.SortFunc(sl, func(a *WebPkgRef, b *WebPkgRef) int {
			return strings.Compare(a.WebPkgId, b.WebPkgId)
		})
		dirty = true
	} else if !slices.Contains(ref.Imports, importPath) {
		ref.Imports = append(ref.Imports, importPath)
		slices.Sort(ref.Imports)
		dirty = true
	}

	return sl, dirty
}

// SortWebPkgRefs sorts the list of refs by web pkg id.
func SortWebPkgRefs(refs []*WebPkgRef) {
	slices.SortStableFunc(refs, func(a, b *WebPkgRef) int {
		return strings.Compare(a.WebPkgId, b.WebPkgId)
	})
}

// FindWebPkgRef finds the web pkg ref with the given web pkg id.
//
// Returns the index or -1 if not found.
func FindWebPkgRef(sl []*WebPkgRef, webPkgID string) (*WebPkgRef, int) {
	for i, v := range sl {
		if v.WebPkgId == webPkgID {
			return v, i
		}
	}
	return nil, -1
}

// ToWebPkgIDList returns a sorted, deduplicated list of web pkg ids from the slice.
func (sl WebPkgRefSlice) ToWebPkgIDList() []string {
	if len(sl) == 0 {
		return nil
	}

	// Extract all web pkg ids
	ids := make([]string, 0, len(sl))
	for _, ref := range sl {
		if webPkgID := ref.GetWebPkgId(); webPkgID != "" {
			ids = append(ids, webPkgID)
		}
	}

	// sort the ids
	slices.Sort(ids)

	// remove duplicates
	return slices.Compact(ids)
}
