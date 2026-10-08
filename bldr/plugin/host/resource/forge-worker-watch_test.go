package plugin_host_resource

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
)

// testForgeWorkerSource supplies two complete revisions without polling.
type testForgeWorkerSource struct{ changes chan []byte }

// WaitForgeWorker emits the initial revision and awaits the next one.
func (s *testForgeWorkerSource) WaitForgeWorker(ctx context.Context, last uint64) ([]byte, string, uint64, error) {
	if last == 0 {
		return []byte{1}, "devices/self", 1, nil
	}
	select {
	case <-ctx.Done():
		return nil, "", 0, ctx.Err()
	case next := <-s.changes:
		return next, "devices/self", last + 1, nil
	}
}

// testForgeWorkerStream records host transport responses.
type testForgeWorkerStream struct {
	srpc.Stream
	ctx  context.Context
	sent chan *plugin_host.WatchForgeWorkerResponse
}

// Context returns the stream's cancellation boundary.
func (s *testForgeWorkerStream) Context() context.Context { return s.ctx }

// Send captures one complete declaration response.
func (s *testForgeWorkerStream) Send(resp *plugin_host.WatchForgeWorkerResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp:
		return nil
	}
}

// SendAndClose captures the terminal response when used.
func (s *testForgeWorkerStream) SendAndClose(resp *plugin_host.WatchForgeWorkerResponse) error {
	return s.Send(resp)
}

// TestWatchForgeWorkerForwardsCurrentRevisionAndChanges checks the host's
// read-only stream and its cancellation path.
func TestWatchForgeWorkerForwardsCurrentRevisionAndChanges(t *testing.T) {
	// Connect the host root to a Forge Worker source and response stream.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	root := plugin_host_root.NewRoot()
	source := &testForgeWorkerSource{changes: make(chan []byte, 1)}
	root.SetForgeWorkerSource(source)
	resource := &PluginHostRoot{pluginID: "spacewave-core", hostRoot: root}
	stream := &testForgeWorkerStream{ctx: ctx, sent: make(chan *plugin_host.WatchForgeWorkerResponse, 2)}

	// Start the Forge Worker watch and verify its initial revision.
	done := make(chan error, 1)
	go func() { done <- resource.WatchForgeWorker(&plugin_host.WatchForgeWorkerRequest{}, stream) }()
	first := <-stream.sent
	if first.GetRevision() != 1 || first.GetDeviceObjectKey() != "devices/self" {
		t.Fatalf("initial declaration = %+v", first)
	}

	// Publish the next revision and verify stream delivery.
	source.changes <- []byte{2}
	second := <-stream.sent
	if second.GetRevision() != 2 || second.GetDeclaration()[0] != 2 {
		t.Fatalf("changed declaration = %+v", second)
	}

	// Cancel the Forge Worker watch and verify its terminal result.
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("watch cancellation = %v", err)
	}
}

// TestWatchForgeWorkerRejectsOtherPlugins keeps the Forge Worker declaration
// outside unrelated plugin authority.
func TestWatchForgeWorkerRejectsOtherPlugins(t *testing.T) {
	resource := &PluginHostRoot{pluginID: "other-plugin", hostRoot: plugin_host_root.NewRoot()}
	stream := &testForgeWorkerStream{ctx: t.Context(), sent: make(chan *plugin_host.WatchForgeWorkerResponse, 1)}
	if err := resource.WatchForgeWorker(&plugin_host.WatchForgeWorkerRequest{}, stream); err == nil {
		t.Fatal("non-core plugin received the Forge Worker watch")
	}
}
