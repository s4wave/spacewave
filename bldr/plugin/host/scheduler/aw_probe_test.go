package plugin_host_scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	"github.com/sirupsen/logrus"
)

// TestAwProbe asks whether a hosted Space plugin whose root the person
// approved on the spacewave-core plugin bus starts on the release host bus.
func TestAwProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Host bus as dist/entrypoint/bus.go builds it: trust controller, empty set
	// (the approved root is not a trusted or catalog root).
	host, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Release()
	relTrust, err := host.Bus.AddController(ctx, bldr_manifest_trust.NewController(bldr_manifest_trust.NewSet()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relTrust()

	// spacewave-core plugin bus: approvals controller as
	// core/resource/root/controller/controller.go:197 adds it.
	core, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Release()
	approvals := plugin_approval.NewApprovals(core.Bus, core.Volume.GetID(), "probe-approvals")
	relAppr, err := core.Bus.AddController(ctx, plugin_approval.NewController(le, approvals), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAppr()

	// The candidate root and the person's approval for the session peer.
	const peerID = "session-peer"
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{0xa0, 7})
	if err != nil {
		t.Fatal(err)
	}
	ref := newTestManifestRef("colors", "js", 7, "candidate").GetManifestRef()
	ref.RootRef = &block.BlockRef{Hash: h}
	root := manifestRootKey(ref)
	if err := approvals.Set(ctx, peerID, &s4wave_plugin_approval.PluginApproval{
		PluginId: "colors", ManifestRoot: root,
		State: s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED,
	}); err != nil {
		t.Fatal(err)
	}

	// Sanity: the approval admits the root on the spacewave-core bus.
	_, _, aref, err := bus.ExecWaitValue[plugin.AdmitPluginRootValue](ctx, core.Bus,
		plugin.NewAdmitPluginRoot(peerID, "colors", root), bus.ReturnIfIdle(true), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if aref == nil {
		t.Fatal("approval does not admit the root on the core bus")
	}
	aref.Release()

	// Host scheduler as newReleaseSchedulerConfig builds it: scheduler key "",
	// host peer, no RequireAdmission.
	conf := &Config{PeerId: "host-peer"}
	c := &Controller{
		le: le, bus: host.Bus, conf: conf,
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)

	// The load HandleLoadPluginRpc builds from the Space's LoadPlugin: the
	// installation peer is the session peer, with the selected manifest.
	args := &executePluginArgs{
		pluginHost:       &testPluginHost{id: "test"},
		rev:              7,
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref},
		installation:     &installedManifests{peerID: peerID},
	}

	waitCtx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	got, err := instance.admittedCandidates(waitCtx, []*executePluginArgs{args})
	if err == nil && len(got) != 0 {
		fmt.Println("AWPROBE: started")
		return
	}
	var swap *plugin.PluginSwap
	for _, st := range c.pluginStatusCtr.GetValue().GetExecutions() {
		swap = st.GetSwap()
	}
	fmt.Printf("AWPROBE: waits: %v (err %v)\n", swap, err)

	// Control: the same approvals controller on the host bus admits the root,
	// so the wait above comes from the missing host-to-core path alone.
	relCtl, err := host.Bus.AddController(ctx, plugin_approval.NewController(le, plugin_approval.NewApprovals(core.Bus, core.Volume.GetID(), "probe-approvals")), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relCtl()
	ctlCtx, stop2 := context.WithTimeout(ctx, 5*time.Second)
	defer stop2()
	got, err = instance.admittedCandidates(ctlCtx, []*executePluginArgs{args})
	fmt.Printf("AWPROBE control (approvals on host bus): admitted %d, err %v\n", len(got), err)
}
