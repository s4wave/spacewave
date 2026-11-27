//go:build tinygo

package transport

import (
	"context"
	"net"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// UDPConfig configures one UDP transport run under the Session's peer.
type UDPConfig struct {
	// ListenAddr is the host:port to listen on.
	ListenAddr string
	// Peers holds the remote peers whose links mount, with their addresses.
	Peers map[peer.ID]string
	// NamedOnly mounts only the links of named peers.
	NamedOnly bool
}

// StartUDP reports that this build has no UDP transport.
func (t *SessionTransport) StartUDP(context.Context, UDPConfig) (net.Addr, error) {
	return nil, errors.New("udp transport is unavailable in this build")
}
