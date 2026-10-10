//go:build !js && !darwin && !linux && !windows

package bldr_project_starlark

import "github.com/pkg/errors"

// limitMemory fails: this platform has no memory limit the child can set, and
// an unbounded evaluation of an untrusted project is not offered.
func limitMemory(uint64) error {
	return errors.New("memory limits are not supported on this platform")
}
