//go:build !linux && !darwin && !windows && !js

package checkoutroot

import "os"

// renameExclusive uses an atomic link where no native rename is available.
func renameExclusive(dir *os.Root, temp, name string) error {
	return dir.Link(temp, name)
}
