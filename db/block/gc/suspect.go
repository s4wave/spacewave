package block_gc

import (
	"github.com/s4wave/spacewave/db/block"
)

// SuspectError refuses a hold onto a suspect block: a stored block whose
// child list no root vouched for when the Volume's lists were verified, so
// the list may lack a child. Putting the block again, with its refs, writes
// its list from its content, after which the hold may succeed. A Volume
// hides a suspect block from its existence checks, so a writer that checks
// before it puts puts it again.
type SuspectError struct {
	// Ref is the suspect block.
	Ref *block.BlockRef
}

// Error describes the refusal.
func (e *SuspectError) Error() string {
	return "block " + e.Ref.MarshalString() + " has a child list no root vouched for: put it again before holding it"
}

// Unwrap returns block.ErrRefsUnknown, which a hold onto a block whose
// children the Volume does not know returns.
func (e *SuspectError) Unwrap() error {
	return block.ErrRefsUnknown
}

// _ is a type assertion
var _ error = (*SuspectError)(nil)
