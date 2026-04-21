package inproc

import (
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

var scheme = "inproc://"

type Addr struct {
	peerID peer.ID
	str    string
}

// NewAddr builds a new Addr
func NewAddr(peerID peer.ID) *Addr {
	return &Addr{
		peerID: peerID,
		str:    scheme + peerID.String(),
	}
}

// ParseAddr parses an address.
func ParseAddr(addr string) (net.Addr, error) {
	if !strings.HasPrefix(addr, scheme) {
		return nil, errors.Errorf("expected inproc prefix: %s", addr)
	}
	pid, err := peer.IDB58Decode(addr[len(scheme):])
	if err != nil {
		return nil, err
	}
	return NewAddr(pid), nil
}

func (a *Addr) Network() string {
	return "inproc"
}

func (a *Addr) String() string {
	return a.str
}

// _ is a type assertion
var _ net.Addr = ((*Addr)(nil))
