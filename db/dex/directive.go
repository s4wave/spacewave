// Package dex fetches blocks from other peers.
//
// A local read that misses, with network lookup enabled in its bucket's
// lookup configuration, emits a LookupBlockFromNetwork directive for the
// block's ref, and any DEX controller on the bus may resolve it: package
// psecho through want lists gossiped over a pub-sub channel, and package
// solicit through a stream per peer that Bifrost protocol solicitation pairs.
// Each verifies the bytes it receives against the requested ref before it
// emits them, so a peer cannot substitute a block. A DEX serves the block
// and stores nothing itself: the bucket lookup decides whether to write it
// back, and it does so only when the block's outgoing refs are known.
package dex

import (
	"context"
	"strconv"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// LookupBlockFromNetwork is a directive to find a block from remote DHT.
type LookupBlockFromNetwork interface {
	// Directive indicates LookupBlockFromNetwork is a directive.
	directive.Directive

	// LookupBlockFromNetworkBucketId returns the associated bucket id.
	// Can be empty.
	LookupBlockFromNetworkBucketId() string
	// LookupBlockFromNetworkRef returns the desired block ref.
	LookupBlockFromNetworkRef() *block.BlockRef
}

// LookupBlockFromNetworkValue is the result type for LookupBlockFromNetwork.
type LookupBlockFromNetworkValue interface {
	// GetError returns any error.
	GetError() error
	// GetData returns the returned data.
	// Returns nil if not found.
	GetData() []byte
	// GetRefs returns the block's outgoing refs when GetRefsKnown is set.
	GetRefs() []*block.BlockRef
	// GetRefsKnown returns false when the source held the bytes without
	// their refs.
	GetRefsKnown() bool
	// GetSubtree returns the blocks under the ref a LookupSubtreeFromNetwork
	// was answered with, each verified against a ref a verified block named,
	// with its refs known. It is empty for any other lookup.
	GetSubtree() []*block.PutBatchEntry
}

// lookupBlockFromNetworkValue implements LookupBlockFromNetworkValue
type lookupBlockFromNetworkValue struct {
	err       error
	data      []byte
	refs      []*block.BlockRef
	refsKnown bool
	subtree   []*block.PutBatchEntry
}

// NewLookupBlockFromNetworkValue builds a LookupBlockFromNetworkValue whose
// refs are unknown.
func NewLookupBlockFromNetworkValue(data []byte, err error) LookupBlockFromNetworkValue {
	return &lookupBlockFromNetworkValue{data: data, err: err}
}

// NewLookupBlockFromNetworkValueWithRefs builds a LookupBlockFromNetworkValue
// for a found block and its outgoing refs.
func NewLookupBlockFromNetworkValueWithRefs(data []byte, refs []*block.BlockRef) LookupBlockFromNetworkValue {
	return &lookupBlockFromNetworkValue{data: data, refs: refs, refsKnown: true}
}

// NewLookupSubtreeFromNetworkValue builds a LookupBlockFromNetworkValue for a
// found block, its outgoing refs, and the blocks under it that the source
// sent.
func NewLookupSubtreeFromNetworkValue(data []byte, refs []*block.BlockRef, subtree []*block.PutBatchEntry) LookupBlockFromNetworkValue {
	return &lookupBlockFromNetworkValue{data: data, refs: refs, refsKnown: true, subtree: subtree}
}

// GetError returns any error.
func (v *lookupBlockFromNetworkValue) GetError() error {
	return v.err
}

// GetData returns the returned data.
// Returns nil if not found.
func (v *lookupBlockFromNetworkValue) GetData() []byte {
	return v.data
}

// GetRefs returns the block's outgoing refs when GetRefsKnown is set.
func (v *lookupBlockFromNetworkValue) GetRefs() []*block.BlockRef {
	return v.refs
}

// GetRefsKnown returns false when the source held the bytes without their
// refs.
func (v *lookupBlockFromNetworkValue) GetRefsKnown() bool {
	return v.refsKnown
}

// GetSubtree returns the blocks under the ref the source sent.
func (v *lookupBlockFromNetworkValue) GetSubtree() []*block.PutBatchEntry {
	return v.subtree
}

// NewLookupBlockFromNetwork constructs an LookupBlockFromNetwork.
func NewLookupBlockFromNetwork(bucketID string, ref *block.BlockRef) LookupBlockFromNetwork {
	return &LookupBlockFromNetworkRequest{
		BucketId: bucketID,
		Ref:      ref,
	}
}

// Validate checks that the directive names a ref.
func (d *LookupBlockFromNetworkRequest) Validate() error {
	if d.LookupBlockFromNetworkRef().GetEmpty() {
		return errors.New("ref cannot be empty")
	}

	return nil
}

// GetValueOptions returns the directive's value options, the defaults.
func (d *LookupBlockFromNetworkRequest) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LookupBlockFromNetworkBucketId returns the desired bucket id.
// Can be empty.
func (d *LookupBlockFromNetworkRequest) LookupBlockFromNetworkBucketId() string {
	return d.GetBucketId()
}

// LookupBlockFromNetworkRef returns the requested block ref.
func (d *LookupBlockFromNetworkRequest) LookupBlockFromNetworkRef() *block.BlockRef {
	return d.GetRef()
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *LookupBlockFromNetworkRequest) IsEquivalent(other directive.Directive) bool {
	// Require another network block lookup before comparing its target.
	od, ok := other.(LookupBlockFromNetwork)
	if !ok {
		return false
	}

	// Compare the bucket and block reference requested by both lookups.
	if d.LookupBlockFromNetworkBucketId() != od.LookupBlockFromNetworkBucketId() {
		return false
	}
	if !d.LookupBlockFromNetworkRef().EqualsRef(od.LookupBlockFromNetworkRef()) {
		return false
	}
	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *LookupBlockFromNetworkRequest) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *LookupBlockFromNetworkRequest) GetName() string {
	return "LookupBlockFromNetwork"
}

