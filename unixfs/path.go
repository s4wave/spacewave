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
	if len(tpath) >= 1 && tpath[0] == PathSeparator {
		isAbsolute = true
		tpath = tpath[1:]
	}
	if len(tpath) >= 2 && tpath[0] == '.' && tpath[1] == PathSeparator {
		tpath = tpath[2:]
	}
	if len(tpath) == 0 {
		return nil, isAbsolute
	}
	return strings.Split(tpath, string([]rune{PathSeparator})), isAbsolute
}

// JoinPath joins a list of path components to a path.
func JoinPath(pathc []string, isAbsolute bool) string {
	p := strings.Join(pathc, string([]rune{PathSeparator}))
	if isAbsolute {
		p = string([]rune{PathSeparator}) + p
	}
	return path.Clean(p)
}
