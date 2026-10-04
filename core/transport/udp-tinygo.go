//go:build tinygo

package transport

import (
	"context"
	"net"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// StartUDP reports that this build has no UDP transport.
func (t *SessionTransport) StartUDP(context.Context, string, map[peer.ID]string) (net.Addr, error) {
	return nil, errors.New("udp transport is unavailable in this build")
}
