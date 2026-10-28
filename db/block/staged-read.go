package block

import (
	"context"
	"sync/atomic"
)

// stagedReadContextKey is the context key of a stagedReadMark.
type stagedReadContextKey struct{}

// stagedReadMark records that a read was served from content no durable store
// holds yet, such as a write buffer's pending block.
type stagedReadMark struct {
	staged atomic.Bool
}

// withStagedReadMark attaches a fresh mark to ctx for one block read.
func withStagedReadMark(ctx context.Context) (context.Context, *stagedReadMark) {
	mark := &stagedReadMark{}
	return context.WithValue(ctx, stagedReadContextKey{}, mark), mark
}

// MarkStagedRead records on ctx that the block being read came from staged
// content: a write buffer, a publication being prepared, or any other bytes
// that are not yet durably stored. A store that serves such content must call
// it. The decoded block cache then keeps the decoded block out of the pool it
// shares across transactions, so a transaction that discards or fails to
// publish the block cannot leave it readable to others.
func MarkStagedRead(ctx context.Context) {
	if ctx == nil {
		return
	}
	if mark, _ := ctx.Value(stagedReadContextKey{}).(*stagedReadMark); mark != nil {
		mark.staged.Store(true)
	}
}

// read reports whether the read was served from staged content.
func (m *stagedReadMark) read() bool {
	return m != nil && m.staged.Load()
}
