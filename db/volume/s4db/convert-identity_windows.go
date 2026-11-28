package volume_s4db

import "os"

// fileIdentity refuses on Windows, where the conversion does not run.
func fileIdentity(os.FileInfo) (string, error) {
	return "", ErrConvertUnsupported
}

// linkCount refuses on Windows, where the conversion does not run.
func linkCount(os.FileInfo) (uint64, error) {
	return 0, ErrConvertUnsupported
}
