//go:build !linux && !darwin && !windows && !js

package checkoutroot

import "os"

// openFileVersion leaves the version unset on hosts without a metadata backend.
func openFileVersion(file nativeFile) ([]byte, error) {
	return nil, nil
}

// destinationVersion has no version that could authorize a conditional replacement.
func destinationVersion(dir *os.Root, name string) ([]byte, error) {
	return nil, nil
}
