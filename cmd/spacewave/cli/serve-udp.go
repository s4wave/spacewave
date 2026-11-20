//go:build !js

package spacewave_cli

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// serveUDPListener listens for the daemon's only Session on addr and serves
// the Resource service to other active sessions of its account until ctx
// ends. The listener also admits the links of the session peer IDs in peers,
// such as a Space member of another account, without granting them the
// Resource service. A peer given as ID@HOST:PORT is also dialed at that address
// and its link held: a test path for peers on a LAN without signaling. A locked
// Session delays the listener until it unlocks.
func serveUDPListener(ctx context.Context, le *logrus.Entry, client *sdkClient, addr string, peers []string) error {
	// Select the one Session the listener speaks for.
	sessions, err := client.root.ListSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "list sessions")
	}
	if len(sessions) != 1 {
		return errors.Errorf("--udp-listen needs exactly one session, found %d", len(sessions))
	}
	sess, err := client.mountSession(ctx, sessions[0].GetSessionIndex())
	if err != nil {
		return err
	}
	defer sess.Release()

	// Hold the UDP transport and remote resource service open. A named peer
	// with no address is admitted when it dials in but is never dialed.
	peerAddrs := make(map[string]string, len(peers))
	for _, named := range peers {
		id, addr, _ := strings.Cut(named, "@")
		peerAddrs[id] = addr
	}
	pt, err := sess.OpenPeerTransport(ctx, &s4wave_session.AccessPeerTransportRequest{
		UdpListenAddr:  addr,
		UdpPeerAddrs:   peerAddrs,
		ServeResources: true,
	})
	if err != nil {
		return errors.Wrap(err, "open peer transport")
	}
	defer pt.Release()
	le.WithField("peer-id", pt.PeerID).
		WithField("udp-addr", pt.UDPAddr).
		Info("serving account sessions over UDP")
	<-ctx.Done()
	return nil
}
