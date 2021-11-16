package unixfs

import (
	"path"
	"path/filepath"
	"strings"
)

// SplitPath splits a path string.
func SplitPath(tpath string) []string {
	tpath = path.Clean(tpath)
	return strings.Split(tpath, string([]rune{filepath.Separator}))
}

// JoinPath joins a list of path components to a path.
func JoinPath(pathc []string) string {
	return path.Clean(strings.Join(pathc, string([]rune{filepath.Separator})))
}
