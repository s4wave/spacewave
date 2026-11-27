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

// SettledOperationQueuer is a SignedOperationQueuer whose
// QueueOperationWithReceipt writes nothing after it returns, even when ctx
// ended first. Unless it returned ErrStateWriteUnknown, the mount's state then
// shows whether storage holds the operation, so a caller may judge the write
// by its absence from that state. A provider whose call can return while its
// write may still commit, such as one waiting on a remote request, does not
// implement it.
type SettledOperationQueuer interface {
	SignedOperationQueuer
	// SettlesQueuedOperations marks the contract and does nothing.
	SettlesQueuedOperations()
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

// OpHash returns the hash of the signed operation, or nil before Publish. The
// caller must not modify it.
func (r *SignedOperationReceipt) OpHash() []byte {
	return r.position.Load().GetOpHash()
}

// Position returns the published position, or nil before Publish. The caller
// must not modify it.
func (r *SignedOperationReceipt) Position() *SOOperationPosition {
	return r.position.Load()
}
