package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_trust "github.com/s4wave/spacewave/bldr/manifest/trust"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_admit "github.com/s4wave/spacewave/bldr/plugin/admit"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// alwaysAdmit answers every PluginRootAdmission watch with admitted.
type alwaysAdmit struct{}

// WatchPluginRootAdmitted sends admitted and holds the stream open.
func (alwaysAdmit) WatchPluginRootAdmitted(
	_ *bldr_plugin_admit.WatchPluginRootAdmittedRequest,
	strm bldr_plugin_admit.SRPCPluginRootAdmission_WatchPluginRootAdmittedStream,
) error {
	if err := strm.Send(&bldr_plugin_admit.WatchPluginRootAdmittedResponse{Admitted: true}); err != nil {
		return err
	}
	<-strm.Context().Done()
	return nil
}

// TestExecControllerCannotStartAdmission checks that a plugin cannot make the
// host admit a root by asking PluginHost.ExecController for an admission
// controller declaring itself as the owner and answering the watch. No factory
// builds one: the host constructs its only admission controllers in code. The
// host bus has an empty trust set and the owner's admit controller with no
// approvals behind it.
func TestExecControllerCannotStartAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	const peerID, ownerID, evilID = "session-peer", "spacewave-core", "evil"

	// Build the host bus as the release entrypoint does: the trust set empty
	// and the owner's admit controller built in code.
	host, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Release()

	// The evil plugin runs on the host and serves PluginRootAdmission.
	evilMux := srpc.NewMux()
	if err := bldr_plugin_admit.SRPCRegisterPluginRootAdmission(evilMux, alwaysAdmit{}); err != nil {
		t.Fatal(err)
	}
	evil := ownerPlugin{srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(evilMux)))}
	ctrls := []controller.Controller{
		bldr_manifest_trust.NewController(bldr_manifest_trust.NewSet()),
		bldr_plugin_admit.NewController(le, host.Bus, ownerID),
		callback.NewCallbackController(
			controller.NewInfo("test/evil", controller.MustParseVersion("0.0.1"), ""),
			nil,
			func(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
				d, ok := inst.GetDirective().(plugin.LoadPlugin)
				if !ok || d.LoadPluginID() != evilID {
					return nil, nil
				}
				return directive.R(directive.NewValueResolver([]plugin.RunningPlugin{evil}), nil)
			},
			nil,
		),
	}
	for _, ctrl := range ctrls {
		rel, err := host.Bus.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer rel()
	}

	// The evil plugin's own PluginHost service, as buildPluginMux registers it.
	hostSrv := bldr_plugin_host.NewPluginHostServer(ctx, host.Bus, le, bldr_plugin_host.PluginHostServerConfig{
		PluginID:         evilID,
		ManifestSnapshot: &manifest.ManifestSnapshot{},
	})
	hostMux := srpc.NewMux()
	if err := plugin.SRPCRegisterPluginHost(hostMux, hostSrv); err != nil {
		t.Fatal(err)
	}
	hostClient := plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostMux))))

	// The evil plugin asks for a second admit controller declaring itself
	// as the owner. The controller must never run. The request may fail or
	// wait for a factory.
	execCtx, stopExec := context.WithTimeout(ctx, 2*time.Second)
	defer stopExec()
	strm, err := hostClient.ExecController(execCtx, &controller_exec.ExecControllerRequest{
		ConfigSet: &configset_proto.ConfigSet{Configs: map[string]*configset_proto.ControllerConfig{
			"admit-via-evil": {Id: bldr_plugin_admit.ControllerID, Rev: 1, Config: []byte(`{"pluginId":"evil"}`)},
		}},
	})
	if err == nil {
		defer strm.Close()
		for {
			resp, err := strm.Recv()
			if err != nil {
				break
			}
			if resp.GetStatus() == controller_exec.ControllerStatus_ControllerStatus_RUNNING {
				t.Fatal("exec controller started an admission controller")
			}
			if resp.GetStatus() == controller_exec.ControllerStatus_ControllerStatus_ERROR {
				break
			}
		}
	}

	// A root no one trusted or approved, loaded for the session peer.
	c := &Controller{
		le: le, bus: host.Bus, conf: &Config{PeerId: "host-peer", AdmissionPluginId: ownerID},
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	c.setPluginStatus("colors", "", plugin.PluginState_PluginState_REQUESTED)
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("unapproved root"))
	if err != nil {
		t.Fatal(err)
	}
	ref := newTestManifestRef("colors", "js", 1, "candidate").GetManifestRef()
	ref.RootRef = &block.BlockRef{Hash: h}
	args := &executePluginArgs{
		pluginHost:       &testPluginHost{id: "test"},
		rev:              1,
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref},
		installation:     &installedManifests{peerID: peerID},
	}

	// One admission pass that gives up after a moment.
	passCtx, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	got, _ := instance.admittedCandidates(passCtx, []*executePluginArgs{args})
	if len(got) != 0 {
		t.Fatalf("unapproved root %s admitted for %q via a plugin's ExecController", manifestRootKey(ref), peerID)
	}
}
