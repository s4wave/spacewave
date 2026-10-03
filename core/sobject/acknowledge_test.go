package sobject

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ackTestObject is a SharedObject holding one state that writes each queued
// operation as its local peer.
type ackTestObject struct {
	SharedObject
	t     *testing.T
	priv  crypto.PrivKey
	state *SOState
	ctr   *ccontainer.CContainer[SharedObjectStateSnapshot]
	// acks counts the acknowledgments written.
	acks chan struct{}
}

// GetPeerID returns the local peer.
func (o *ackTestObject) GetPeerID() peer.ID {
	id, err := peer.IDFromPrivateKey(o.priv)
	if err != nil {
		o.t.Fatal(err)
	}
	return id
}

// AccessSharedObjectState returns the state container.
func (o *ackTestObject) AccessSharedObjectState(context.Context, func()) (ccontainer.Watchable[SharedObjectStateSnapshot], func(), error) {
	return o.ctr, func() {}, nil
}

// QueueOperation writes op as the local peer and publishes the state.
func (o *ackTestObject) QueueOperation(_ context.Context, op []byte) (string, error) {
	writeTestOp(o.t, o.state, o.priv, string(op))
	if len(op) == 0 {
		o.acks <- struct{}{}
	}
	o.publish()
	return "", nil
}

// publish sets a snapshot of the held state.
func (o *ackTestObject) publish() {
	o.ctr.SetValue(testHandle(o.t, o.state.CloneVT(), o.priv))
}

// TestAcknowledge checks that a writer acknowledges once the owner's edits
// reach the lag, and not again for its own acknowledgment.
func TestAcknowledge(t *testing.T) {
	// The owner writes one edit short of the lag.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	for range AcknowledgmentLag - 1 {
		writeTestOp(t, state, keys[0], "edit")
	}
	so := &ackTestObject{
		t:     t,
		priv:  keys[1],
		state: state,
		ctr:   ccontainer.NewCContainer[SharedObjectStateSnapshot](nil),
		acks:  make(chan struct{}, 2),
	}
	so.publish()

	// Run the writer's acknowledger.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	write := func(ctx context.Context) error {
		_, err := so.QueueOperation(ctx, nil)
		return err
	}
	go func() { done <- Acknowledge(ctx, so, write) }()

	// The edit reaching the lag draws one acknowledgment, which draws none.
	// The writer's next edit publishes a state after the acknowledgment.
	writeTestOp(t, state, keys[0], "edit")
	so.publish()
	<-so.acks
	if _, err := so.QueueOperation(ctx, []byte("edit")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-so.acks:
		t.Fatal("the writer acknowledged twice")
	default:
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("acknowledger ended with %v", err)
	}
}
