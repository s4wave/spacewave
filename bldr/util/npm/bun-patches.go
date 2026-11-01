package npm

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// ParseBunPatchPaths returns the sorted, distinct paths whose bytes Bun applies to
// installed dependencies. Paths stay inside the copied install seed, so a
// cache rebuild cannot overwrite a source file or another install's contents.
func ParseBunPatchPaths(packageJSON []byte) ([]string, error) {
	// Read the manifest's patch mapping without resolving or modifying project files.
	manifest, err := fastjson.ParseBytes(packageJSON)
	if err != nil {
		return nil, err
	}
	patches := manifest.GetObject("patchedDependencies")
	if patches == nil {
		return nil, nil
	}

	// Validate destinations before the shared installer creates or clears its cache directory.
	var paths []string
	patches.Visit(func(_ []byte, value *fastjson.Value) {
		// Keep each declared patch separate from the install metadata and generated packages.
		if err != nil {
			return
		}
		name := string(value.GetStringBytes())
		if !filepath.IsLocal(name) {
			err = errors.Errorf("Bun patch path %q must stay inside its dependency manifest directory", name)
			return
		}
		name = filepath.Clean(name)
		reserved := strings.ToLower(name)
		if reserved == "package.json" || reserved == "bun.lock" || reserved == installHashFile || reserved == "node_modules" || strings.HasPrefix(reserved, "node_modules"+string(filepath.Separator)) {
			err = errors.Errorf("Bun patch path %q overlaps an install-owned file", name)
			return
		}
		paths = append(paths, name)
	})
	if err != nil {
		return nil, err
	}

	// A patch shared by several packages is still one source input.
	slices.Sort(paths)
	return slices.Compact(paths), nil
}
