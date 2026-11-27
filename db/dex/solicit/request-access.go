package dex_solicit

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
)

// RequestAuthorizer acquires current read authority for an authenticated
// requesting peer. It gates incoming requests only: a peer without read authority
// may still hold blocks that an authorized requester needs.
// Success returns a non-nil RequestAccess, which the handler releases exactly once.
type RequestAuthorizer func(context.Context, *Config, peer.ID) (RequestAccess, error)

// RequestAccess retains the source of authority for one incoming request.
// Check and WaitRevoked may run concurrently, but both finish before Release.
type RequestAccess interface {
	// Check rejects a request when its peer no longer has read authority.
	Check() error
	// WaitRevoked returns an error when read authority is lost, its source fails,
	// or ctx ends. It must honor cancellation.
	WaitRevoked(ctx context.Context) error
	// Release releases the authority source after the request and its watch finish.
	Release()
}
