//go:build !js && !windows

package nativehost

import (
	"errors"
	"os"
	"sync"
)

// EndpointSet contains the three endpoint descriptors inherited as fd 5..7.
type EndpointSet struct {
	// Resource is the resource endpoint.
	Resource *os.File
	// State is the state endpoint.
	State *os.File
	// Control is the control endpoint.
	Control *os.File
	// CloseFunc closes the endpoint transport.
	CloseFunc func() error
	// WaitFunc waits for the endpoint transport.
	WaitFunc func() error

	// once guards closeErr and endpoint shutdown.
	once sync.Once
	// closeErr records the joined shutdown result.
	closeErr error
}

// closeAndWait closes endpoint transports and joins their servers once.
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
