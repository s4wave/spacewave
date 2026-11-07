// Package cdn_writer is the writer of self-hosted CDN Spaces. It serves the
// session API routes the publisher speaks, keeps each Space's root pointer
// in a Store, and has writers upload packs straight to the bucket on a
// presigned PUT. A pack joins a Space only after the bucket holds its exact
// bytes. A reclaim's replacement pack supersedes others, which stay in the
// bucket for a grace period, so a reader holding an old pointer keeps
// working.
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
	// ErrPackReplaced refuses a replacement of a pack the Space does not list
	// as current, and a committed entry already marked superseded.
	ErrPackReplaced = errors.New("a CDN Space replacement may only supersede its current packs")
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
	// Replaces are the current packs the commit's one added pack
	// supersedes.
	Replaces []string
	// Retires are superseded packs to drop from the pointer, whose objects
	// the writer deleted.
	Retires []string
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

// ApplyCommit applies c to root in place: it marks the replaced packs
// superseded by c's one pack, drops the retired ones, appends the packs root
// does not list, each with the next sequence, and moves root to c.Checkpoint
// when set. A pack root already lists is a retried commit and is skipped. It
// reports whether root changed. Entries it changes are copies, so root's
// entries may be shared.
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

	// Supersede the replaced packs and drop the retired ones.
	if len(c.Replaces) != 0 || len(c.Retires) != 0 {
		packs, replaced, err := replacePacks(root.GetPacks(), c)
		if err != nil {
			return false, err
		}
		if replaced {
			root.Packs = packs
			changed = true
		}
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

// replacePacks returns packs with c's replaced packs marked superseded by
// c's one added pack and c's retired packs dropped, on a new list, and
// reports whether it differs. A replaced pack already superseded by c's pack
// is a retried commit; a retired pack packs lacks was dropped already.
func replacePacks(packs []*packfile.PackfileEntry, c *Commit) ([]*packfile.PackfileEntry, bool, error) {
	// A replacement adds the one pack that supersedes the others.
	var by string
	if len(c.Replaces) != 0 {
		if len(c.Packs) != 1 {
			return nil, false, errors.New("a replacement commit adds exactly one pack")
		}
		by = c.Packs[0].GetId()
	}
	pending := make(map[string]bool, len(c.Replaces))
	for _, id := range c.Replaces {
		pending[id] = true
	}
	retired := make(map[string]bool, len(c.Retires))
	for _, id := range c.Retires {
		retired[id] = true
	}

	// Copy the list, marking and dropping as c names.
	out := make([]*packfile.PackfileEntry, 0, len(packs))
	changed := false
	for _, pack := range packs {
		id := pack.GetId()
		switch {
		case retired[id]:
			if !pack.IsSuperseded() {
				return nil, false, errors.Errorf("retired pack %s is current", id)
			}
			changed = true
			continue
		case pending[id]:
			// The replacing pack itself stays current, as does one a retry
			// already superseded.
			delete(pending, id)
			if id == by || pack.GetSupersededBy() == by {
				break
			}
			if pack.IsSuperseded() || pack.IsTrash() {
				return nil, false, errors.Wrapf(ErrPackReplaced, "pack %s is superseded", id)
			}
			pack = pack.CloneVT()
			pack.SupersededBy, pack.SupersededAt = by, timestamppb.New(c.Now)
			changed = true
		}
		out = append(out, pack)
	}

	// Every replaced pack must be listed.
	for id := range pending {
		return nil, false, errors.Wrapf(ErrPackReplaced, "pack %s is not listed", id)
	}
	return out, changed, nil
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
