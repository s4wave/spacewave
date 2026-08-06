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
	var controls []*resource.ResourceClientRequest
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		controls = append(controls, req)
		return true
	})
	first := lifetime.createReference(7)
	second := lifetime.createReference(7)
	if len(controls) != 1 {
		t.Fatalf("adopt controls = %d, want 1", len(controls))
	}
	if kind, id := controlKindID(t, controls[0]); kind != "adopt" || id != 7 {
		t.Fatalf("first control = %s/%d", kind, id)
	}
	first.Release()
	if len(controls) != 1 {
		t.Fatalf("release before final ref queued unexpectedly: %d", len(controls))
	}
	second.Release()
	if len(controls) != 2 {
		t.Fatalf("controls = %d, want adopt/release", len(controls))
	}
	if kind, id := controlKindID(t, controls[1]); kind != "release" || id != 7 {
		t.Fatalf("final control = %s/%d", kind, id)
	}
}

func TestResourceLifetimeSerializesFinalReleaseBeforeNewAdopt(t *testing.T) {
	var controlsMtx sync.Mutex
	var controls []*resource.ResourceClientRequest
	releaseEntered := make(chan struct{})
	allowRelease := make(chan struct{})
	var releaseOnce sync.Once
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		if _, release := req.GetBody().(*resource.ResourceClientRequest_Release); release {
			releaseOnce.Do(func() {
				close(releaseEntered)
				<-allowRelease
			})
		}
		controlsMtx.Lock()
		controls = append(controls, req)
		controlsMtx.Unlock()
		return true
	})
	ref := lifetime.createReference(7)
	controlsMtx.Lock()
	controls = nil
	controlsMtx.Unlock()

	released := make(chan struct{})
	go func() {
		ref.Release()
		close(released)
	}()
	<-releaseEntered

	created := make(chan ResourceRef, 1)
	go func() { created <- lifetime.createReference(7) }()
	select {
	case <-created:
		close(allowRelease)
		<-released
		t.Fatal("new reference passed the final-release transition")
	case <-time.After(20 * time.Millisecond):
	}

	close(allowRelease)
	<-released
	newRef := <-created
	defer newRef.Release()

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
	var controls []*resource.ResourceClientRequest
	lifetime := newResourceLifetime(context.Background(), nil, func(req *resource.ResourceClientRequest) bool {
		controls = append(controls, req)
		return true
	})
	lifetime.createReference(11)
	lifetime.createReference(3)
	lifetime.releaseAll()
	if len(controls) != 4 {
		t.Fatalf("controls = %d, want 4", len(controls))
	}
	for i, want := range []uint32{3, 11} {
		kind, id := controlKindID(t, controls[2+i])
		if kind != "release" || id != want {
			t.Fatalf("close control %d = %s/%d", i, kind, id)
		}
	}
	if got := lifetime.createReference(11); !got.(*resourceRef).released {
		t.Fatal("reference created after close was live")
	}
}

func TestResourceLifetimeReleasedNotificationClearsReferences(t *testing.T) {
	lifetime := newResourceLifetime(context.Background(), nil, func(*resource.ResourceClientRequest) bool { return true })
	ref := lifetime.createReference(9)
	lifetime.releaseFromServer(9)
	if _, err := ref.GetClient(); err != resource.ErrResourceOrClientReleased {
		t.Fatalf("GetClient error = %v", err)
	}
}

func TestResourceControlQueueRetirePurgesUnsentControls(t *testing.T) {
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

	queue.retire(wantErr)

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
