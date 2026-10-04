//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// serveUDPListener listens for the daemon's only Session on addr and serves
// the Resource service to other active sessions of its account until ctx
// ends. A locked Session delays the listener until it unlocks.
func serveUDPListener(ctx context.Context, le *logrus.Entry, client *sdkClient, addr string) error {
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

	// Hold the UDP transport and remote resource service open.
	pt, err := sess.OpenPeerTransport(ctx, &s4wave_session.AccessPeerTransportRequest{
		UdpListenAddr:  addr,
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
