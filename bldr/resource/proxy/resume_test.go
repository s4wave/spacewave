package resource_proxy_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_proxy "github.com/s4wave/spacewave/bldr/resource/proxy"
)

// viewService names the service whose Watch method streams a versioned
// view: field 1 holds the view's body and field 1000 its ViewVersion.
const viewService = "test.Views"

// viewFrame encodes a versioned view message: body, then the ViewVersion
// naming version, marked unchanged when body is empty.
func viewFrame(body, version string) []byte {
	var view []byte
	view = protowire.AppendTag(view, 1, protowire.BytesType)
	view = protowire.AppendString(view, version)
	if body == "" {
		view = protowire.AppendTag(view, 3, protowire.VarintType)
		view = protowire.AppendVarint(view, 1)
	}
	var msg []byte
	if body != "" {
		msg = protowire.AppendTag(msg, 1, protowire.BytesType)
		msg = protowire.AppendString(msg, body)
	}
	msg = protowire.AppendTag(msg, 1000, protowire.BytesType)
	return protowire.AppendBytes(msg, view)
}

// heldVersion reads the version a Watch request holds, field 1000.
func heldVersion(req []byte) string {
	var held string
	for len(req) != 0 {
		num, typ, n := protowire.ConsumeTag(req)
		if n < 0 {
			return ""
		}
		req = req[n:]
		m := protowire.ConsumeFieldValue(num, typ, req)
		if m < 0 {
			return ""
		}
		if num == 1000 && typ == protowire.BytesType {
			value, _ := protowire.ConsumeBytes(req)
			held = string(value)
		}
		req = req[m:]
	}
	return held
}

// watchView serves Watch. The first backend sends view v1. A later one
// answers a request holding v1 with v1 unchanged and anything else with v1
// whole, then sends v2, so a client sees whether the proxy resumed.
func (b *backend) watchView(strm srpc.Stream) error {
	req := srpc.NewRawMessage(nil, false)
	if err := strm.MsgRecv(req); err != nil {
		return err
	}
	held := heldVersion(req.GetData())
	b.call("Watch " + held)
	first := viewFrame("state", "v1")
	if b.gen > 1 && held == "v1" {
		first = viewFrame("", "v1")
	}
	if err := strm.MsgSend(srpc.NewRawMessage(first, false)); err != nil {
		return err
	}
	if b.gen > 1 {
		if err := strm.MsgSend(srpc.NewRawMessage(viewFrame("changed", "v2"), false)); err != nil {
			return err
		}
	}
	<-strm.Context().Done()
	return nil
}

// TestProxyResumesVersionedWatch watches a versioned view through the proxy
// while its upstream restarts twice. Each reopened request holds the version
// the client last received, and the client receives neither the unchanged
// answer nor a repeat, only the next change. The proxy reports the first
// resume quiet and the second not.
func TestProxyResumesVersionedWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// Serve a client through the proxy to the first upstream.
	first := newBackend(1)
	var current atomic.Pointer[backend]
	current.Store(first)
	proxy := resource_proxy.NewProxy(ctx, rules, func(context.Context) (resource.SRPCResourceServiceClient, error) {
		b := current.Load()
		if b.dead.Load() {
			return nil, errors.New("upstream down")
		}
		return resource.NewSRPCResourceServiceClient(srpc.NewClient(b.open)), nil
	}, resource_proxy.NewLimit(8, func() {}))
	var resumedMtx sync.Mutex
	var resumed []bool
	proxy.OnResume(func(quiet bool) {
		resumedMtx.Lock()
		resumed = append(resumed, quiet)
		resumedMtx.Unlock()
	})
	mux := srpc.NewMux()
	if err := resource.SRPCRegisterResourceService(mux, proxy); err != nil {
		t.Fatal(err)
	}
	service := resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()
	root := client.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Open the watch holding nothing and receive v1.
	watch, err := rootClient.NewStream(ctx, viewService, "Watch", srpc.NewRawMessage(nil, false))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watch.Close() }()
	recv := func() []byte {
		t.Helper()
		msg := srpc.NewRawMessage(nil, false)
		if err := watch.MsgRecv(msg); err != nil {
			t.Fatal(err)
		}
		return msg.GetData()
	}
	if got := recv(); string(got) != string(viewFrame("state", "v1")) {
		t.Fatalf("first message %x, want v1", got)
	}
	first.waitCalls(t, ctx, "Watch ")

	// After a restart the reopened request holds v1, and the client
	// receives v2 next.
	second := newBackend(2)
	first.kill()
	current.Store(second)
	if got := recv(); string(got) != string(viewFrame("changed", "v2")) {
		t.Fatalf("after the restart the client received %x, want v2", got)
	}
	second.waitCalls(t, ctx, "Watch v1")

	// After another the request holds v2, which the third backend does not
	// know: it answers v1 whole, which the client receives as the change.
	third := newBackend(3)
	second.kill()
	current.Store(third)
	if got := recv(); string(got) != string(viewFrame("state", "v1")) {
		t.Fatalf("after the second restart the client received %x, want v1", got)
	}
	third.waitCalls(t, ctx, "Watch v2")
	resumedMtx.Lock()
	defer resumedMtx.Unlock()
	if !slices.Equal(resumed, []bool{true, false}) {
		t.Fatalf("the proxy reported resumes %v, want quiet then sent", resumed)
	}
}
