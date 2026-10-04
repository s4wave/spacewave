//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// remoteEnvVars are the environment variables that select a remote daemon.
var remoteEnvVars = []string{"SPACEWAVE_REMOTE_DAEMON"}

// remoteFlag returns the flag that sends commands to another session's daemon.
func remoteFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "remote-daemon",
		Usage:   "run against the daemon of another session of this account, as <peer-id>@<host:port>",
		EnvVars: remoteEnvVars,
	}
}

// connectRemoteFromContext returns local itself without --remote-daemon.
// Otherwise it opens the remote daemon's Resource service through local's
// Session transport over UDP; the Session key never leaves the local daemon.
// The returned client owns local.
func connectRemoteFromContext(ctx context.Context, c *cli.Context, local *sdkClient) (*sdkClient, error) {
	// Keep the local client when no remote daemon is selected.
	target, _ := lineageFlagValue(c, "remote-daemon")
	if target == "" {
		return local, nil
	}

	// Hand local to the remote client, or close it on failure.
	remote, err := connectRemoteDaemon(ctx, local, effectiveSessionIndex(c), target)
	if err != nil {
		local.close()
		return nil, err
	}
	return remote, nil
}

// connectRemoteDaemon dials target, <peer-id>@<host:port>, as the Session at
// sessionIdx on local.
func connectRemoteDaemon(ctx context.Context, local *sdkClient, sessionIdx uint32, target string) (*sdkClient, error) {
	// Parse the remote session and its UDP address.
	peerID, addr, ok := strings.Cut(target, "@")
	if !ok || peerID == "" || addr == "" {
		return nil, errors.Errorf("--remote-daemon %q: want <peer-id>@<host:port>", target)
	}

	// Link to the remote session through the local Session transport.
	sess, err := local.mountSession(ctx, sessionIdx)
	if err != nil {
		return nil, err
	}
	pt, err := sess.OpenPeerTransport(ctx, &s4wave_session.AccessPeerTransportRequest{
		UdpPeerAddrs: map[string]string{peerID: addr},
	})
	if err != nil {
		sess.Release()
		return nil, errors.Wrap(err, "open peer transport")
	}
	release := func() {
		pt.Release()
		sess.Release()
	}

	// Run the Resource client over the remote resource stream.
	stream, err := pt.Dial(ctx, peerID, string(s4wave_session.RemoteResourceProtocolID))
	if err != nil {
		release()
		return nil, errors.Wrapf(err, "dial remote daemon %s", peerID)
	}
	conn, ok := stream.(net.Conn)
	if !ok {
		_ = stream.Close()
		release()
		return nil, errors.New("remote stream is not a connection")
	}
	remote, err := buildSDKClient(ctx, conn)
	if err != nil {
		release()
		return nil, errors.Wrap(err, "remote daemon")
	}
	remote.release = func() {
		release()
		local.close()
	}
	return remote, nil
}

// effectiveSessionIndex returns the nearest --session-index value, or 1.
func effectiveSessionIndex(c *cli.Context) uint32 {
	value, _ := lineageFlagValue(c, "session-index")
	idx, err := strconv.ParseUint(value, 10, 32)
	if err != nil || idx == 0 {
		return 1
	}
	return uint32(idx)
}
