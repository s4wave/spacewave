package provider_transfer

import (
	"context"
	"crypto/sha256"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// TargetHolds is what a target holds under the id of a Space a transfer
// would move there, as the transfer's source sees it.
type TargetHolds int

const (
	// TargetHoldsNothing means the target does not list the id.
	TargetHoldsNothing TargetHolds = iota
	// TargetHoldsArriving means a transfer from this source listed the Space
	// and has not written its state. The target lets nothing else write it.
	TargetHoldsArriving
	// TargetHoldsWritten means a transfer from this source wrote the Space
	// and nothing changed it since. TargetSpace.Head names the source World
	// that write copied.
	TargetHoldsWritten
	// TargetHoldsShared means the target lists the id as a Space shared with
	// its account.
	TargetHoldsShared
	// TargetHoldsOther means anything else: a Space created on the target,
	// moved from another source, or changed since a transfer wrote it.
	TargetHoldsOther
)

// TargetSpace is what a target holds under a Space's id, read before the
// transfer decides what to do with the Space.
type TargetSpace struct {
	// Holds is what the target holds.
	Holds TargetHolds
	// Head is the SHA-256 of the source World the transfer wrote, set when
	// Holds is TargetHoldsWritten.
	Head []byte
	// Expect is the target's record of the state it read, opaque to the
	// transfer. WriteSharedObjectState writes only while the target still
	// holds that state.
	Expect any
}

// TransferTarget provides write access to a provider account for transfer.
type TransferTarget interface {
	// GetBlockStore returns the block store ops for a shared object's block store.
	// Creates the block store if it does not exist.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
	// ReadSharedObject returns what the target holds under the id of a Space
	// from the source origin names.
	ReadSharedObject(ctx context.Context, le *logrus.Entry, sharedObjectID string, origin *TransferOrigin) (*TargetSpace, error)
	// AddSharedObject lists a Space the target does not list, arriving from
	// the source origin names, and returns what the target then holds. It
	// returns sobject.ErrSharedObjectExists, writing nothing, when the target
	// lists the id.
	// The ref should already have the target's provider resource ref.
	AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta, origin *TransferOrigin) (*TargetSpace, error)
	// WriteSharedObjectState replaces the state of a Space with a new lineage
	// owned solely by owner whose checkpoint holds stateData, and records
	// origin, whose head names that World, in the same write. It writes only
	// while the target holds the state expect read, and otherwise returns
	// ErrTargetChanged, writing nothing.
	WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, origin *TransferOrigin, expect *TargetSpace) error
}

// ErrTargetChanged is returned by WriteSharedObjectState when the target's
// Space changed after the transfer read it.
var ErrTargetChanged = errors.New("the Space changed on the target during the transfer")

// GetExpect returns the target's record of the state it read, nil for a nil
// TargetSpace.
func (s *TargetSpace) GetExpect() any {
	if s == nil {
		return nil
	}
	return s.Expect
}

// sameSource reports whether two origins name the same source account.
func sameSource(a, b *TransferOrigin) bool {
	return a.GetSourceProviderId() == b.GetSourceProviderId() && a.GetSourceAccountId() == b.GetSourceAccountId()
}

// worldHead returns the SHA-256 of the encoded World, which names it: replay
// encodes the same World to the same bytes.
func worldHead(world *space_world.InnerState) ([]byte, error) {
	data, err := world.MarshalVT()
	if err != nil {
		return nil, err
	}
	return encodedWorldHead(data), nil
}

// encodedWorldHead returns the head of the World data encodes.
func encodedWorldHead(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
