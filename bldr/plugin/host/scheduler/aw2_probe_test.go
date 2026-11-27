package plugin_host_scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestAw2Probe shows the swap state a canceled admission pass leaves behind
// after a newer pass settled.
func TestAw2Probe(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	rel, err := tb.Bus.AddController(ctx, bldr_manifest_trust.NewController(bldr_manifest_trust.NewSet()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	c := &Controller{
		le: le, bus: tb.Bus, conf: &Config{PeerId: "host"},
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
	cand := func(rev uint64) *executePluginArgs {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte{0xa0, byte(rev)})
		if err != nil {
			t.Fatal(err)
		}
		ref := newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef()
		ref.RootRef = &block.BlockRef{Hash: h}
		return &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, rev: rev,
			manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref},
			installation:     &installedManifests{peerID: "peer"}}
	}
	// A newer pass settled; then the old pass, already canceled, runs its loop.
	instance.setSwapSettled(ctx, time.Now())
	oldCtx, oldCancel := context.WithCancel(ctx)
	oldCancel()
	got, err := instance.admittedCandidates(oldCtx, []*executePluginArgs{cand(1), cand(2)})
	var swap *plugin.PluginSwap
	for _, st := range c.pluginStatusCtr.GetValue().GetExecutions() {
		swap = st.GetSwap()
	}
	fmt.Printf("AW2PROBE: canceled pass returned %d, err %v; swap now %v\n", len(got), err, swap)
}
