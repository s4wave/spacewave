package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
)

// MaxConfigSuffixEntries bounds one peer catch-up transaction.
const MaxConfigSuffixEntries = 4096

// MaxConfigSuffixBytes bounds serialized configuration history held during catch-up.
const MaxConfigSuffixBytes = 8 * 1024 * 1024

// ErrConfigHistoryUnavailable requires a trusted recovery source instead of peer catch-up.
var ErrConfigHistoryUnavailable = errors.New("shared object configuration history requires recovery")

// ErrParticipantRevoked reports a committed removal of local readable participation.
var ErrParticipantRevoked = errors.New("shared object participant access revoked")

// SOHostSyncFuncs supplies provider persistence for authenticated peer import.
// Configure these callbacks when constructing the host, before publishing it.
type SOHostSyncFuncs struct {
	// Lock imports accepted peer state without publishing a local root to a server.
	// When nil, the host uses its ordinary local persistence lock.
	Lock SOStateLockFunc
	// CheckpointLock atomically installs an externally authenticated invitation checkpoint.
	// It is never used for peer synchronization or an unauthenticated recovery candidate.
	CheckpointLock SOStateLockFunc
	// History returns a bounded oldest-first suffix between two exact head hashes.
	// It returns ErrConfigHistoryUnavailable for missing or divergent history.
	History func(context.Context, string, []byte, []byte) ([]*SOConfigChange, error)
	// Entry returns the retained configuration change addressed by its hash, or
	// nil when it is not retained.
	Entry func(context.Context, string, []byte) (*SOConfigChange, error)
	// WaitDurable waits, without forcing a flush, until every state write
	// completed before the call is durable. When nil, state writes are durable
	// when they return.
	WaitDurable func(context.Context) error
}

// ReadConfigSuffix follows retained hash-addressed entries from target to base.
// The caller provides one consistent read scope. Missing history fails closed;
// returned entries remain untrusted until VerifyConfigChainSuffix accepts them.
func ReadConfigSuffix(ctx context.Context, base, target []byte, read func(context.Context, []byte) (*SOConfigChange, error)) ([]*SOConfigChange, error) {
	// An empty receiver head cannot supply a trust anchor.
	if len(base) == 0 || len(target) == 0 {
		return nil, ErrConfigHistoryUnavailable
	}

	// Bound both traversal work and retained bytes, including cyclic corrupt history.
	var entries []*SOConfigChange
	var size int
	for !bytes.Equal(base, target) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(entries) == MaxConfigSuffixEntries || len(target) == 0 {
			return nil, ErrConfigHistoryUnavailable
		}
		entry, err := read(ctx, target)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, ErrConfigHistoryUnavailable
		}
		data, err := entry.MarshalVT()
		if err != nil {
			return nil, err
		}
		size += len(data)
		if size > MaxConfigSuffixBytes {
			return nil, ErrConfigHistoryUnavailable
		}
		hash, err := HashSOConfigChange(entry)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(hash, target) {
			return nil, ErrConfigHistoryUnavailable
		}
		entries = append(entries, entry)
		target = entry.GetPreviousHash()
	}

	// Verification and transmission consume transitions in causal order.
	slices.Reverse(entries)
	return entries, nil
}
