package web_pkg_esbuild

import (
	"strings"

	"golang.org/x/exp/slices"
)

// WebPkgRef contains information about a reference to a Web pkg that was replaced.
type WebPkgRef struct {
	// WebPkgID is the web package id.
	WebPkgID string
	// WebPkgRoot is the path to the web package root dir.
	WebPkgRoot string
	// Imports is the list of paths that were imported from the web pkg.
	Imports []string
	// Refs is the list of other web packages this web package references.
	// NOTE: this is not filled until ResolveWebPkgRefsEsbuild is called!
	Refs []string
}

// AddWebPkgRef adds / deduplicates a web package ref in a slice.
//
// Returns if any changes were made.
func AddWebPkgRef(sl []*WebPkgRef, webPkgID, webPkgRoot, importPath string) ([]*WebPkgRef, bool) {
	// check if the ref already exists
	var ref *WebPkgRef
	var dirty bool
	for _, sref := range sl {
		if sref.WebPkgID == webPkgID {
			ref = sref
			break
		}
	}

	if ref == nil {
		ref = &WebPkgRef{
			WebPkgID:   webPkgID,
			WebPkgRoot: webPkgRoot,
			Imports:    []string{importPath},
		}
		sl = append(sl, ref)
		slices.SortFunc(sl, func(a *WebPkgRef, b *WebPkgRef) int {
			return strings.Compare(a.WebPkgID, b.WebPkgID)
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
		return strings.Compare(a.WebPkgID, b.WebPkgID)
	})
}

// FindWebPkgRef finds the web pkg ref with the given web pkg id.
//
// Returns the index or -1 if not found.
func FindWebPkgRef(sl []*WebPkgRef, webPkgID string) (*WebPkgRef, int) {
	for i, v := range sl {
		if v.WebPkgID == webPkgID {
			return v, i
		}
	}
	return nil, -1
}
