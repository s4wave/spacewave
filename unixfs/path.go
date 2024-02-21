package unixfs

import (
	"io/fs"
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

// JoinPathPts joins multiple path parts slices.
// (concats the slices together)
func JoinPathPts(pts ...[]string) []string {
	if len(pts) == 0 {
		return nil
	}
	out := make([]string, 0, len(pts)*len(pts[0]))
	for _, pti := range pts {
		out = append(out, pti...)
	}
	return out
}

// CleanSplitValidatePath cleans a path, splits it, and validates it.
func CleanSplitValidatePath(filePath string) (pathPts []string, isAbsolute bool, err error) {
	filePath = path.Clean(filePath)
	if filePath == "/" || filePath == "." {
		filePath = ""
	}
	if filePath != "" && filePath[0] == PathSeparator {
		filePath = filePath[1:]
	}
	if filePath != "" && !fs.ValidPath(filePath) {
		return nil, false, fs.ErrInvalid
	}

	pathPts, isAbsolute = SplitPath(filePath)
	return pathPts, isAbsolute, nil
}
