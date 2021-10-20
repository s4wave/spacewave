package unixfs

import (
	"path"
	"strings"
)

// PathSeparator is the universally used path separator.
const PathSeparator = '/'

// SplitPath splits a path string.
func SplitPath(tpath string) []string {
	tpath = path.Clean(tpath)
	return strings.Split(tpath, string([]rune{PathSeparator}))
}

// JoinPath joins a list of path components to a path.
func JoinPath(pathc []string) string {
	return path.Clean(strings.Join(pathc, string([]rune{PathSeparator})))
}
