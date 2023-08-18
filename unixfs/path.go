package unixfs

import (
	"path"
	"strings"
)

// PathSeparator is the universally used path separator.
const PathSeparator = '/'

// SplitPath splits a path string.
// Absolute paths are ignored (converted to relative paths starting at ./).
// Returns if the path was absolute or relative.
func SplitPath(tpath string) (out []string, isAbsolute bool) {
	tpath = path.Clean(tpath)
	if len(tpath) != 0 && tpath[0] == PathSeparator {
		isAbsolute = true
		tpath = tpath[1:]
	}
	if len(tpath) == 0 {
		return nil, isAbsolute
	}
	out = strings.Split(tpath, string([]rune{PathSeparator}))
	if len(out) == 1 && out[0] == "." {
		out = nil
	}
	return out, isAbsolute
}

// JoinPath joins a list of path components to a path.
func JoinPath(pathc []string, isAbsolute bool) string {
	p := strings.Join(pathc, string([]rune{PathSeparator}))
	if isAbsolute {
		p = string([]rune{PathSeparator}) + p
	}
	return path.Clean(p)
}
