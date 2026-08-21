package inproc

import (
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// scheme is the address scheme prefix for inproc addresses.
const scheme = "inproc://"

// Addr is an in-process address identifying a local peer.
type Addr struct {
	// peerID is the addressed peer.
	peerID peer.ID
	// str is the cached string form of the address.
	str string
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
	// Require the in-process address scheme before decoding its peer ID.
	if !strings.HasPrefix(addr, scheme) {
		return nil, errors.Errorf("expected inproc prefix: %s", addr)
	}

	// Decode the peer identity embedded in the address.
	pid, err := peer.IDB58Decode(addr[len(scheme):])
	if err != nil {
		return nil, err
	}

	// Rebuild the typed address from the decoded peer identity.
	return NewAddr(pid), nil
}

// Network returns the network name.
func (a *Addr) Network() string {
	return "inproc"
}

// String returns the address string.
func (a *Addr) String() string {
	return a.str
}

// _ is a type assertion
var _ net.Addr = (*Addr)(nil)
