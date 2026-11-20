//go:build !js

package unixfs_billy

import (
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
)

// isNativeOSFS reports whether the backend opens independent OS directory
// handles and supports concurrent reads.
func isNativeOSFS(bfs billy.Basic) bool {
	switch bfs.(type) {
	case *osfs.BoundOS, *osfs.RootOS:
		return true
	default:
		return false
	}
}
