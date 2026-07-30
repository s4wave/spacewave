//go:build !darwin && !linux && !js && !windows

package bldr_buildbudget

import "errors"

func availableHostMemoryBytes() (uint64, error) {
	return 0, errors.New("host memory detection is unsupported on this platform")
}
