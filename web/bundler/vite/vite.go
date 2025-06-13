package bldr_web_bundler_vite

import (
	"slices"
	"strings"
)

// SortViteOutputMetas sorts and compacts a list of esbuild output meta.
func SortViteOutputMetas(metas []*ViteOutputMeta) []*ViteOutputMeta {
	slices.SortFunc(metas, func(a, b *ViteOutputMeta) int {
		return strings.Compare(a.GetPath(), b.GetPath())
	})
	return slices.CompactFunc(metas, func(a, b *ViteOutputMeta) bool {
		return a.EqualVT(b)
	})
}
