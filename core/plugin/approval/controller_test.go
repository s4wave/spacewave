package plugin_approval

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/sirupsen/logrus"
)

// testApprovalsStoreID is the device-local ObjectStore the tests use.
const testApprovalsStoreID = "test-plugin-approvals"

// watchAdmitted watches AdmitPluginRoot for one root on b and sends the count
// of admitting values after the first idle and on each change.
func watchAdmitted(t *testing.T, ctx context.Context, b bus.Bus, peerID, pluginID, root string) <-chan int {
	t.Helper()
	counts := make(chan int, 16)
	_, release, err := bus.ExecCollectValuesWatch[bldr_plugin.AdmitPluginRootValue](
		ctx,
		b,
		bldr_plugin.NewAdmitPluginRoot(peerID, pluginID, root),
		true,
		func(_ []error, vals []bldr_plugin.AdmitPluginRootValue) error {
			counts <- len(vals)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return counts
}

// waitAdmitted waits until counts reports want.
func waitAdmitted(t *testing.T, ctx context.Context, counts <-chan int, want int) {
	t.Helper()
	for {
		select {
		case got := <-counts:
			if got == want {
				return
			}
		case <-ctx.Done():
			t.Fatalf("admitting values never reached %d", want)
		}
	}
}

// TestApprovalGateFailsClosed checks that only an ALLOWED record of the asking
// peer admits a root, and that the watch follows each committed decision.
func TestApprovalGateFailsClosed(t *testing.T) {
	// Serve the approvals of one device-local store on a real bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	approvals := NewApprovals(tb.Bus, tb.Volume.GetID(), testApprovalsStoreID)
	rel, err := tb.Bus.AddController(ctx, NewController(tb.Logger, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	const peer, root = "peer-a", "root-1"
	putRaw := func(pluginID string, data []byte) {
		t.Helper()
		err := approvals.withStore(ctx, func(store volume.ObjectStoreHandle) error {
			return kvtx.RunTransaction(ctx, true,
				func(ctx context.Context) (kvtx.Tx, error) {
					return store.GetObjectStore().NewTransaction(ctx, true)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					return tx.Set(ctx, Key(peer, pluginID, root), data)
				},
			)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A missing record refuses.
	waitAdmitted(t, ctx, watchAdmitted(t, ctx, tb.Bus, peer, "missing", root), 0)

	// A record that does not decode refuses.
	putRaw("unreadable", []byte{0xff, 0xff, 0xff})
	if ok, err := approvals.Admits(ctx, peer, "unreadable", root); ok || err == nil {
		t.Fatalf("unreadable record: admitted %v, error %v", ok, err)
	}
	waitAdmitted(t, ctx, watchAdmitted(t, ctx, tb.Bus, peer, "unreadable", root), 0)

	// A state this build does not know refuses, and Set will not write one.
	unknown := &s4wave_plugin_approval.PluginApproval{PluginId: "unknown", ManifestRoot: root, State: 7}
	if err := approvals.Set(ctx, peer, unknown); err == nil {
		t.Fatal("Set wrote an unknown state")
	}
	data, err := unknown.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	putRaw("unknown", data)
	waitAdmitted(t, ctx, watchAdmitted(t, ctx, tb.Bus, peer, "unknown", root), 0)

	// ALLOWED admits the deciding peer only; DECLINED withdraws the value.
	counts := watchAdmitted(t, ctx, tb.Bus, peer, "colors", root)
	other := watchAdmitted(t, ctx, tb.Bus, "peer-b", "colors", root)
	waitAdmitted(t, ctx, counts, 0)
	waitAdmitted(t, ctx, other, 0)
	decide := func(state s4wave_plugin_approval.PluginApprovalState) {
		t.Helper()
		err := approvals.Set(ctx, peer, &s4wave_plugin_approval.PluginApproval{PluginId: "colors", ManifestRoot: root, State: state})
		if err != nil {
			t.Fatal(err)
		}
	}
	decide(s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	waitAdmitted(t, ctx, counts, 1)
	if ok, err := approvals.Admits(ctx, "peer-b", "colors", root); ok || err != nil {
		t.Fatalf("another peer: admitted %v, error %v", ok, err)
	}
	if ok, _ := approvals.Admits(ctx, "", "colors", root); ok {
		t.Fatal("an empty peer was admitted")
	}
	decide(s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_DECLINED)
	waitAdmitted(t, ctx, counts, 0)
	decide(s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED)
	waitAdmitted(t, ctx, counts, 1)
	decide(s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_TURNED_OFF)
	waitAdmitted(t, ctx, counts, 0)
	select {
	case n := <-other:
		if n != 0 {
			t.Fatal("one peer's decision admitted another peer")
		}
	default:
	}
}
