// Package statepath owns cleanup policy for Bldr state roots.
package statepath

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// ClearBuildState removes Bldr-owned transient entries from root. When
// preserveStartupBuildCache is false it also removes the durable startup build
// cache. Unrecognized entries and state-root lock anchors are always preserved.
func ClearBuildState(root string, preserveStartupBuildCache bool) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "read state path")
	}
	for _, entry := range entries {
		name := entry.Name()
		remove := false
		switch name {
		case "logs", "src", "plugin", "cli":
			remove = true
		}
		if !preserveStartupBuildCache &&
			(name == "build" || strings.HasPrefix(name, "devtool.db") || strings.HasPrefix(name, "devtool.s4wave")) {
			remove = true
		}
		if !remove {
			continue
		}
		path := filepath.Join(root, name)
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "remove state path %s", path)
		}
	}
	return nil
}
