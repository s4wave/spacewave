package spacewave_launcher_gossip

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// testLauncher verifies pushed configs against trusted signers and adopts
// newer revisions, like the launcher controller.
type testLauncher struct {
	// trusted is the set of accepted signers.
	trusted []peer.ID
	// bcast guards the fields below.
	bcast broadcast.Broadcast
	// info is the current launcher info.
	info *spacewave_launcher.LauncherInfo
	// pushes counts Push calls.
	pushes int
}

// Snapshot returns the current launcher info.
func (l *testLauncher) Snapshot() (*spacewave_launcher.LauncherInfo, <-chan struct{}) {
	var info *spacewave_launcher.LauncherInfo
	var waitCh <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		info = l.info.CloneVT()
		waitCh = getWaitCh()
	})
	return info, waitCh
}

// Push verifies msg and adopts it if newer.
func (l *testLauncher) Push(_ context.Context, msg string) (bool, error) {
	conf, confMsg, _, err := spacewave_launcher.ParseDistConfigPackedMsg(nil, []byte(msg), l.trusted, "spacewave")
	var adopted bool
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.pushes++
		if err == nil && conf.GetRev() > l.info.GetDistConfig().GetRev() {
			l.info = &spacewave_launcher.LauncherInfo{DistConfig: conf, DistConfigMsg: confMsg}
			adopted = true
		}
		broadcast()
	})
	return adopted, err
}

// waitFor blocks until cond holds for the launcher info and push count.
func (l *testLauncher) waitFor(ctx context.Context, t *testing.T, cond func(*spacewave_launcher.LauncherInfo, int) bool) {
	t.Helper()
	for {
		var ok bool
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ok = cond(l.info, l.pushes)
			waitCh = getWaitCh()
		})
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-waitCh:
		}
	}
}

// newTestLauncher returns a launcher holding conf signed by signer.
func newTestLauncher(t *testing.T, trusted []peer.ID, signer peer.Peer, rev uint64) *testLauncher {
	t.Helper()
	priv, err := signer.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	conf := &spacewave_launcher.DistConfig{ProjectId: "spacewave", Rev: rev, ChannelKey: "stable"}
	encoded, err := spacewave_launcher.EncodeSignedDistConfig(priv, conf)
	if err != nil {
		t.Fatal(err)
	}
	return &testLauncher{
		trusted: trusted,
		info: &spacewave_launcher.LauncherInfo{
			DistConfig:    conf,
			DistConfigMsg: packedmsg.EncodePackedMessage(encoded),
		},
	}
}

// runExchange connects a and b with an in-memory stream.
func runExchange(ctx context.Context, a, b *testLauncher) {
	le := logrus.NewEntry(logrus.New())
	connA, connB := net.Pipe()
	go func() {
		_ = exchange(ctx, le, stream_packet.NewSession(connA, maxMessageSize), a)
	}()
	go func() {
		_ = exchange(ctx, le, stream_packet.NewSession(connB, maxMessageSize), b)
	}()
}

// TestExchangeAdoptsNewerConfig checks that the older side adopts the newer
// side's signed config and the newer side keeps its own.
func TestExchangeAdoptsNewerConfig(t *testing.T) {
	signer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []peer.ID{signer.GetPeerID()}
	newer := newTestLauncher(t, trusted, signer, 2)
	older := newTestLauncher(t, trusted, signer, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	runExchange(ctx, newer, older)

	older.waitFor(ctx, t, func(info *spacewave_launcher.LauncherInfo, _ int) bool {
		return info.GetDistConfig().GetRev() == 2
	})
	olderInfo, _ := older.Snapshot()
	newerInfo, _ := newer.Snapshot()
	if olderInfo.GetDistConfigMsg() != newerInfo.GetDistConfigMsg() {
		t.Fatal("adopted config differs from the offered message")
	}
	newer.waitFor(ctx, t, func(_ *spacewave_launcher.LauncherInfo, pushes int) bool {
		if pushes != 0 {
			t.Fatalf("newer side pushed %d configs, want 0", pushes)
		}
		return true
	})
}

// TestExchangeRejectsUntrustedConfig checks that a higher revision signed by
// an untrusted key is requested once and not adopted.
func TestExchangeRejectsUntrustedConfig(t *testing.T) {
	trustedSigner, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	forger, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []peer.ID{trustedSigner.GetPeerID()}
	forged := newTestLauncher(t, []peer.ID{forger.GetPeerID()}, forger, 5)
	local := newTestLauncher(t, trusted, trustedSigner, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	runExchange(ctx, forged, local)

	local.waitFor(ctx, t, func(_ *spacewave_launcher.LauncherInfo, pushes int) bool {
		return pushes == 1
	})
	localInfo, _ := local.Snapshot()
	if rev := localInfo.GetDistConfig().GetRev(); rev != 1 {
		t.Fatalf("rev = %d, want 1", rev)
	}
}
