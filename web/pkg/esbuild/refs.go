package web_pkg_esbuild

import (
	"strings"

	"golang.org/x/exp/slices"
)

// WebPkgRef contains information about a reference to a Web pkg that was replaced.
type WebPkgRef struct {
	// WebPkgID is the web package id
	WebPkgID string
	// WebPkgRoot is the path to the web package root dir.
	WebPkgRoot string
	// Imports is the list of paths that were imported.
	Imports []string
}

// AddWebPkgRef adds / deduplicates a web package ref in a slice.
func AddWebPkgRef(sl []*WebPkgRef, webPkgID, webPkgRoot, importPath string) []*WebPkgRef {
	// check if the ref already exists
	var ref *WebPkgRef
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
	} else if !slices.Contains(ref.Imports, importPath) {
		ref.Imports = append(ref.Imports, importPath)
		slices.Sort(ref.Imports)
	}
	return sl
}
