package bldr_launcher

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/util/packedmsg"
	"github.com/sirupsen/logrus"
)

// TestPackDistConfig tests encrypting and decrypting DistConfig.
func TestPackDistConfig(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// NOTE: this peer private key is used for testing only.
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	signerPeerID := signerPeer.GetPeerID()

	config := &DistConfig{
		ProjectId: "bldr-test",
		Rev:       42,
	}

	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	encoded, err := EncodeSignedDistConfig(signerPriv, config)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log("successfully encoded dist config")

	// test packedmsg
	packedMsg := packedmsg.EncodePackedMessage(encoded)
	packedMsgInJunk := "demand to see life's manager! " + packedMsg + " oh, I like this guy!"
	packedMsgs, _ := packedmsg.FindPackedMessages(packedMsgInJunk)
	if len(packedMsgs) != 1 {
		t.Fail()
	}
	if !bytes.Equal(packedMsgs[0], encoded) {
		t.Fail()
	}
	t.Logf("packed message: %s", packedMsg)

	conf, foundPackedMsg, foundPeer, err := ParseDistConfigPackedMsg(le, []byte(packedMsg), []peer.ID{signerPeerID}, config.GetProjectId())
	if err != nil {
		t.Fatal(err.Error())
	}
	foundPackedMsg = strings.TrimSpace(foundPackedMsg)
	if foundPackedMsg != packedMsg || !foundPeer.MatchesPublicKey(signerPeer.GetPubKey()) {
		t.Fail()
	}
	if !conf.EqualMessageVT(config) {
		t.Fail()
	}
}
