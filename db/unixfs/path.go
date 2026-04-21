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
	// check for ./
	if len(tpath) >= 2 && tpath[0] == '.' && tpath[1] == PathSeparator {
		tpath = tpath[2:] // Is this even possible with path.Clean?
	}
	if len(tpath) == 0 || (len(tpath) == 1 && tpath[0] == '.') {
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
	cleanPath := path.Clean(p)
	if !isAbsolute && cleanPath[0] == '/' {
		if len(cleanPath) == 1 {
			cleanPath = "."
		} else {
			cleanPath = cleanPath[1:]
		}
	}
	return cleanPath
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

// CleanSplitValidateRelativePath cleans a path, splits it, and validates it.
// Coerces the path to be a relative path, not absolute.
func CleanSplitValidateRelativePath(filePath string) (pathPts []string, err error) {
	filePath = path.Clean(filePath)
	if filePath == "/" || filePath == "." {
		filePath = ""
	}
	if filePath != "" && filePath[0] == PathSeparator {
		filePath = filePath[1:]
	}
	if filePath != "" && !fs.ValidPath(filePath) {
		return nil, fs.ErrInvalid
	}

	pathPts, _ = SplitPath(filePath)
	return pathPts, nil
}
