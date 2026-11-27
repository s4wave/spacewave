//go:build !js

package spacewave_cli

import (
	"context"
	"math"
	"strings"

	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// serveUDP selects the UDP listener of serve: its address, the session it
// speaks for and the peers it admits.
type serveUDP struct {
	// listen is the host:port to listen on; empty starts no listener.
	listen string
	// sessionIdx is the session to speak for; zero selects the only one.
	sessionIdx uint
	// peers are the session peer IDs to admit, each optionally ID@HOST:PORT.
	peers []string
}

// serveUDPListener listens for one Session on udp.listen and serves the
// Resource service to other active sessions of its account until ctx ends:
// the session udp.sessionIdx selects, or the daemon's only one. The
// listener also admits the links of the session peer IDs in udp.peers,
// such as a Space member of another account, without granting them the
// Resource service. A peer given as ID@HOST:PORT is also dialed at that address
// and its link held: a test path for peers on a LAN without signaling. A locked
// Session delays the listener until it unlocks.
func serveUDPListener(ctx context.Context, le *logrus.Entry, client *sdkClient, udp serveUDP) error {
	// Select the Session the listener speaks for.
	if udp.sessionIdx > math.MaxUint32 {
		return errors.Errorf("--udp-session-index %d is out of range", udp.sessionIdx)
	}
	idx := uint32(udp.sessionIdx) //nolint:gosec // the MaxUint32 check above bounds the index.
	if idx == 0 {
		sessions, err := client.root.ListSessions(ctx)
		if err != nil {
			return errors.Wrap(err, "list sessions")
		}
		if len(sessions) != 1 {
			return errors.Errorf("--udp-listen needs --udp-session-index with %d sessions", len(sessions))
		}
		idx = sessions[0].GetSessionIndex()
	}
	sess, err := client.mountSession(ctx, idx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Hold the UDP transport and remote resource service open. A named peer
	// with no address is admitted when it dials in but is never dialed.
	peerAddrs := make(map[string]string, len(udp.peers))
	for _, named := range udp.peers {
		id, addr, _ := strings.Cut(named, "@")
		peerAddrs[id] = addr
	}
	pt, err := sess.OpenPeerTransport(ctx, &s4wave_session.AccessPeerTransportRequest{
		UdpListenAddr:  udp.listen,
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
