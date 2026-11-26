package sobject

import (
	"bytes"
	"context"
	"sync/atomic"
)

// SignedOperationQueuer is a SharedObject that can report a write's signed
// identity before the operation becomes visible to replay.
type SignedOperationQueuer interface {
	// QueueOperationWithReceipt is QueueOperation with a one-shot receipt.
	// The signer publishes before exposing the operation. A published receipt
	// identifies the operation even if the state write later fails; it does
	// not report durability, placement, or acceptance.
	QueueOperationWithReceipt(ctx context.Context, op []byte, receipt *SignedOperationReceipt) (string, *SOOperationPosition, error)
}

// SignedOperationReceipt holds the immutable position of one signed operation.
// Its zero value has no position. It must not be copied after first use.
type SignedOperationReceipt struct {
	// position is published once and is never exposed to mutation.
	position atomic.Pointer[SOOperationPosition]
}

// Publish records position without calling the consumer or taking its lock.
// It returns false for an empty hash or a receipt that was already published.
func (r *SignedOperationReceipt) Publish(position *SOOperationPosition) bool {
	return len(position.GetOpHash()) != 0 && r.position.CompareAndSwap(nil, position.CloneVT())
}

// MatchesHash reports whether hash names the signed operation in the receipt.
func (r *SignedOperationReceipt) MatchesHash(hash []byte) bool {
	return len(hash) != 0 && bytes.Equal(r.position.Load().GetOpHash(), hash)
}
