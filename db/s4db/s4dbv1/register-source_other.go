//go:build !darwin && !linux && !windows

package s4dbv1

import "os"

// openRegisteredSource opens a source on platforms without native file sharing.
func openRegisteredSource(path string) (*os.File, func() error, error) {
	// A device-backed runtime has no native multi-process descriptor registry.
	f, err := openSource(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
