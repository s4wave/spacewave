package spacewave_launcher

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestResolvePushedDistConfigRejectsOlderRev(t *testing.T) {
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	older := &DistConfig{
		ProjectId:  "spacewave",
		Rev:        41,
		ChannelKey: "stable",
	}
	encoded, err := EncodeSignedDistConfig(signerPriv, older)
	if err != nil {
		t.Fatal(err)
	}

	got, _, _, updated, err := ResolvePushedDistConfig(
		logrus.NewEntry(logrus.New()),
		[]byte(packedmsg.EncodePackedMessage(encoded)),
		[]peer.ID{signerPeer.GetPeerID()},
		"spacewave",
		42,
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("older rev should not update launcher info")
	}
	if got.GetRev() != 41 {
		t.Fatalf("parsed rev = %d, want 41", got.GetRev())
	}
}
