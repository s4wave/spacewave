package block_gc

// ListRebuildOptions selects how a list rebuild treats marked lists.
type ListRebuildOptions struct {
	// Reverify treats a marked list like an unverified one: Verify merges the
	// decoded refs into it, refusing refs that lack a child it names, and a
	// marked list that gains a child is rewritten marked and notes the
	// rewrite, so the counts owe a recount. Without it a marked list is
	// skipped, since Land and the rebuild mark a list whatever refs their
	// caller hands them, copied or decoded.
	//
	// Invariant: after a reverify walk that finishes with an empty front, every
	// list a root reaches through the walker and that it decoded was compared
	// with its block's content in this run. Outside the invariant, and so
	// reported rather than held by the front: a marked block the walker
	// could not decode (MarkedUndecodable), a marked list the merge refused
	// (MarkedRefused), a block no read resolves, and the blocks below a ref
	// whose type the walker cannot name.
	Reverify bool
	// DryRun writes nothing: Verify only compares marked lists with their
	// content and counts what a reverify would find. It requires Reverify,
	// and runs on a Volume in any state.
	DryRun bool
}