// GetDebugVals returns the directive arguments stringified.
func (d *LookupBlockFromNetworkRequest) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.LookupBlockFromNetworkBucketId() != "" {
		vals["bucket-id"] = []string{d.LookupBlockFromNetworkBucketId()}
	}
	if vre := d.LookupBlockFromNetworkRef(); vre != nil && !vre.GetEmpty() {
		vals["ref"] = []string{vre.MarshalString()}
	}
	return vals
}

// LookupSubtreeFromNetwork is a LookupBlockFromNetwork that also asks for the
// blocks under its ref that the asker lacks, depth first, up to a budget of
// block data, so a walk reads its next levels locally. Its value's GetSubtree
// holds them. A resolver that cannot send them answers the ref alone.
type LookupSubtreeFromNetwork interface {
	LookupBlockFromNetwork

	// LookupSubtreeBytes returns the budget of block data under the ref.
	LookupSubtreeBytes() uint32
	// LookupSubtreeHave reports which of refs the asker already holds, so
	// no source sends them or walks the blocks under them.
	LookupSubtreeHave(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}

// lookupSubtreeFromNetwork implements LookupSubtreeFromNetwork.
type lookupSubtreeFromNetwork struct {
	*LookupBlockFromNetworkRequest
	bytes uint32
	have  func(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}

// NewLookupSubtreeFromNetwork constructs a LookupSubtreeFromNetwork for ref in
// bucketID, asking for up to bytes of block data under it, with have
// reporting the blocks the asker holds.
func NewLookupSubtreeFromNetwork(
	bucketID string,
	ref *block.BlockRef,
	bytes uint32,
	have func(ctx context.Context, refs []*block.BlockRef) ([]bool, error),
) LookupSubtreeFromNetwork {
	return &lookupSubtreeFromNetwork{
		LookupBlockFromNetworkRequest: &LookupBlockFromNetworkRequest{BucketId: bucketID, Ref: ref},
		bytes:                         bytes,
		have:                          have,
	}
}

// LookupSubtreeBytes returns the budget of block data under the ref.
func (d *lookupSubtreeFromNetwork) LookupSubtreeBytes() uint32 {
	return d.bytes
}

// LookupSubtreeHave reports which of refs the asker already holds.
func (d *lookupSubtreeFromNetwork) LookupSubtreeHave(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return d.have(ctx, refs)
}

// IsEquivalent checks if the other directive is a subtree lookup of the same
// bucket and ref, whose answer carries the subtree this one asks for.
func (d *lookupSubtreeFromNetwork) IsEquivalent(other directive.Directive) bool {
	if _, ok := other.(LookupSubtreeFromNetwork); !ok {
		return false
	}
	return d.LookupBlockFromNetworkRequest.IsEquivalent(other)
}

// GetName returns the directive's type name.
func (d *lookupSubtreeFromNetwork) GetName() string {
	return "LookupSubtreeFromNetwork"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupSubtreeFromNetwork) GetDebugVals() directive.DebugValues {
	vals := d.LookupBlockFromNetworkRequest.GetDebugVals()
	vals["subtree-bytes"] = []string{strconv.FormatUint(uint64(d.bytes), 10)}
	return vals
}

// _ are type assertions.
var (
	_ LookupBlockFromNetwork   = (*LookupBlockFromNetworkRequest)(nil)
	_ LookupSubtreeFromNetwork = (*lookupSubtreeFromNetwork)(nil)
)
