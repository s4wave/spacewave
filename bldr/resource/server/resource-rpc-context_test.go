package resource_server

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
)

type resourceRPCSendStream struct {
	ctx     context.Context
	sendErr error
}

func (s *resourceRPCSendStream) Context() context.Context {
	return s.ctx
}

func (s *resourceRPCSendStream) MsgSend(srpc.Message) error {
	return s.sendErr
}

func (s *resourceRPCSendStream) MsgRecv(srpc.Message) error {
	return nil
}

func (s *resourceRPCSendStream) CloseSend() error {
	return nil
}

func (s *resourceRPCSendStream) Close() error {
	return nil
}

func TestResourceRPCResponseResourceOwnership(t *testing.T) {
	sendErr := errors.New("send failed")
	tests := []struct {
		name                string
		handled             bool
		sendErr             error
		addValue            bool
		releaseBeforeReturn bool
		wantRetained        bool
		wantReleases        int
		wantNotifications   int
	}{
		{name: "successful response", handled: true, wantRetained: true},
		{name: "failed response", handled: true, sendErr: sendErr, wantReleases: 1, wantNotifications: 1},
		{name: "failed value response", handled: true, sendErr: sendErr, addValue: true, wantReleases: 1, wantNotifications: 1},
		{name: "unhandled response", wantReleases: 1, wantNotifications: 1},
		{
			name:                "resource released before response failure",
			handled:             true,
			sendErr:             sendErr,
			releaseBeforeReturn: true,
			wantReleases:        1,
			wantNotifications:   1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, cancel := newTestClient(t)
			t.Cleanup(cancel)
			server := client.server
			resourceReleases := 0
			var resourceID uint32
			invoker := &resourceServerClientInvoker{
				client: client,
				mux: srpc.InvokerFunc(func(_, _ string, strm srpc.Stream) (bool, error) {
					resourceCtx, err := MustGetResourceClientContext(strm.Context())
					if err != nil {
						return true, err
					}
					releaseFn := func() {
						resourceReleases++
					}
					if test.addValue {
						resourceID, err = resourceCtx.AddResourceValue(srpc.NewMux(), "value", releaseFn)
					} else {
						resourceID, err = resourceCtx.AddResource(srpc.NewMux(), releaseFn)
					}
					if err != nil {
						return true, err
					}
					if test.releaseBeforeReturn {
						resourceCtx.ReleaseResource(resourceID)
					}
					if !test.handled {
						return false, nil
					}
					return true, strm.MsgSend(&resource.ResourceRefReleaseResponse{})
				}),
			}

			ok, err := invoker.InvokeMethod("test.Service", "CreateResource", &resourceRPCSendStream{
				ctx:     t.Context(),
				sendErr: test.sendErr,
			})
			if ok != test.handled {
				t.Fatalf("resource RPC handled: got %v, want %v", ok, test.handled)
			}
			if !errors.Is(err, test.sendErr) {
				t.Fatalf("response error: got %v, want %v", err, test.sendErr)
			}

			var retained bool
			var notifications int
			server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				_, retained = client.resources[resourceID]
				notifications = len(client.txQueue)
			})
			if retained != test.wantRetained {
				t.Fatalf("resource %d retained: got %v, want %v", resourceID, retained, test.wantRetained)
			}
			if resourceReleases != test.wantReleases {
				t.Fatalf("resource release callbacks: got %d, want %d", resourceReleases, test.wantReleases)
			}
			if notifications != test.wantNotifications {
				t.Fatalf("resource release notifications: got %d, want %d", notifications, test.wantNotifications)
			}
			if retained {
				client.ReleaseResource(resourceID)
			}
		})
	}
}

func TestRemoteResourceClientFinishDuringAddDoesNotLeak(t *testing.T) {
	type addResult struct {
		resourceID uint32
		err        error
	}

	for range 100 {
		client, cancel := newTestClient(t)
		client.adoptionAckEnabled = true
		resourceCtx := newResourceRPCContext(client)
		var resourceReleases atomic.Int32
		start := make(chan struct{})
		addDone := make(chan addResult, 1)
		finishDone := make(chan struct{})

		go func() {
			<-start
			resourceID, err := resourceCtx.AddResource(srpc.NewMux(), func() {
				resourceReleases.Add(1)
			})
			addDone <- addResult{resourceID: resourceID, err: err}
		}()
		go func() {
			<-start
			resourceCtx.finish(false)
			close(finishDone)
		}()
		close(start)

		result := <-addDone
		<-finishDone
		if result.err != nil && !errors.Is(result.err, resource.ErrClientReleased) {
			t.Fatalf("add resource: %v", result.err)
		}
		wantReleases := int32(0)
		if result.err == nil {
			wantReleases = 1
		}
		if got := resourceReleases.Load(); got != wantReleases {
			t.Fatalf("resource release callbacks: got %d, want %d", got, wantReleases)
		}

		client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if len(client.resources) != 0 {
				t.Fatalf("retained resources: %d", len(client.resources))
			}
			if len(client.pendingAdoptions) != 0 {
				t.Fatalf("pending adoptions: %d", len(client.pendingAdoptions))
			}
		})
		cancel()
	}
}

