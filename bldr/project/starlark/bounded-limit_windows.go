//go:build windows

package bldr_project_starlark

// limitMemory does nothing: the parent starts the child in a Job Object that
// limits its memory.
func limitMemory(uint64) error {
	return nil
}
