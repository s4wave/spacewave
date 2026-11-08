// Package world_opcheck checks that a World operation is the same everywhere
// it runs: validated, rehearsed outside the writer slot, applied under it, and
// replayed from its encoding on another replica.
package world_opcheck

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Case is one operation to check and the World it applies to.
type Case struct {
	// Setup builds the World the operation applies to, in one write.
	Setup func(ctx context.Context, ws world.WorldState) error
	// Op is the operation, carrying every input its apply takes.
	Op world.Operation
	// Sender is the peer the operation is applied as.
	Sender peer.ID
	// LookupOp resolves operation types for the replay.
	LookupOp world.LookupOp
}

// errNetworkCall is returned for every call through the failing transport.
var errNetworkCall = errors.New("an operation's apply called the network")

// transportMtx serializes checks, which replace the process's HTTP transport.
var transportMtx sync.Mutex

// Check validates c's operation, then applies it to the World Setup builds
// three ways: on a rehearsal, under the writer slot, and decoded from its
// encoding on a second replica a moment later. It fails t when the three
// disagree on the operation's rejection, when the applied and replayed World
// roots differ, as they do when the apply reads the clock or a random source,
// or when an apply calls the network through the HTTP transport, which fails
// every call while the check runs.
func Check(t testing.TB, c Case) {
	// Validate the operation.
	t.Helper()
	ctx := context.Background()
	if err := world.ValidateOp(c.Op); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Build the World the operation applies to.
	tbed, base := buildBase(ctx, t, c)
	defer tbed.Release()

	// Fail and count every HTTP call while the operation applies.
	transportMtx.Lock()
	defer transportMtx.Unlock()
	calls := &failingTransport{}
	prev := http.DefaultTransport
	http.DefaultTransport = calls
	defer func() { http.DefaultTransport = prev }()

	// Rehearse, then apply under the slot, on one replica.
	eng, closeEng := openAt(ctx, t, tbed, base, c.LookupOp)
	defer closeEng()
	rehearsed := world.Rehearse(ctx, eng, c.Sender, c.Op)
	appliedRoot, applied := applyOp(ctx, eng, c.Op, c.Sender)

	// Replay the decoded operation on a second replica a moment later.
	time.Sleep(2 * time.Millisecond)
	replayOp := decodeOp(ctx, t, c)
	replica, closeReplica := openAt(ctx, t, tbed, base, c.LookupOp)
	defer closeReplica()
	replayedRoot, replayed := applyOp(ctx, replica, replayOp, c.Sender)

	// Compare the three runs.
	if n := calls.n.Load(); n != 0 {
		t.Fatalf("the operation's apply made %d network calls", n)
	}
	if !sameOutcome(applied, replayed) {
		t.Fatalf("apply returned %v, replay returned %v", applied, replayed)
	}
	if !sameOutcome(applied, rehearsed) {
		t.Fatalf("apply returned %v, rehearsal returned %v", applied, rehearsed)
	}
	if !appliedRoot.EqualsRef(replayedRoot) {
		t.Fatal("the replayed World differs from the applied World")
	}
}

// buildBase builds the World Setup describes and returns its root.
func buildBase(ctx context.Context, t testing.TB, c Case) (*testbed.Testbed, *bucket.ObjectRef) {
	// Open a testbed with an empty World.
	t.Helper()
	tbed, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	eng, closeEng := openAt(ctx, t, tbed, nil, c.LookupOp)
	defer closeEng()

	// Run Setup in one write.
	if c.Setup != nil {
		if err := world.ExecTransaction(ctx, eng, true, c.Setup); err != nil {
			tbed.Release()
			t.Fatalf("setup: %v", err)
		}
	}
	return tbed, eng.GetRootRef()
}

// openAt opens a World engine on tbed at root, or on an empty World.
func openAt(ctx context.Context, t testing.TB, tbed *testbed.Testbed, root *bucket.ObjectRef, lookupOp world.LookupOp) (*world_block.Engine, func()) {
	// Build a cursor at root.
	t.Helper()
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if root != nil {
		ocs.SetRootRef(root.GetRootRef())
	}

	// Open the engine on it.
	eng, err := world_block.NewEngine(ctx, tbed.Logger, ocs, lookupOp, nil, false)
	if err != nil {
		ocs.Release()
		t.Fatal(err)
	}
	return eng, func() {
		eng.Close()
		ocs.Release()
	}
}

// applyOp applies op under the writer slot and returns the resulting World
// root and the operation's rejection.
func applyOp(ctx context.Context, eng *world_block.Engine, op world.Operation, sender peer.ID) (*bucket.ObjectRef, error) {
	_, _, err := world.NewEngineWorldState(eng, true).ApplyWorldOp(ctx, op, sender)
	return eng.GetRootRef(), err
}

// decodeOp encodes c's operation and decodes it as a replica does, through
// the operation type lookup.
func decodeOp(ctx context.Context, t testing.TB, c Case) world.Operation {
	// Encode the operation.
	t.Helper()
	data, err := c.Op.MarshalVT()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode it into a fresh operation of its type.
	op, err := c.LookupOp(ctx, c.Op.TypeID())
	if err != nil || op == nil {
		t.Fatalf("look up operation type %q: %v", c.Op.TypeID(), err)
	}
	if err := op.UnmarshalVT(data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return op
}

// sameOutcome reports whether two runs both accepted the operation or both
// rejected it with the same message.
func sameOutcome(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Error() == b.Error()
}

// failingTransport is an HTTP transport that fails and counts every call.
type failingTransport struct {
	// n counts the calls.
	n atomic.Int64
}

// RoundTrip fails the call.
func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.n.Add(1)
	return nil, errNetworkCall
}
