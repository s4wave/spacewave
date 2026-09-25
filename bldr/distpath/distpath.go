// Package distpath resolves Bldr dist source paths. It stays free of the
// embedded dist sources so a program that only needs the layout, such as a
// GoScript plugin, does not carry them.
package distpath

import (
	"os"
	"path/filepath"
)

// Resolve resolves a dist source file in a Bldr root or in a Spacewave root
// that nests Bldr under bldr/. The nested path wins because a Spacewave root
// has its own entries, such as sdk/, that shadow Bldr names.
func Resolve(distSourcePath string, elems ...string) string {
	candidates := [][]string{
		append([]string{"bldr"}, elems...),
		elems,
	}
	for _, c := range candidates {
		p := filepath.Join(append([]string{distSourcePath}, c...)...)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(append([]string{distSourcePath}, elems...)...)
}
