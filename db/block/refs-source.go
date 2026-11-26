package block

import "sync/atomic"

// RefsSource says what a read knows of a block's outgoing refs, and so
// whether they prove the block's child list.
//
// A Volume that frees writes a child list with its mark only from refs the
// writer asserts it read from the block's content, since the mark is the
// proof a free trusts. The assertion never leaves the process: no read
// format, wire, pack or SQL row encodes it, so refs read from one are only
// listed, whatever their store's refs_known says. A copier sets a put's
// RefsKnown from Asserted. A store that never marks, such as a pack, a cache
// or a peer's answer, reads every recorded list back as listed, so a copy
// into one may declare listed refs known with Recorded and launders nothing.
//
// The zero value is RefsUnknown, so a read that forgets to say asserts
// nothing.
type RefsSource uint8

const (
	// RefsUnknown is a read of the bytes without their refs: a leaf cannot
	// be told from a block whose edges were lost.
	RefsUnknown RefsSource = iota
	// RefsListed is a read of refs a store recorded that nothing in this
	// process read from the block's content: a peer's answer, a pack, a SQL
	// row, or a Volume's unverified list. They may lack a child.
	RefsListed
	// RefsAsserted is a read of refs decoded from the block's bytes in this
	// process, held by this process's write buffer for a put that asserted
	// them, or read from a Volume's marked list.
	RefsAsserted
)

// ListedIf returns RefsListed when recorded is set and RefsUnknown
// otherwise, for a read format whose flag says only that its store recorded
// the refs.
func ListedIf(recorded bool) RefsSource {
	if recorded {
		return RefsListed
	}
	return RefsUnknown
}

// AssertedIf returns RefsAsserted when asserted is set and RefsUnknown
// otherwise, for this process's record of a put's RefsKnown.
func AssertedIf(asserted bool) RefsSource {
	if asserted {
		return RefsAsserted
	}
	return RefsUnknown
}

// Recorded reports whether the refs are listed or asserted, so empty refs
// name a leaf as far as their source knows.
func (s RefsSource) Recorded() bool {
	return s != RefsUnknown
}

// Asserted reports whether the refs may set the RefsKnown of a put into a
// store that marks lists, such as a Volume that frees. Listed refs pass too
// while SetTrustListedRefs is set.
func (s RefsSource) Asserted() bool {
	return s == RefsAsserted || (s == RefsListed && trustListedRefs.Load())
}

// trustListedRefs is the process-wide switch SetTrustListedRefs sets.
var trustListedRefs atomic.Bool

// SetTrustListedRefs sets whether this process takes listed refs as
// asserted, as every copy did before a list's provenance was tracked: a
// copy then marks the target's list from refs a source recorded, which may
// lack a child. It is a way back for a copy that fails on listed refs, safe
// only while no Volume of the process frees, since a laundered list lets a
// free take a child its block still names. It returns the previous value.
func SetTrustListedRefs(trust bool) bool {
	return trustListedRefs.Swap(trust)
}

// String names the source for logs.
func (s RefsSource) String() string {
	switch s {
	case RefsListed:
		return "listed"
	case RefsAsserted:
		return "asserted"
	default:
		return "unknown"
	}
}
