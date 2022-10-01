package plugin_compiler

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// dotSlash is ./
var dotSlash = string([]rune{'.', os.PathSeparator})

// parseGoModFile parses a go.mod file at a path.
func parseGoModFile(srcPath string) (*modfile.File, error) {
	srcGoMod, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}

	// Adjust the module path by adding a prefix.
	return modfile.Parse(srcPath, srcGoMod, nil)
}

// relocateGoModFile transforms all references to a new path.
//
// expects the modfile to have been parsed with an absolute path.
func relocateGoModFile(
	modf *modfile.File,
	nextModPath string,
) error {
	prevGoModPath := modf.Syntax.Name
	// has no effect if the path is already absolute.
	modPathAbs, err := filepath.Abs(path.Dir(prevGoModPath))
	if err != nil {
		return err
	}
	nextGoModDir := path.Dir(nextModPath)
	codegenModDir, err := filepath.Abs(nextGoModDir)
	if err != nil {
		return err
	}

	var adjOps [](func() error)
	for _, srcReplace := range modf.Replace {
		newPath := srcReplace.New.Path
		// if the path replacement has a ./ or ../ prefix, update the relative path
		if strings.HasPrefix(newPath, "./") || strings.HasPrefix(newPath, "../") {
			// join the absolute path to the module with the relative replacement
			// prevNewPathAbs is the absolute path to the replacement module.
			prevNewPathAbs := filepath.Join(modPathAbs, newPath)
			// newPathRelative is the relative path from the new module to the replacement.
			newPathRelative, err := filepath.Rel(codegenModDir, prevNewPathAbs)
			if err != nil {
				return err
			}
			// ensure starts with ./
			newPathRelative = ensureStartsWithDotSlash(newPathRelative)
			// add a new replacement to override the old
			oldSrcReplacePath := srcReplace.Old.Path
			oldSrcReplaceVersion := srcReplace.Old.Version
			oldSrcReplaceNewVersion := srcReplace.New.Version
			adjOps = append(adjOps, func() error {
				return modf.AddReplace(
					oldSrcReplacePath,
					oldSrcReplaceVersion,
					newPathRelative,
					oldSrcReplaceNewVersion,
				)
			})
		}
	}

	for _, op := range adjOps {
		if err := op(); err != nil {
			return err
		}
	}

	modf.Cleanup()
	return nil
}

// ensureStartsWithDotSlash returns the string with a ./ or ../ prefix
func ensureStartsWithDotSlash(p string) string {
	// ensure starts with ./ or ../ - check simply for '.'
	if !strings.HasPrefix(p, dotSlash[:1]) {
		p = dotSlash + p
	}
	return p
}
