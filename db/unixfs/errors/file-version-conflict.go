package unixfs_errors

// FileVersionConflict reports a conditional upload that published nothing.
type FileVersionConflict struct {
	// Version is the current file version, absent when the name holds no regular file.
	Version []byte
}

// Error describes the stale publication expectation.
func (e *FileVersionConflict) Error() string {
	return "file changed since its version was read"
}
