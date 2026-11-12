//go:build !darwin && !linux && !js

package devtool_store

import "errors"

// cloneFile reports that this platform has no clone, so copyFile copies bytes.
func cloneFile(src, dst string) error {
	return errors.ErrUnsupported
}
