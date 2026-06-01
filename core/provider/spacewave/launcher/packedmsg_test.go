package spacewave_launcher

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
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
		LauncherConfigSet: map[string]*configset_proto.ControllerConfig{
			"release-world-cdn-store": {
				Id:     "spacewave/cdn/bstore",
				Rev:    42,
				Config: []byte("encoded release cdn config"),
			},
		},
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

func TestE2EReleaseWASMInitDistConfigFixtureParses(t *testing.T) {
	initDistConfig := readBldrStarString(t, "E2E_RELEASE_WASM_INIT_DIST_CONFIG")
	distPeerIDStr := readBldrStarString(t, "E2E_RELEASE_WASM_DIST_PEER_ID")
	distPeerID, err := peer.IDB58Decode(distPeerIDStr)
	if err != nil {
		t.Fatal(err)
	}

	conf, _, confPeer, err := ParseDistConfigPackedMsg(logrus.NewEntry(logrus.New()), []byte(initDistConfig), []peer.ID{distPeerID}, "spacewave")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := conf.GetProjectId(), "spacewave"; got != want {
		t.Fatalf("project id=%q want %q", got, want)
	}
	if got := confPeer.String(); got != distPeerIDStr {
		t.Fatalf("dist peer id=%q want %q", got, distPeerIDStr)
	}
	if conf.GetRev() == 0 {
		t.Fatal("expected non-zero dist config revision")
	}
	if conf.GetChannelKey() == "" {
		t.Fatal("expected channel key")
	}
}

func TestDistConfigKeyDerivationFixture(t *testing.T) {
	initDistConfig := readBldrStarString(t, "E2E_RELEASE_WASM_INIT_DIST_CONFIG")
	packedMsgs, _ := packedmsg.FindPackedMessages(initDistConfig)
	if len(packedMsgs) != 1 {
		t.Fatalf("packed message count=%d want 1", len(packedMsgs))
	}
	signedMsg := &peer.SignedMsg{}
	if err := signedMsg.UnmarshalVT(packedMsgs[0]); err != nil {
		t.Fatal(err)
	}
	key, nonce, err := deriveDistConfigKey(signedMsg.GetFromPeerId(), signedMsg.GetSignature().GetHashType(), "spacewave")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := signedMsg.GetFromPeerId(), readBldrStarString(t, "E2E_RELEASE_WASM_DIST_PEER_ID"); got != want {
		t.Fatalf("signed from peer=%q want %q", got, want)
	}
	if got, want := signedMsg.GetSignature().GetHashType().String(), "HashType_SHA256"; got != want {
		t.Fatalf("signature hash type=%q want %q", got, want)
	}
	if got, want := hex.EncodeToString(key), "511ad49309b02c6cfda84f6eaba60089b795ddd4125f48fba29429cdbaac11a2"; got != want {
		t.Fatalf("derived key=%s want %s", got, want)
	}
	if got, want := hex.EncodeToString(nonce), "3b0e8b955cf88591b3ae2e6dad74d3f3005378850d195fdf"; got != want {
		t.Fatalf("derived nonce=%s want %s", got, want)
	}
}

func readBldrStarString(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + ` = "([^"]+)"$`)
	match := re.FindSubmatch(data)
	if match == nil {
		t.Fatalf("missing %s in bldr.star", name)
	}
	return string(match[1])
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

func TestParseDistConfigPackedMsgHandlesNilLoggerForInvalidPackedMsg(t *testing.T) {
	packedMsg := packedmsg.EncodePackedMessage([]byte("not a signed dist config"))
	if _, _, _, err := ParseDistConfigPackedMsg(nil, []byte(packedMsg), nil, "bldr-test"); err == nil {
		t.Fatal("expected invalid packed message to be rejected")
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

func TestEncodeSignedDistConfigRejectsInvalidLauncherConfigSet(t *testing.T) {
	signerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	config := &DistConfig{
		ProjectId:  "bldr-test",
		Rev:        42,
		ChannelKey: "stable",
		LauncherConfigSet: map[string]*configset_proto.ControllerConfig{
			"missing-id": {
				Rev: 42,
			},
		},
	}
	signerPriv, err := signerPeer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeSignedDistConfig(signerPriv, config); err == nil {
		t.Fatal("expected invalid launcher_config_set to be rejected before signing")
	}
}