func TestResourceRPCInvokerPreservesAttachedResourcePath(t *testing.T) {
	strm := &resourceRPCSendStream{ctx: t.Context()}
	invoked := false
	invoker := &resourceServerClientInvoker{
		mux: srpc.InvokerFunc(func(_, _ string, got srpc.Stream) (bool, error) {
			invoked = true
			if got != strm {
				t.Fatal("attached resource stream was wrapped")
			}
			if GetResourceClientContext(got.Context()) != nil {
				t.Fatal("attached resource received a server client context")
			}
			return false, nil
		}),
	}

	ok, err := invoker.InvokeMethod("test.Service", "Attached", strm)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("attached resource RPC was unexpectedly handled")
	}
	if !invoked {
		t.Fatal("attached resource invoker was not called")
	}
}

func TestResourceRPCAdoptionCommitReleasesOnlyPending(t *testing.T) {
	client, cancel := newTestClient(t)
	t.Cleanup(cancel)
	client.adoptionAckEnabled = true
	client.pendingAdoptions = make(map[uint32]*resourceRPCContext)
	resourceCtx := newResourceRPCContext(client)
	var adoptedReleases, pendingReleases int

	adoptedID, err := resourceCtx.AddResource(srpc.NewMux(), func() {
		adoptedReleases++
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingID, err := resourceCtx.AddResource(srpc.NewMux(), func() {
		pendingReleases++
	})
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(adoptedID) {
		t.Fatal("first adoption was rejected")
	}
	if !client.adoptResource(adoptedID) {
		t.Fatal("duplicate adoption of retained resource was rejected")
	}

	resourceCtx.finish(true)

	client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if client.resources[adoptedID] == nil {
			t.Fatal("adopted resource was released on commit")
		}
		if client.resources[pendingID] != nil {
			t.Fatal("created-but-unreturned resource survived commit")
		}
		if len(client.pendingAdoptions) != 0 {
			t.Fatalf("pending adoptions after commit: %d", len(client.pendingAdoptions))
		}
		if len(client.adoptedAdoptions) != 0 {
			t.Fatalf("adopted adoptions after commit: %d", len(client.adoptedAdoptions))
		}
	})
	if adoptedReleases != 0 || pendingReleases != 1 {
		t.Fatalf(
			"release callbacks: adopted=%d pending=%d, want 0/1",
			adoptedReleases,
			pendingReleases,
		)
	}
	client.ReleaseResource(adoptedID)
	if adoptedReleases != 1 {
		t.Fatalf("adopted release callbacks: got %d, want 1", adoptedReleases)
	}
}

func TestResourceRPCFinishWakesNilReleaseNotification(t *testing.T) {
	client, cancel := newTestClient(t)
	t.Cleanup(cancel)
	client.adoptionAckEnabled = true
	resourceCtx := newResourceRPCContext(client)

	resourceID, err := resourceCtx.AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var waitCh <-chan struct{}
	client.server.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	resourceCtx.finish(false)

	select {
	case <-waitCh:
	default:
		t.Fatal("queued release notification did not wake ResourceClient")
	}
	client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if client.resources[resourceID] != nil {
			t.Fatal("canceled resource was retained")
		}
		if len(client.txQueue) != 1 {
			t.Fatalf("queued notifications: got %d, want 1", len(client.txQueue))
		}
		released := client.txQueue[0].GetResourceReleased()
		if released == nil || released.GetResourceId() != resourceID {
			t.Fatalf("queued notification: got %v, want release for %d", released, resourceID)
		}
	})
}

func TestResourceRPCAdoptionCancelReleasesPendingAndAdopted(t *testing.T) {
	client, cancel := newTestClient(t)
	t.Cleanup(cancel)
	client.adoptionAckEnabled = true
	client.pendingAdoptions = make(map[uint32]*resourceRPCContext)
	client.adoptedAdoptions = make(map[uint32]*resourceRPCContext)
	resourceCtx := newResourceRPCContext(client)
	releases := 0
	pendingID, err := resourceCtx.AddResource(srpc.NewMux(), func() {
		releases++
	})
	if err != nil {
		t.Fatal(err)
	}
	adoptedID, err := resourceCtx.AddResource(srpc.NewMux(), func() {
		releases++
	})
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(adoptedID) {
		t.Fatal("adoption was rejected")
	}

	resourceCtx.finish(false)
	resourceCtx.finish(false)

	client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if client.resources[pendingID] != nil {
			t.Fatal("canceled pending resource was retained")
		}
		if client.resources[adoptedID] != nil {
			t.Fatal("canceled adopted resource was retained")
		}
		if len(client.pendingAdoptions) != 0 {
			t.Fatalf("pending adoptions after cancel: %d", len(client.pendingAdoptions))
		}
		if len(client.adoptedAdoptions) != 0 {
			t.Fatalf("adopted adoptions after cancel: %d", len(client.adoptedAdoptions))
		}
	})
	if releases != 2 {
		t.Fatalf("release callbacks: got %d, want 2", releases)
	}
}

// _ is a type assertion
var _ srpc.Stream = (*resourceRPCSendStream)(nil)
