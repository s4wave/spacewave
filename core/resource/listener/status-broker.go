package resource_listener

import (
	"sync"

	"github.com/aperturerobotics/util/broadcast"
)

// processStatusBrokerOnce guards lazy construction of the process-wide
// listener status broker. The listener controller and the Root
// resource server share a single broker so UI subscribers see the
// same listener state the controller publishes.
var (
	processStatusBrokerOnce sync.Once
	processStatusBroker     *StatusBroker
)

// GetProcessStatusBroker returns the process-wide listener status
// broker. On the first call it lazily constructs a broker.
func GetProcessStatusBroker() *StatusBroker {
	processStatusBrokerOnce.Do(func() {
		processStatusBroker = NewStatusBroker()
	})
	return processStatusBroker
}

// ListenerStatus is the current listener state emitted to UI
// subscribers.
type ListenerStatus struct {
	// SocketPath is the resolved absolute socket path. Populated as
	// soon as the controller resolves the configured path, before the
	// first bind attempt.
	SocketPath string
	// Listening is true when the listener has an open Unix socket.
	// False before the first bind, while the runtime is handed off,
	// and while tearing down.
	Listening bool
	// ConnectedClients is the count of currently accepted client
	// connections on the listener socket.
	ConnectedClients uint32
}

// StatusBroker tracks the desktop resource listener state behind a
// single broadcast.Broadcast and exposes a snapshot + wait-channel
// interface for UI subscribers.
//
// The listener controller mutates state via SetSocketPath,
// SetListening, and AddClient / RemoveClient. Subscribers read the
// snapshot under the broadcast lock and then block on the returned
// wait channel until the next change.
type StatusBroker struct {
	bcast  broadcast.Broadcast
	status ListenerStatus
}

// NewStatusBroker constructs a StatusBroker with zero state (empty
// socket path, not listening, zero clients).
func NewStatusBroker() *StatusBroker {
	return &StatusBroker{}
}

// SetSocketPath records the resolved socket path. Broadcasts if the
// path changed.
func (b *StatusBroker) SetSocketPath(path string) {
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if b.status.SocketPath == path {
			return
		}
		b.status.SocketPath = path
		broadcastFn()
	})
}

// SetListening records whether the listener socket is bound.
// Broadcasts if the value changed. When transitioning to not
// listening the connected-client count is reset to zero to match
// observable reality (all in-flight clients drop when the socket
// closes).
func (b *StatusBroker) SetListening(listening bool) {
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		changed := b.status.Listening != listening
		b.status.Listening = listening
		if !listening && b.status.ConnectedClients != 0 {
			b.status.ConnectedClients = 0
			changed = true
		}
		if changed {
			broadcastFn()
		}
	})
}

// AddClient increments the connected-client counter and broadcasts.
func (b *StatusBroker) AddClient() {
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		b.status.ConnectedClients++
		broadcastFn()
	})
}

// RemoveClient decrements the connected-client counter and
// broadcasts. No-op when already zero to guard against double-count.
func (b *StatusBroker) RemoveClient() {
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if b.status.ConnectedClients == 0 {
			return
		}
		b.status.ConnectedClients--
		broadcastFn()
	})
}

// Snapshot returns the current listener status and a wait channel
// that closes on the next state change.
func (b *StatusBroker) Snapshot() (ListenerStatus, <-chan struct{}) {
	var out ListenerStatus
	var waitCh <-chan struct{}
	b.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		out = b.status
	})
	return out, waitCh
}
