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

	// childOnce guards childErr and inherited descriptor closure.
	childOnce sync.Once
	// childErr records inherited descriptor close failures.
	childErr error
	// once guards closeErr and endpoint shutdown.
	once sync.Once
	// closeErr records the joined shutdown result.
	closeErr error
}

// closeChildFiles closes inherited child descriptors exactly once.
func (e *EndpointSet) closeChildFiles() error {
	if e == nil {
		return nil
	}
	e.childOnce.Do(func() {
		for _, child := range []*os.File{e.Resource, e.State, e.Control} {
			if child != nil {
				e.childErr = errors.Join(e.childErr, child.Close())
			}
		}
	})
	return e.childErr
}

// closeAndWait closes endpoint transports and joins their servers once.
func (e *EndpointSet) closeAndWait() error {
	if e == nil {
		return nil
	}
	e.once.Do(func() {
		childErr := e.closeChildFiles()
		var closeErr, waitErr error
		if e.CloseFunc != nil {
			closeErr = e.CloseFunc()
		}
		if e.WaitFunc != nil {
			waitErr = e.WaitFunc()
		}
		e.closeErr = errors.Join(childErr, closeErr, waitErr)
	})
	return e.closeErr
}
