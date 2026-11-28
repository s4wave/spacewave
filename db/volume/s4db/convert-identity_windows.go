package volume_s4db

import "os"

// fileIdentity refuses on Windows, where the conversion does not run.
func fileIdentity(os.FileInfo) (string, error) {
	return "", ErrConvertUnsupported
}
