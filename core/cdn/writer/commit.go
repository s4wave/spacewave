// Package cdn_writer is the writer of self-hosted CDN Spaces. It serves the
// session API routes the publisher speaks, keeps each Space's root pointer
// in a Store, and has writers upload packs straight to the bucket on a
// presigned PUT. A pack joins a Space only after the bucket holds its exact
// bytes, and packs are only ever added, so a reader holding an old pointer
// keeps working.
package cdn_writer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/crypto"
)

var (
	// ErrSpaceUnknown refuses a request for a Space the writer does not own.
	ErrSpaceUnknown = errors.New("the writer does not own this CDN Space")
	// ErrCheckpointStale refuses a checkpoint that does not follow the
	// recorded one, such as one built from an older state.
	ErrCheckpointStale = errors.New("checkpoint does not follow the Space's current checkpoint")
	// ErrCheckpointSigner refuses a checkpoint the Space's owners did not
	// sign, which readers would refuse.
	ErrCheckpointSigner = errors.New("checkpoint is not signed by the Space's owner")
	// ErrPackReplaced refuses a pack that supersedes or retires another: a
	// commit only adds packs.
	ErrPackReplaced = errors.New("a CDN Space commit may only add packs")
	// ErrPackNotAdmitted refuses a commit of a pack no live upload admitted.
	ErrPackNotAdmitted = errors.New("no live upload admitted this pack")
)

// Commit is one change to a Space's pointer: packs the bucket already holds,
// and the checkpoint the Space moves to.
type Commit struct {
	// SpaceID is the Space committed to.
	SpaceID string
	// Checkpoint follows the recorded checkpoint, or is nil to keep it.
	Checkpoint *sobject.SOCheckpoint
	// Packs are the packs the commit adds.
	Packs []*packfile.PackfileEntry
	// Now stamps the new pointer and the added packs.
	Now time.Time
	// Signer, when set, is the peer whose signature the checkpoint must
	// carry in place of the recorded checkpoint's signers, so a writer
	// admitted at runtime moves the Space under its own key.
	Signer string
}

// Store holds the root pointer of each Space the writer owns.
type Store interface {
	// Root returns the recorded pointer of spaceID, or nil when there is
	// none.
	Root(ctx context.Context, spaceID string) (*cdn.CdnRootPointer, error)
	// Import records root unless its Space is recorded, and returns the
	// recorded pointer.
	Import(ctx context.Context, root *cdn.CdnRootPointer) (*cdn.CdnRootPointer, error)
	// Commit applies c to the recorded pointer with ApplyCommit in one
	// atomic step and returns the new pointer once it is durable. Commits to
	// one Space must not interleave between reading and writing the pointer.
	Commit(ctx context.Context, c *Commit) (*cdn.CdnRootPointer, error)
}

// ApplyCommit applies c to root in place: it appends the packs root does not
// list, each with the next sequence, and moves root to c.Checkpoint when set.
// A pack root already lists is a retried commit and is skipped. It reports
// whether root changed.
func ApplyCommit(root *cdn.CdnRootPointer, c *Commit) (bool, error) {
	// Move to the checkpoint when it follows the recorded one.
	changed := false
	if next := c.Checkpoint; next != nil && !next.EqualVT(root.GetCheckpoint()) {
		if err := CheckCheckpointFollows(root, next, c.Signer); err != nil {
			return false, err
		}
		root.Checkpoint = next.CloneVT()
		changed = true
	}

	// Append the packs not yet listed.
	listed := make(map[string]struct{}, len(root.GetPacks()))
	var sequence uint64
	for _, pack := range root.GetPacks() {
		listed[pack.GetId()] = struct{}{}
		sequence = max(sequence, pack.GetSequence())
	}
	for _, pack := range c.Packs {
		if pack.GetId() == "" {
			return false, errors.New("a committed pack needs an ID")
		}
		if pack.GetSupersededBy() != "" || pack.GetSupersededAt() != nil || pack.GetTrashedAt() != nil {
			return false, ErrPackReplaced
		}
		if _, ok := listed[pack.GetId()]; ok {
			continue
		}
		listed[pack.GetId()] = struct{}{}
		sequence++
		added := pack.CloneVT()
		added.Sequence = sequence
		if added.CreatedAt == nil {
			added.CreatedAt = timestamppb.New(c.Now)
		}
		root.Packs = append(root.Packs, added)
		changed = true
	}

	// Stamp a changed pointer.
	if changed {
		root.CreatedAtMs = c.Now.UnixMilli()
	}
	return changed, nil
}

// CheckCheckpointFollows reports whether next is the checkpoint after root's,
// for the same Space and config, signed by signer when set and otherwise by
// every signer of root's.
func CheckCheckpointFollows(root *cdn.CdnRootPointer, next *sobject.SOCheckpoint, signer string) error {
	// A Space of packs alone has no recorded checkpoint to follow.
	if root.GetCheckpoint() == nil {
		return errors.Wrap(ErrCheckpointStale, "the Space holds no checkpoint")
	}

	// Authenticate both checkpoints, then require the given signer or the
	// recorded ones.
	inner, signers, err := next.Verify(root.GetSpaceId())
	if err != nil {
		return errors.Wrap(err, "checkpoint")
	}
	prev, owners, err := root.GetCheckpoint().Verify(root.GetSpaceId())
	if err != nil {
		return errors.Wrap(err, "recorded checkpoint")
	}
	if signer != "" {
		owners = []string{signer}
	}
	for _, owner := range owners {
		if !slices.Contains(signers, owner) {
			return errors.Wrapf(ErrCheckpointSigner, "checkpoint lacks %s", owner)
		}
	}

	// Require the next height, linked to the recorded checkpoint, under the
	// recorded config.
	switch {
	case inner.GetSharedObjectId() != root.GetSpaceId():
		return errors.Wrap(ErrCheckpointStale, "checkpoint names another Space")
	case inner.GetHeight() != prev.GetHeight()+1 || !bytes.Equal(inner.GetPrevCheckpointHash(), root.GetCheckpoint().Hash()):
		return ErrCheckpointStale
	case !bytes.Equal(inner.GetConfigHash(), root.GetConfigChainHash()):
		return errors.Wrap(ErrCheckpointStale, "checkpoint names another config")
	}
	return nil
}

// NewGenesis returns the pointer of a new Space with no packs, at a genesis
// checkpoint signed by owner. Later checkpoints are signed by owner too, as
// the publisher signs them with the Space's owner key.
func NewGenesis(spaceID string, owner crypto.PrivKey) (*cdn.CdnRootPointer, error) {
	// Name the Space's config by its ID, since a CDN Space has no config
	// chain of its own.
	configHash := sha256.Sum256([]byte("spacewave/cdn/space/" + spaceID))
	genesis, err := sobject.BuildGenesisSOCheckpoint(owner, spaceID, configHash[:], nil)
	if err != nil {
		return nil, err
	}
	return &cdn.CdnRootPointer{
		SpaceId:          spaceID,
		Checkpoint:       genesis,
		ConfigChainHash:  configHash[:],
		ConfigChainSeqno: 1,
	}, nil
}
