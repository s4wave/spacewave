package sobject

import (
	"bytes"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// BuildSOConfigMerge builds and signs a MERGE record on head that absorbs
// branch, the records after fork on a concurrent branch, oldest first. The
// signer must be an OWNER of head.
func BuildSOConfigMerge(
	sharedObjectID string,
	head *SharedObjectConfig,
	fork *SOConfigChange,
	branch []*SOConfigChange,
	signerPrivKey crypto.PrivKey,
) (*SOConfigChange, error) {
	// Compute the config the branch yields on head.
	merge := &SOConfigMerge{Fork: fork.CloneVT(), Branch: make([]*SOConfigChange, len(branch))}
	for i, change := range branch {
		merge.Branch[i] = change.CloneVT()
	}
	next, err := mergeConfigBranch(sharedObjectID, head, merge)
	if err != nil {
		return nil, err
	}

	// Link the record to head and sign it.
	entry := newSOConfigChange(sharedObjectID, head, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_MERGE)
	entry.Merge = merge
	if err := signSOConfigChange(entry, signerPrivKey); err != nil {
		return nil, err
	}
	return entry, nil
}

// verifyConfigMerge checks that a MERGE record's config is the one its branch
// yields on current, and that only MERGE records carry a branch.
func verifyConfigMerge(sharedObjectID string, current *SharedObjectConfig, entry *SOConfigChange) error {
	// Only a MERGE record carries a branch.
	if entry.GetChangeType() != SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_MERGE {
		if entry.GetMerge() != nil {
			return errors.New("only a merge record may carry a merge branch")
		}
		return nil
	}

	// Recompute the merged config and require the record to carry it.
	expected, err := mergeConfigBranch(sharedObjectID, current, entry.GetMerge())
	if err != nil {
		return errors.Wrap(err, "merge record")
	}
	if !EqualSOConfigs(entry.GetConfig(), expected) {
		return errors.New("merge record config differs from its merged branch")
	}
	return nil
}

// mergeConfigBranch returns the config merge yields on head. Each branch record
// must follow the one before it, and the first must follow the fork. Records
// decided under group control carry no owner signature and are dropped.
func mergeConfigBranch(sharedObjectID string, head *SharedObjectConfig, merge *SOConfigMerge) (*SharedObjectConfig, error) {
	// An owner signs a merge alone, so the head must be under owner control.
	if head.IsGroupControl() {
		return nil, errors.New("merge requires owner control")
	}
	fork := merge.GetFork()
	if fork == nil || len(merge.GetBranch()) == 0 {
		return nil, errors.New("merge requires a fork and a branch")
	}
	parent, err := appliedConfig(fork)
	if err != nil {
		return nil, err
	}

	// Apply the fields each record changed, while its signers still own.
	result := head.CloneVT()
	for i, change := range merge.GetBranch() {
		if !isPeerConfigChange(change.GetChangeType()) {
			return nil, errors.Errorf("branch[%d]: unsupported merged config change %s", i, change.GetChangeType())
		}
		next, err := VerifyConfigChange(sharedObjectID, parent, change)
		if err != nil {
			return nil, errors.Wrapf(err, "branch[%d]", i)
		}
		if signedByOwners(change, result) {
			candidate := applyConfigDelta(result, parent, next)
			if candidate.Validate() == nil {
				result = candidate
			}
		}
		parent = next
	}
	return result, nil
}

// signedByOwners reports whether entry carries signatures and every signer is
// an OWNER of cfg. The caller has verified the signatures.
func signedByOwners(entry *SOConfigChange, cfg *SharedObjectConfig) bool {
	sigs := entry.GetSignatures()
	if len(sigs) == 0 {
		return false
	}
	for _, sig := range sigs {
		_, id, err := signerOf(sig)
		if err != nil || !isOwnerPeer(cfg, id.String()) {
			return false
		}
	}
	return true
}

// applyConfigDelta returns target with each field that next changed from
// parent set to its value in next. Participants and removed authors merge by
// peer ID, and dropped roster peers merge as a set. The chain head fields
// keep target's values. A new SharedObjectConfig field must be merged here.
func applyConfigDelta(target, parent, next *SharedObjectConfig) *SharedObjectConfig {
	// Merge the keyed lists by peer ID.
	out := target.CloneVT()
	out.Participants = mergeByKey(
		out.GetParticipants(), parent.GetParticipants(), next.GetParticipants(),
		(*SOParticipantConfig).GetPeerId, (*SOParticipantConfig).EqualVT, (*SOParticipantConfig).CloneVT,
	)
	out.RemovedAuthors = mergeByKey(
		out.GetRemovedAuthors(), parent.GetRemovedAuthors(), next.GetRemovedAuthors(),
		(*SOOperationPosition).GetPeerId, (*SOOperationPosition).EqualVT, (*SOOperationPosition).CloneVT,
	)
	slices.SortFunc(out.RemovedAuthors, func(a, b *SOOperationPosition) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	out.RosterDroppedPeerIds = mergeByKey(
		out.GetRosterDroppedPeerIds(), parent.GetRosterDroppedPeerIds(), next.GetRosterDroppedPeerIds(),
		func(id string) string { return id }, func(a, b string) bool { return a == b }, func(id string) string { return id },
	)
	slices.Sort(out.RosterDroppedPeerIds)

	// Take each single field the record changed.
	if !parent.GetSequencer().EqualVT(next.GetSequencer()) {
		out.Sequencer = next.GetSequencer().CloneVT()
	}
	if parent.GetControl() != next.GetControl() {
		out.Control = next.GetControl()
	}
	if !parent.GetSealedCheckpoint().EqualVT(next.GetSealedCheckpoint()) {
		out.SealedCheckpoint = next.GetSealedCheckpoint().CloneVT()
	}
	return out
}

// mergeByKey applies the change from parent to next onto target, matching
// items by key: an item next added or changed replaces or joins target's, and
// an item next removed leaves target. Target keeps its order; new items follow.
func mergeByKey[T any](target, parent, next []T, key func(T) string, equal func(T, T) bool, clone func(T) T) []T {
	// Index the parent's items by key.
	before := make(map[string]T, len(parent))
	for _, item := range parent {
		before[key(item)] = item
	}
	after := make(map[string]struct{}, len(next))
	for _, item := range next {
		after[key(item)] = struct{}{}
	}

	// Drop removed items, then add or replace changed ones.
	out := slices.DeleteFunc(slices.Clone(target), func(item T) bool {
		_, held := before[key(item)]
		_, kept := after[key(item)]
		return held && !kept
	})
	for _, item := range next {
		if prev, ok := before[key(item)]; ok && equal(prev, item) {
			continue
		}
		i := slices.IndexFunc(out, func(existing T) bool { return key(existing) == key(item) })
		if i == -1 {
			out = append(out, clone(item))
		} else {
			out[i] = clone(item)
		}
	}
	return out
}

// AbsorbedFork returns the fork of the merge in entry that absorbs the record
// hashed head, searching merges nested in absorbed branches, or nil when entry
// absorbs no such record.
func AbsorbedFork(head []byte, entry *SOConfigChange) *SOConfigChange {
	merge := entry.GetMerge()
	if entry.GetChangeType() != SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_MERGE || len(head) == 0 {
		return nil
	}
	for _, change := range merge.GetBranch() {
		hash, err := HashSOConfigChange(change)
		if err != nil {
			return nil
		}
		if bytes.Equal(hash, head) {
			return merge.GetFork()
		}
		if fork := AbsorbedFork(head, change); fork != nil {
			return fork
		}
	}
	return nil
}

// AbsorbedConfigChanges returns every record merge records in changes absorb,
// including those of nested merges, so a replica retains the configs that
// operations signed on an absorbed branch name.
func AbsorbedConfigChanges(changes []*SOConfigChange) []*SOConfigChange {
	var out []*SOConfigChange
	for _, change := range changes {
		branch := change.GetMerge().GetBranch()
		out = append(out, branch...)
		out = append(out, AbsorbedConfigChanges(branch)...)
	}
	return out
}

// ConfigSuffixBase returns the config a suffix of changes starts from, given
// the held config current. A suffix that does not follow current must contain
// a merge that absorbs current's head and starts at that merge's fork. The
// absorbed records link the fork to current's head by hash, so current's head
// authenticates the fork; verifying the suffix checks those links.
func ConfigSuffixBase(current *SharedObjectConfig, changes []*SOConfigChange) (*SharedObjectConfig, error) {
	// A suffix that follows the held head starts there.
	head := current.GetConfigChainHash()
	if len(changes) == 0 || bytes.Equal(changes[0].GetPreviousHash(), head) {
		return current, nil
	}

	// Otherwise start at the fork of the merge that absorbs the held head.
	for _, change := range changes {
		fork := AbsorbedFork(head, change)
		if fork == nil {
			continue
		}
		forkHash, err := HashSOConfigChange(fork)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(forkHash, changes[0].GetPreviousHash()) {
			return nil, ErrConfigChainHeadMismatch
		}
		return configWithAppliedConfigChainHead(fork.GetConfig(), fork.GetConfigSeqno(), forkHash), nil
	}
	return nil, ErrConfigChainHeadMismatch
}

// MergeWins reports whether the config head (seqno, hash) is chosen over
// (otherSeqno, otherHash) when two branches merge: the higher sequence number
// wins, and a tie goes to the lower hash.
func MergeWins(seqno uint64, hash []byte, otherSeqno uint64, otherHash []byte) bool {
	if seqno != otherSeqno {
		return seqno > otherSeqno
	}
	return bytes.Compare(hash, otherHash) < 0
}
