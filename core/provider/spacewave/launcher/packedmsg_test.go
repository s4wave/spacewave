package spacewave_launcher

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/net/peer"
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
		ProjectId:  "bldr-test",
		Rev:        42,
		ChannelKey: "stable",
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

func TestParseDistConfigPackedMsgRejectsInvalidValidator(t *testing.T) {
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	config := &DistConfig{
		ProjectId:  "bldr-test",
		Rev:        42,
		ChannelKey: "stable",
	}
	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeSignedDistConfig(signerPriv, config)
	if err != nil {
		t.Fatal(err)
	}
	packedMsg := packedmsg.EncodePackedMessage(encoded)
	if _, _, _, err := ParseDistConfigPackedMsg(logrus.NewEntry(logrus.New()), []byte(packedMsg), []peer.ID{otherPeer.GetPeerID()}, config.GetProjectId()); err == nil {
		t.Fatal("expected invalid validator to be rejected")
	}
}

func TestParseDistConfigPackedMsgRejectsMissingChannelKey(t *testing.T) {
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	config := &DistConfig{
		ProjectId: "bldr-test",
		Rev:       42,
	}
	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeSignedDistConfig(signerPriv, config); err == nil {
		t.Fatal("expected missing channel_key to be rejected before signing")
	}
}
