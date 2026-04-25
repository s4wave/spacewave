package resource_listener

import (
	"sync"
	"testing"
	"time"
)

// TestStatusBrokerInitialState asserts a fresh broker starts with
// zero-value state so a subscriber subscribing before the listener
// controller runs sees listening=false and an empty socket path.
func TestStatusBrokerInitialState(t *testing.T) {
	b := NewStatusBroker()
	snapshot, waitCh := b.Snapshot()
	if snapshot.Listening {
		t.Fatalf("fresh broker should not report listening")
	}
	if snapshot.SocketPath != "" {
		t.Fatalf("fresh broker socket path: %q", snapshot.SocketPath)
	}
	if snapshot.ConnectedClients != 0 {
		t.Fatalf("fresh broker connected clients: %d", snapshot.ConnectedClients)
	}
	select {
	case <-waitCh:
		t.Fatalf("wait channel closed without state change")
	default:
	}
}

// TestStatusBrokerSetSocketPathBeforeListen asserts the socket path
// can be published before Listening flips to true and that a single
// subscriber observes the path + listening=false snapshot first.
func TestStatusBrokerSetSocketPathBeforeListen(t *testing.T) {
	b := NewStatusBroker()
	b.SetSocketPath("/run/spacewave.sock")
	snapshot, _ := b.Snapshot()
	if snapshot.SocketPath != "/run/spacewave.sock" {
		t.Fatalf("socket path: %q", snapshot.SocketPath)
	}
	if snapshot.Listening {
		t.Fatalf("socket path set should not flip listening")
	}
}

// TestStatusBrokerListenTransitions asserts the listen/stop
// transitions broadcast and the snapshot reflects each step.
func TestStatusBrokerListenTransitions(t *testing.T) {
	b := NewStatusBroker()
	b.SetSocketPath("/run/spacewave.sock")

	_, waitCh := b.Snapshot()
	b.SetListening(true)
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatalf("listen transition did not wake subscriber")
	}
	snapshot, waitCh := b.Snapshot()
	if !snapshot.Listening {
		t.Fatalf("listening false after SetListening(true)")
	}
	if snapshot.SocketPath != "/run/spacewave.sock" {
		t.Fatalf("socket path lost: %q", snapshot.SocketPath)
	}

	b.AddClient()
	b.AddClient()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatalf("accept did not wake subscriber")
	}
	snapshot, waitCh = b.Snapshot()
	if snapshot.ConnectedClients != 2 {
		t.Fatalf("connected clients: %d", snapshot.ConnectedClients)
	}

	b.RemoveClient()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatalf("close did not wake subscriber")
	}
	snapshot, waitCh = b.Snapshot()
	if snapshot.ConnectedClients != 1 {
		t.Fatalf("connected clients after one close: %d", snapshot.ConnectedClients)
	}

	b.SetListening(false)
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatalf("stop transition did not wake subscriber")
	}
	snapshot, _ = b.Snapshot()
	if snapshot.Listening {
		t.Fatalf("still listening after SetListening(false)")
	}
	if snapshot.ConnectedClients != 0 {
		t.Fatalf("client count should zero on stop, got %d", snapshot.ConnectedClients)
	}
}

// TestStatusBrokerSetSocketPathIdempotent asserts that a no-op
// SetSocketPath does not close the subscriber's wait channel.
func TestStatusBrokerSetSocketPathIdempotent(t *testing.T) {
	b := NewStatusBroker()
	b.SetSocketPath("/run/spacewave.sock")
	_, waitCh := b.Snapshot()
	b.SetSocketPath("/run/spacewave.sock")
	select {
	case <-waitCh:
		t.Fatalf("no-op SetSocketPath should not broadcast")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestStatusBrokerSetListeningIdempotent asserts that a no-op
// SetListening does not close the subscriber's wait channel.
func TestStatusBrokerSetListeningIdempotent(t *testing.T) {
	b := NewStatusBroker()
	b.SetListening(true)
	_, waitCh := b.Snapshot()
	b.SetListening(true)
	select {
	case <-waitCh:
		t.Fatalf("no-op SetListening should not broadcast")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestStatusBrokerRemoveClientNoUnderflow asserts RemoveClient with
// zero count is a silent no-op instead of underflowing to uint32 max.
func TestStatusBrokerRemoveClientNoUnderflow(t *testing.T) {
	b := NewStatusBroker()
	b.RemoveClient()
	snapshot, _ := b.Snapshot()
	if snapshot.ConnectedClients != 0 {
		t.Fatalf("connected clients after underflow: %d", snapshot.ConnectedClients)
	}
}

// TestStatusBrokerMultipleSubscribers asserts two independent
// subscribers each wake on state changes and observe consistent
// snapshots.
func TestStatusBrokerMultipleSubscribers(t *testing.T) {
	b := NewStatusBroker()
	b.SetSocketPath("/run/spacewave.sock")

	_, wA := b.Snapshot()
	_, wB := b.Snapshot()

	b.SetListening(true)

	select {
	case <-wA:
	case <-time.After(time.Second):
		t.Fatalf("subscriber A did not wake")
	}
	select {
	case <-wB:
	case <-time.After(time.Second):
		t.Fatalf("subscriber B did not wake")
	}

	sA, _ := b.Snapshot()
	sB, _ := b.Snapshot()
	if sA != sB {
		t.Fatalf("subscribers observed different snapshots: %+v vs %+v", sA, sB)
	}
	if !sA.Listening || sA.SocketPath != "/run/spacewave.sock" {
		t.Fatalf("unexpected snapshot: %+v", sA)
	}
}

// TestStatusBrokerConcurrentClients asserts the counter stays
// accurate under concurrent AddClient/RemoveClient pressure.
func TestStatusBrokerConcurrentClients(t *testing.T) {
	b := NewStatusBroker()
	b.SetListening(true)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			b.AddClient()
		}()
	}
	wg.Wait()
	snapshot, _ := b.Snapshot()
	if snapshot.ConnectedClients != n {
		t.Fatalf("connected clients after %d adds: %d", n, snapshot.ConnectedClients)
	}

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			b.RemoveClient()
		}()
	}
	wg.Wait()
	snapshot, _ = b.Snapshot()
	if snapshot.ConnectedClients != 0 {
		t.Fatalf("connected clients after %d removes: %d", n, snapshot.ConnectedClients)
	}
}

// TestGetProcessStatusBrokerSingleton asserts the process-wide broker
// is cached across calls.
func TestGetProcessStatusBrokerSingleton(t *testing.T) {
	a := GetProcessStatusBroker()
	b := GetProcessStatusBroker()
	if a != b {
		t.Fatalf("process broker singleton differs across calls")
	}
}
