package cdn_sharedobject

import (
	"github.com/aperturerobotics/controllerbus/bus"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CdnSharedObjectOptions configures a CdnSharedObject.
type CdnSharedObjectOptions struct {
	// SpaceID is the CDN Space ULID this SharedObject represents.
	SpaceID string
	// Bus is the controllerbus used by session consumers of GetBus().
	Bus bus.Bus
	// PeerID is the local peer id. An empty value is allowed for anonymous
	// mounts on local-only bootstraps where no session identity exists yet.
	PeerID peer.ID
	// Logger logs the driver of an attached body, or is nil to discard.
	Logger *logrus.Entry
	// BlockStore reads the CDN Space and tracks its root pointer. It is
	// either the transport-owning CdnBlockStore or a SuppliedBlockStore reading
	// through a process that already owns that transport.
	BlockStore cdn_bstore.RootBlockStore
}
