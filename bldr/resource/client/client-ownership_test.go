package resource_client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/bldr/resource"
)

func controlKindID(t *testing.T, req *resource.ResourceClientRequest) (string, uint32) {
	t.Helper()
	switch body := req.GetBody().(type) {
	case *resource.ResourceClientRequest_Adopt:
		return "adopt", body.Adopt.GetResourceId()
	case *resource.ResourceClientRequest_Release:
		return "release", body.Release.GetResourceId()
	default:
		t.Fatalf("unexpected control %T", req.GetBody())
		return "", 0
	}
}

func TestResourceLifetimeQueuesOneAdoptAndFinalRelease(t *testing.T) {
	// Capture lifecycle controls for two references to the same resource.
	var controls []*resource.ResourceClientRequest
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		controls = append(controls, req)
		return true
	})
	first := lifetime.createReference(7)
	second := lifetime.createReference(7)

	// Verify both references share one resource adoption.
	if len(controls) != 1 {
		t.Fatalf("adopt controls = %d, want 1", len(controls))
	}
	if kind, id := controlKindID(t, controls[0]); kind != "adopt" || id != 7 {
		t.Fatalf("first control = %s/%d", kind, id)
	}

	// Verify releasing the first reference keeps the resource adopted.
	first.Release()
	if len(controls) != 1 {
		t.Fatalf("release before final ref queued unexpectedly: %d", len(controls))
	}

	// Verify releasing the final reference queues one resource release.
	second.Release()
	if len(controls) != 2 {
		t.Fatalf("controls = %d, want adopt/release", len(controls))
	}
	if kind, id := controlKindID(t, controls[1]); kind != "release" || id != 7 {
		t.Fatalf("final control = %s/%d", kind, id)
	}
}

func TestResourceLifetimeSerializesFinalReleaseBeforeNewAdopt(t *testing.T) {
	// Gate final resource release while recording lifecycle control order.
	var controlsMtx sync.Mutex
	var controls []*resource.ResourceClientRequest
	releaseEntered := make(chan struct{})
	allowRelease := make(chan struct{})
	var releaseOnce sync.Once
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		// Block the release control until the new adoption attempts to start.
		if _, release := req.GetBody().(*resource.ResourceClientRequest_Release); release {
			releaseOnce.Do(func() {
				close(releaseEntered)
				<-allowRelease
			})
		}

		// Record the resource control after its release gate opens.
		controlsMtx.Lock()
		controls = append(controls, req)
		controlsMtx.Unlock()
		return true
	})

	// Adopt the initial resource and clear its setup control.
	ref := lifetime.createReference(7)
	controlsMtx.Lock()
	controls = nil
	controlsMtx.Unlock()

	// Begin releasing the final reference and wait for its control callback.
	released := make(chan struct{})
	go func() {
		ref.Release()
		close(released)
	}()
	<-releaseEntered

	// Verify a new resource reference waits for the final release transition.
	created := make(chan ResourceRef, 1)
	go func() { created <- lifetime.createReference(7) }()
	select {
	case <-created:
		close(allowRelease)
		<-released
		t.Fatal("new reference passed the final-release transition")
	case <-time.After(20 * time.Millisecond):
	}

	// Allow the release to finish and retain the newly adopted reference.
	close(allowRelease)
	<-released
	newRef := <-created
	defer newRef.Release()

	// Verify lifecycle controls release the old resource before adopting it again.
	controlsMtx.Lock()
	defer controlsMtx.Unlock()
	if len(controls) != 2 {
		t.Fatalf("controls = %d, want release/adopt", len(controls))
	}
	if kind, id := controlKindID(t, controls[0]); kind != "release" || id != 7 {
		t.Fatalf("first control = %s/%d, want release/7", kind, id)
	}
	if kind, id := controlKindID(t, controls[1]); kind != "adopt" || id != 7 {
		t.Fatalf("second control = %s/%d, want adopt/7", kind, id)
	}
}

func TestResourceLifetimeCloseQueuesAllReleases(t *testing.T) {
	// Capture lifecycle controls for resources left outstanding at close.
	var controls []*resource.ResourceClientRequest
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		controls = append(controls, req)
		return true
	})

	// Leave two resource references outstanding and close their lifetime.
	lifetime.createReference(11) //nolint:lostresource // Leave references outstanding to verify releaseAll queues their releases.
	lifetime.createReference(3)  //nolint:lostresource // Leave references outstanding to verify releaseAll queues their releases.
	lifetime.releaseAll()

	// Verify closing the resource lifetime releases every outstanding resource.
	if len(controls) != 4 {
		t.Fatalf("controls = %d, want 4", len(controls))
	}
	for i, want := range []uint32{3, 11} {
		// Verify the release control identifies the expected outstanding resource.
		kind, id := controlKindID(t, controls[2+i])
		if kind != "release" || id != want {
			t.Fatalf("close control %d = %s/%d", i, kind, id)
		}
	}

	// Verify a retired resource lifetime cannot create a live reference.
	if got := lifetime.createReference(11); !got.(*resourceRef).released { //nolint:lostresource // Verify that a retired lifetime cannot create a live reference.
		t.Fatal("reference created after close was live")
	}
}

func TestResourceLifetimeReleasedNotificationClearsReferences(t *testing.T) {
	lifetime := newResourceLifetime(context.Background(), nil, func(*resource.ResourceClientRequest) bool { return true })
	ref := lifetime.createReference(9) //nolint:lostresource // Exercise server-driven release with an outstanding local reference.
	lifetime.releaseFromServer(9)
	if _, err := ref.GetClient(); err != resource.ErrResourceOrClientReleased {
		t.Fatalf("GetClient error = %v", err)
	}
}

func TestResourceControlQueueRetirePurgesUnsentControls(t *testing.T) {
	// Prepare a control queue with an unsent adoption and a failure callback.
	wantErr := errors.New("writer failed")
	var gotErr error
	queue := &resourceControlQueue{
		items: []*resource.ResourceClientRequest{
			{Body: &resource.ResourceClientRequest_Adopt{
				Adopt: &resource.ResourceClientAdopt{ResourceId: 7},
			}},
		},
		onFailure: func(err error) { gotErr = err },
	}

	// Retire the control queue with its writer failure.
	queue.retire(wantErr)

	// Verify retirement purges pending controls and rejects further releases.
	if !queue.retired || len(queue.items) != 0 {
		t.Fatalf(
			"retired queue state = retired:%v items:%d",
			queue.retired,
			len(queue.items),
		)
	}
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("failure callback error = %v, want %v", gotErr, wantErr)
	}
	if queue.enqueue(&resource.ResourceClientRequest{
		Body: &resource.ResourceClientRequest_Release{
			Release: &resource.ResourceClientRelease{ResourceId: 7},
		},
	}) {
		t.Fatal("retired queue accepted a new control")
	}
}
