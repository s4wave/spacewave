package spacewave_launcher_controller

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestDistConfigAuthority checks the same signature and channel contract for
// embedded, stored, package-shipped and endpoint-fetched configurations.
func TestDistConfigAuthority(t *testing.T) {
	trusted, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		channel string
		signer  peer.Peer
		wantErr string
	}{
		{"staging", "staging", trusted, ""},
		{"stable", "stable", trusted, "does not match host channel"},
		{"wrong-signer", "staging", other, "signature"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Sign the actual wire configuration, including the wrong-channel case.
			priv, err := test.signer.GetPrivKey(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := spacewave_launcher.EncodeSignedDistConfig(priv, &spacewave_launcher.DistConfig{
				ProjectId: "spacewave", Rev: 1, ChannelKey: test.channel,
			})
			if err != nil {
				t.Fatal(err)
			}
			packed := packedmsg.EncodePackedMessage(encoded)
			conf := &Config{ProjectId: "spacewave", ChannelKey: "staging", InitDistConfig: packed}
			peers := []peer.ID{trusted.GetPeerID()}
			ctrl := &Controller{conf: conf, distPeerIDs: peers, le: logrus.NewEntry(logrus.New())}
			_, _, _, storedErr := ctrl.parseDistConf([]byte(packed))
			_, _, _, initErr := conf.ParseInitDistConfig(conf.ProjectId, peers)
			for path, err := range map[string]error{"stored/fetched": storedErr, "embedded": initErr} {
				if test.wantErr == "" {
					if err != nil {
						t.Fatalf("%s: %v", path, err)
					}
					continue
				}
				if err == nil || (test.name != "wrong-signer" && !strings.Contains(err.Error(), test.wantErr)) {
					t.Fatalf("%s: got %v, want rejection %q", path, err, test.wantErr)
				}
			}
		})
	}
}
