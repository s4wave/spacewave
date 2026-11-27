package wasivm_plugin

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// updateSource serves the Display method as the WasiVM plugin does, but sends
// pre-encoded update bytes, so a test controls exactly what is on the wire.
type updateSource struct {
	update []byte
}

// InvokeMethod sends the update once and ends the stream.
func (s *updateSource) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != s4wave_vm.SRPCWasiVmDisplayServiceServiceID || methodID != "Display" {
		return false, nil
	}
	return true, strm.MsgSend(srpc.NewRawMessage(s.update, true))
}

// TestDisplayUpdateRelayedUnchanged checks that a hop delivers an update's
// bytes as the plugin encoded them. The hop forwards the Display RPC with
// srpc.NewClientInvoker, as the core plugin does for a plugin's resource.
// The update is written with its fields out of order, which decoding and
// encoding the update would put back in order.
func TestDisplayUpdateRelayedUnchanged(t *testing.T) {
	// Encode an update the way the plugin does, then append its screen,
	// which an encoder writes first.
	update := &s4wave_vm.WasiVmScene{
		Patches: []*s4wave_vm.WasiVmScenePatch{{
			Layer: 1, X: 10, Y: 20, Width: 256, Height: 256,
			Rgba: bytes.Repeat([]byte{1, 2, 3, 255}, 256*256),
		}},
	}
	encoded, err := update.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	screen, err := (&s4wave_vm.WasiVmScene{Screen: &s4wave_vm.WasiVmSceneScreen{Width: 640, Height: 480}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	wire := append(encoded, screen...)

	// Control: decoding and encoding the update puts the fields back in order.
	var decoded s4wave_vm.WasiVmScene
	if err := decoded.UnmarshalVT(wire); err != nil {
		t.Fatal(err)
	}
	if again, err := decoded.MarshalVT(); err != nil || bytes.Equal(again, wire) {
		t.Fatalf("the field order survived a decode and encode (err %v)", err)
	}

	// Relay the plugin's Display RPC through a client invoker.
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(&updateSource{update: wire})))
	relayClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewClientInvoker(pluginClient))))

	// The viewer receives the plugin's bytes without a decode on the way.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	strm, err := relayClient.NewStream(ctx, s4wave_vm.SRPCWasiVmDisplayServiceServiceID, "Display", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	got := srpc.NewRawMessage(nil, true)
	if err := strm.MsgRecv(got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.GetData(), wire) {
		t.Fatalf("the relayed update is %d bytes, want the plugin's %d bytes unchanged", len(got.GetData()), len(wire))
	}
}
