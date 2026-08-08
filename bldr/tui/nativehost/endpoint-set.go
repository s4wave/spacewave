//go:build !js && !windows

package nativehost

import (
	"errors"
	"os"
	"sync"
)

// EndpointSet contains the three endpoint descriptors inherited as fd 5..7.
type EndpointSet struct {
	// resource is the resource endpoint.
	Resource *os.File
	// state is the state endpoint.
	State *os.File
	// control is the control endpoint.
	Control *os.File
	// closeFunc closes the endpoint transport.
	CloseFunc func() error
	// waitFunc waits for the endpoint transport.
	WaitFunc func() error
	once     sync.Once
	closeErr error
}

func (e *EndpointSet) closeAndWait() error {
	if e == nil {
		return nil
	}
	e.once.Do(func() {
		var closeErr, waitErr error
		if e.CloseFunc != nil {
			closeErr = e.CloseFunc()
		}
		if e.WaitFunc != nil {
			waitErr = e.WaitFunc()
		}
		e.closeErr = errors.Join(closeErr, waitErr)
	})
	return e.closeErr
}
