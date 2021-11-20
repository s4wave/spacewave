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
	out := strings.Split(tpath, string([]rune{PathSeparator}))
	if len(out) == 1 && out[0] == "." {
		out = nil
	}
	return out
}

// JoinPath joins a list of path components to a path.
func JoinPath(pathc []string) string {
	return path.Clean(strings.Join(pathc, string([]rune{PathSeparator})))
}
