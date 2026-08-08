//go:build !js && !windows

package nativehost

import (
	"errors"
	"os"

	"github.com/aperturerobotics/util/broadcast"
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

	// bcast guards every lifecycle field below.
	bcast broadcast.Broadcast
	// childClosing reports inherited descriptor closure in progress.
	childClosing bool
	// childClosed reports inherited descriptor closure completion.
	childClosed bool
	// childErr records inherited descriptor close failures.
	childErr error
	// closing reports endpoint shutdown in progress.
	closing bool
	// closed reports endpoint shutdown completion.
	closed bool
	// closeErr records the joined shutdown result.
	closeErr error
}

// closeChildFiles closes inherited child descriptors exactly once.
func (e *EndpointSet) closeChildFiles() error {
	if e == nil {
		return nil
	}
	for {
		locked := e.bcast.Lock()
		if e.childClosed {
			err := e.childErr
			locked.Unlock()
			return err
		}
		if e.childClosing {
			wait := locked.WaitCh()
			locked.Unlock()
			<-wait
			continue
		}
		e.childClosing = true
		locked.Broadcast()
		locked.Unlock()

		var childErr error
		for _, child := range []*os.File{e.Resource, e.State, e.Control} {
			if child != nil {
				childErr = errors.Join(childErr, child.Close())
			}
		}
		locked = e.bcast.Lock()
		e.childClosing = false
		e.childClosed = true
		e.childErr = childErr
		locked.Broadcast()
		locked.Unlock()
	}
}

// closeAndWait closes endpoint transports and joins their servers once.
func (e *EndpointSet) closeAndWait() error {
	if e == nil {
		return nil
	}
	for {
		locked := e.bcast.Lock()
		if e.closed {
			err := e.closeErr
			locked.Unlock()
			return err
		}
		if e.closing {
			wait := locked.WaitCh()
			locked.Unlock()
			<-wait
			continue
		}
		e.closing = true
		locked.Broadcast()
		locked.Unlock()

		childErr := e.closeChildFiles()
		var closeErr, waitErr error
		if e.CloseFunc != nil {
			closeErr = e.CloseFunc()
		}
		if e.WaitFunc != nil {
			waitErr = e.WaitFunc()
		}
		locked = e.bcast.Lock()
		e.closing = false
		e.closed = true
		e.closeErr = errors.Join(childErr, closeErr, waitErr)
		locked.Broadcast()
		locked.Unlock()
	}
}
