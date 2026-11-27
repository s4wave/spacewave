package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	"github.com/s4wave/spacewave/db/testbed"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/sirupsen/logrus"
)

// TestE1APForgeApproval: plugin-side code with the volume handle a release
// scheduler (instance key "") serves writes an ALLOWED record for another
// plugin's root into platform-account; the device's Approvals then reads it.
// A Space scheduler's plugin tries the same as a control.
func TestE1APForgeApproval(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	hostVol := tb.Volume
	storeID := "platform-account"

	approvals := plugin_approval.NewApprovals(tb.Bus, hostVol.GetID(), storeID)
	if _, err := tb.Bus.AddController(ctx, plugin_approval.NewController(le, approvals), nil); err != nil {
		t.Fatal(err.Error())
	}

	const peerID = "12D3KooWVictimPeerVictimPeerVictimPeerVictimPeer"
	forge := func(c *Controller, evilPlugin, target, root string) {
		vol := servePluginVolume(ctx, t, c, hostVol, evilPlugin)
		store, rel, err := vol.AccessObjectStore(ctx, storeID, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rel()
		err = plugin_approval.PutApproval(ctx, store, peerID, &s4wave_plugin_approval.PluginApproval{
			PluginId:     target,
			ManifestRoot: root,
			State:        s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	admits := func(target, root string) (bool, bool) {
		ok, err := approvals.Admits(ctx, peerID, target, root)
		if err != nil {
			t.Fatal(err.Error())
		}
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, werr := bldr_plugin.WaitPluginRootAdmitted(wctx, tb.Bus, peerID, target, root)
		return ok, werr == nil
	}

	// Release scheduler: instance key "".
	release := &Controller{conf: &Config{}}
	forge(release, "evil-plugin", "victim-plugin", "rootA")
	ok, dir := admits("victim-plugin", "rootA")
	if ok || dir {
		t.Logf("e1-ap-forge: forged: release-scheduler plugin wrote ALLOWED; Admits=%v AdmitPluginRoot(fresh)=%v", ok, dir)
	} else {
		t.Logf("e1-ap-forge: refused: release-scheduler plugin write not visible; Admits=%v AdmitPluginRoot=%v", ok, dir)
	}

	// Release scheduler running an instance under another key, as a hosted
	// Space's LoadPlugin(pid, SpaceId) does: newPluginVolume sees only conf.
	forge(release, "space-plugin", "victim-plugin", "rootB")
	ok, dir = admits("victim-plugin", "rootB")
	t.Logf("e1-ap-forge: hosted-instance on key-\"\" scheduler: Admits=%v AdmitPluginRoot(fresh)=%v", ok, dir)

	// Control: a Space scheduler's plugin.
	space := &Controller{conf: &Config{InstanceKey: "space-a"}}
	forge(space, "space-plugin", "victim-plugin", "rootC")
	ok, dir = admits("victim-plugin", "rootC")
	t.Logf("e1-ap-forge: control Space-scoped plugin: Admits=%v AdmitPluginRoot(fresh)=%v", ok, dir)
}
