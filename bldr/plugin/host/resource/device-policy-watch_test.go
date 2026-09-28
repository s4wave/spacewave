package plugin_host_resource

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
)

// testDevicePolicySource supplies two complete revisions without polling.
type testDevicePolicySource struct{ changes chan []byte }

// WaitDevicePolicy emits the initial snapshot and awaits the next revision.
func (s *testDevicePolicySource) WaitDevicePolicy(ctx context.Context, last []byte) ([]byte, string, uint64, error) {
	if len(last) == 0 {
		return []byte{1}, "devices/self", 1, nil
	}
	select {
	case <-ctx.Done():
		return nil, "", 0, ctx.Err()
	case next := <-s.changes:
		return next, "devices/self", 2, nil
	}
}

// testDevicePolicyStream records host transport responses.
type testDevicePolicyStream struct {
	srpc.Stream
	ctx  context.Context
	sent chan *plugin_host.WatchDevicePolicyResponse
}

// Context returns the stream's cancellation boundary.
func (s *testDevicePolicyStream) Context() context.Context { return s.ctx }

// Send captures one complete policy response.
func (s *testDevicePolicyStream) Send(resp *plugin_host.WatchDevicePolicyResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp:
		return nil
	}
}

// SendAndClose captures the terminal response when used.
func (s *testDevicePolicyStream) SendAndClose(resp *plugin_host.WatchDevicePolicyResponse) error {
	return s.Send(resp)
}

// TestWatchDevicePolicyForwardsCurrentRevisionAndChanges checks the host's
// read-only stream and its cancellation path.
func TestWatchDevicePolicyForwardsCurrentRevisionAndChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	root := plugin_host_root.NewRoot()
	source := &testDevicePolicySource{changes: make(chan []byte, 1)}
	root.SetDevicePolicySource(source)
	resource := &PluginHostRoot{pluginID: "spacewave-core", hostRoot: root}
	stream := &testDevicePolicyStream{ctx: ctx, sent: make(chan *plugin_host.WatchDevicePolicyResponse, 2)}
	done := make(chan error, 1)
	go func() { done <- resource.WatchDevicePolicy(&plugin_host.WatchDevicePolicyRequest{}, stream) }()
	first := <-stream.sent
	if first.GetRevision() != 1 || first.GetDeviceObjectKey() != "devices/self" {
		t.Fatalf("initial policy = %+v", first)
	}
	source.changes <- []byte{2}
	second := <-stream.sent
	if second.GetRevision() != 2 || second.GetPolicy()[0] != 2 {
		t.Fatalf("changed policy = %+v", second)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("watch cancellation = %v", err)
	}
}

// TestWatchDevicePolicyRejectsOtherPlugins keeps the local Device policy
// outside unrelated plugin authority.
func TestWatchDevicePolicyRejectsOtherPlugins(t *testing.T) {
	resource := &PluginHostRoot{pluginID: "other-plugin", hostRoot: plugin_host_root.NewRoot()}
	stream := &testDevicePolicyStream{ctx: t.Context(), sent: make(chan *plugin_host.WatchDevicePolicyResponse, 1)}
	if err := resource.WatchDevicePolicy(&plugin_host.WatchDevicePolicyRequest{}, stream); err == nil {
		t.Fatal("non-core plugin received the Device policy watch")
	}
}
