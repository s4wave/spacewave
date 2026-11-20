package unixfs_billy

import "github.com/go-git/go-billy/v6"

// isNativeOSFS reports false because osfs uses memfs on this platform.
func isNativeOSFS(bfs billy.Basic) bool {
	return false
}
