package spacewave_launcher_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestPushDistConfRejectsOlderRev(t *testing.T) {
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	older := &spacewave_launcher.DistConfig{
		ProjectId:  "spacewave",
		Rev:        41,
		ChannelKey: "stable",
	}
	encoded, err := spacewave_launcher.EncodeSignedDistConfig(signerPriv, older)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := &Controller{
		le:          logrus.NewEntry(logrus.New()),
		conf:        &Config{ProjectId: "spacewave"},
		distPeerIDs: []peer.ID{signerPeer.GetPeerID()},
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:  "spacewave",
					Rev:        42,
					ChannelKey: "stable",
				},
			},
		),
	}

	got, _, _, updated, prevRev, err := ctrl.PushDistConf(context.Background(), []byte(packedmsg.EncodePackedMessage(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("older rev should not update launcher info")
	}
	if prevRev != 42 {
		t.Fatalf("prev rev = %d, want 42", prevRev)
	}
	if got.GetRev() != 41 {
		t.Fatalf("parsed rev = %d, want 41", got.GetRev())
	}
	if ctrl.launcherInfoCtr.GetValue().GetDistConfig().GetRev() != 42 {
		t.Fatal("current dist config was downgraded")
	}
}
