package sobject

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// TestBodyHostAttachesOneBody checks that a body host attaches one body at a
// time: a second body waits until the first is released, and a wait whose
// context ends attaches nothing.
func TestBodyHostAttachesOneBody(t *testing.T) {
	// Attach a first body.
	h := NewBodyHost(logrus.NewEntry(logrus.New()), nil, nil)
	first, second := &testBody{}, &testBody{}
	_, release, err := h.SetBody(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}

	// A second attach whose context ends while the first is attached fails.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := h.SetBody(ctx, second); !errors.Is(err, context.Canceled) {
		t.Fatalf("attach beside an attached body = %v, want canceled", err)
	}

	// A second attach waits for the first's release, then attaches.
	attached := make(chan func(), 1)
	go func() {
		_, rel, err := h.SetBody(t.Context(), second)
		if err != nil {
			t.Error(err)
		}
		attached <- rel
	}()
	select {
	case <-attached:
		t.Fatal("a second body attached while the first was attached")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	(<-attached)()
}

// heldInstallBody keeps Install in flight after cancellation until finish closes.
type heldInstallBody struct {
	testBody
	// entered reports that Install is using the body's resources.
	entered chan struct{}
	// canceled reports that the driver requested shutdown during Install.
	canceled chan struct{}
	// finish lets Install release the body's resources and return.
	finish chan struct{}
}

// Install holds the body across cancellation to expose an unjoined teardown.
func (b *heldInstallBody) Install(ctx context.Context, _ *SOCheckpointInner, _ int) (bool, error) {
	// Keep the body's resources until the test permits the call to finish.
	close(b.entered)
	<-ctx.Done()
	close(b.canceled)
	<-b.finish
	return false, context.Cause(ctx)
}

// TestBodyHostStopRetainsClaim joins an in-flight body call before cleanup,
// then keeps the next body out until the caller releases the cleaned-up body.
func TestBodyHostStopRetainsClaim(t *testing.T) {
	// Run a real driver against the in-memory SharedObject state.
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	so := &ackTestObject{t: t, priv: keys[0], state: state, ctr: ccontainer.NewCContainer[SharedObjectStateSnapshot](nil)}
	so.publish()
	h := NewBodyHost(logrus.NewEntry(logrus.New()), so, nil)
	body := &heldInstallBody{entered: make(chan struct{}), canceled: make(chan struct{}), finish: make(chan struct{})}
	stop, release, err := h.SetBody(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	t.Cleanup(func() {
		select {
		case <-body.finish:
		default:
			close(body.finish)
		}
	})
	h.SetContext(t.Context())

	// Request teardown while Install still owns body resources.
	select {
	case <-body.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("driver never entered Install")
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-body.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("stop never canceled Install")
	}
	select {
	case <-stopped:
		t.Fatal("stop returned while Install still used the body")
	case <-time.After(50 * time.Millisecond):
	}

	// Finish the body call, then retain the claim throughout resource cleanup.
	close(body.finish)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop never joined Install")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := h.SetBody(ctx, &testBody{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("successor before cleanup = %v, want canceled", err)
	}
	stop()

	// Release only after cleanup, admitting the successor and no earlier.
	release()
	release()
	_, releaseNext, err := h.SetBody(t.Context(), &testBody{})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	release()
	if _, _, err := h.SetBody(ctx, &testBody{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("old release freed the successor's claim: %v", err)
	}
	releaseNext()
}
