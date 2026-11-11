//go:build !darwin && !linux && !windows

package s4dbv1

import "os"

// openRegisteredSource opens a source on platforms without native file sharing.
func openRegisteredSource(path string, opener func(string) (*os.File, error)) (*os.File, func() error, error) {
	// A device-backed runtime has no native multi-process descriptor registry.
	f, err := opener(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
