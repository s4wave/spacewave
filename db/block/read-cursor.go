package block

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/net/hash"
)

// ReadCursor follows immutable stored references without building a mutable
// parent graph. It borrows its source transaction's store, transform and cache;
// the caller must keep that transaction and its root retention alive. Values
// returned by UnmarshalReadBlock are shared and must not be mutated.
//
// Child links retain evictable slots, not decoded blocks. Every read checks
// cache epochs and durable-removal generations before using a retained value.
// Eviction clears the slot's payload, including its outgoing links.
type ReadCursor struct {
	// source supplies the transaction's current read authority.
	source *Cursor
	// cache is the owning decoded-block scope captured at construction.
	cache *DecodedBlockCache
	// ref identifies this immutable stored block.
	ref *BlockRef
	// link memoizes this reference and its pool slot, when the parent is loaded.
	link *decodedBlockReadLink
}

// NewReadCursor borrows source for a nonempty immutable reference. It returns
// nil for a missing reference, and for a writable transaction whose source
// cursor is dirty: that subtree's pages live on the mutable cursor tree, and a
// read of it sees the uncommitted pages and retains them on the tree. A clean
// source has a clean subtree, so its reference names stored blocks no later
// write changes, and the reader is a snapshot of the subtree as of its open,
// on a writable transaction as on a read-only one. The reader takes the
// context's scope, else the transaction's enabled borrowed scope, else its
// private scope in the process-wide pool, so a scopeless reader still decodes
// a page once rather than on every follow.
func NewReadCursor(ctx context.Context, source *Cursor, ref *BlockRef) *ReadCursor {
	// Require a stored reference beneath an immutable or clean source.
	if source == nil || source.t == nil || ref.GetEmpty() {
		return nil
	}
	if !source.t.readOnly.Load() && source.IsDirty() {
		return nil
	}

	// Capture the reader's cache scope without retaining an operation context.
	cache := decodedBlockCacheFromContext(ctx)
	if cache == nil {
		cache = source.t.readerCache()
	}
	out := &ReadCursor{source: source, cache: cache, ref: ref.Clone()}
	if cache.enabled() {
		out.link = &decodedBlockReadLink{}
	}
	return out
}

// FollowRef returns a reader at ref. An edge of the loaded immutable parent
// shares its memoized reference, and a typed edge its slot; an unknown edge
// clones ref.
func (r ReadCursor) FollowRef(id uint32, ref *BlockRef) ReadCursor {
	// Borrow only a link whose stored reference matches this exact parent edge.
	link := r.edge(id)
	if link != nil && link.ref.EqualsRef(ref) {
		return ReadCursor{source: r.source, cache: r.cache, ref: link.ref, link: link}
	}

	// Unmemoized references also own their identity independently of the caller.
	return ReadCursor{source: r.source, cache: r.cache, ref: ref.Clone()}
}

// FollowDigest returns a reader at the block with the given hash type and
// digest, as FollowRef does for a reference. It lets a parent that stores its
// references flat follow a memoized edge without building a BlockRef: only an
// unmemoized edge copies the digest, so the caller may reuse its bytes.
func (r ReadCursor) FollowDigest(id uint32, hashType hash.HashType, digest []byte) ReadCursor {
	// Borrow only a link whose stored reference matches this exact parent edge.
	link := r.edge(id)
	if link != nil {
		if h := link.ref.GetHash(); h.GetHashType() == hashType && bytes.Equal(h.GetHash(), digest) {
			return ReadCursor{source: r.source, cache: r.cache, ref: link.ref, link: link}
		}
	}

	// Unmemoized references own a copy of the digest, in one allocation.
	return ReadCursor{source: r.source, cache: r.cache, ref: newDigestRef(hashType, digest)}
}

// digestRef holds a BlockRef with its Hash and digest bytes, so a reference
// built from a flat digest costs one allocation rather than three.
type digestRef struct {
	ref    BlockRef
	hash   hash.Hash
	digest [digestRefLen]byte
}

// digestRefLen is the longest digest of a supported hash type.
const digestRefLen = 32

// newDigestRef returns a reference to the block with hashType and a copy of
// digest. A digest longer than every supported hash type is copied apart.
func newDigestRef(hashType hash.HashType, digest []byte) *BlockRef {
	// Hold the digest inline when it fits, so the reference is one allocation.
	out := &digestRef{}
	out.hash.HashType = hashType
	if len(digest) <= digestRefLen {
		out.hash.Hash = out.digest[:len(digest):len(digest)]
		copy(out.hash.Hash, digest)
	} else {
		out.hash.Hash = bytes.Clone(digest)
	}
	out.ref.Hash = &out.hash
	return &out.ref
}

// edge returns the parent's memoized link for the edge id, or nil when
// the parent is not loaded or has no such edge.
func (r ReadCursor) edge(id uint32) *decodedBlockReadLink {
	// An uncached reader or an evicted parent has no edges to share.
	if r.link == nil {
		return nil
	}
	slot := r.link.slot.Load()
	if slot == nil {
		return nil
	}
	data := slot.data.Load()
	if data == nil {
		return nil
	}
	return data.links[id]
}

// FetchLeaf reads the referenced leaf through the source's current storage
// view, as Cursor.FetchLeaf does. Raw values are not retained by the
// decoded-node graph.
func (r ReadCursor) FetchLeaf(ctx context.Context) ([]byte, bool, error) {
	return r.source.fetchLeafAt(ctx, r.ref)
}

// Cursor materializes a private position in the borrowed transaction, without
// adding parent links to the immutable traversal. Its transaction stays readonly.
func (r ReadCursor) Cursor() *Cursor {
	if r.source == nil {
		return nil
	}
	return &Cursor{t: r.source.t, store: r.source.store, pos: &handle{ref: r.ref.Clone()}}
}

// loadCursor borrows the immutable reference only for an internal read call.
func (r ReadCursor) loadCursor() *Cursor {
	return &Cursor{t: r.source.t, store: r.source.store, pos: &handle{ref: r.ref}}
}

// current returns a retained payload valid for this read's scope and storage
// generation. Store refresh happens before this method, outside cache locks.
func (r ReadCursor) current(ctx context.Context) *decodedBlockReadData {
	// Require the same active scope that admitted this retained payload.
	if r.link == nil || !r.cache.enabled() {
		return nil
	}
	cache := decodedBlockCacheFromContext(ctx)
	if cache == nil {
		cache = r.source.t.readerCache()
	}
	if cache != r.cache {
		return nil
	}

	// Resolve only an admitted payload under its original scope and removal stamp.
	slot := r.link.slot.Load()
	if slot == nil {
		return nil
	}
	data := slot.data.Load()
	if data == nil || data.cache != r.cache || data.epochs != r.cache.epochs(data.refKey) || removedSince(data.refKey, data.durableGen) {
		return nil
	}
	slot.recent.Store(true)
	return data
}

// UnmarshalReadBlock returns the immutable decoded block at reader as a *T.
// Exact decoded types, transforms and cache scopes remain separate. Node misses
// use Cursor.Unmarshal, preserving validation, staging and durable admission.
func UnmarshalReadBlock[T any, PT Ptr[T]](ctx context.Context, reader ReadCursor) (PT, error) {
	// Stop absent or cancelled reads before using the borrowed authority.
	if reader.source == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// A writer's own reads skip its private scope; a reader on it reads through
	// the transaction's current reader scope, so a later scope change still
	// bypasses what the old scope retained.
	if !reader.source.t.readOnly.Load() && decodedBlockCacheFromContext(ctx) == nil {
		ctx = WithDecodedBlockCache(ctx, reader.source.t.readerCache())
	}
	store := reader.source.readStore(ctx)
	if freshener, ok := store.(DecodedBlockCacheFreshener); ok {
		if err := freshener.EnsureDecodedBlockCacheFresh(withoutReadOperationStore(ctx)); err != nil {
			return nil, err
		}
	}

	// Reuse a node only when its concrete decoded type matches this request.
	if data := reader.current(ctx); data != nil {
		if out, ok := data.block.(PT); ok {
			observeDecodedBlockHit(ctx, reader.ref)
			RecordDecodedBlockCacheHit(ctx, false)
			return out, nil
		}
	}

	// Load through the normal verified decoder and attach only a durable entry.
	cursor := reader.loadCursor()
	out, err := UnmarshalBlock[T, PT](ctx, cursor)
	if err != nil || out == nil {
		return out, err
	}
	if reader.link != nil {
		ctx = cursor.decodedBlockCacheContext(ctx)
		cache := decodedBlockCacheFromContext(ctx)
		key, ok := decodedBlockCacheKeyFor(reader.ref, out, cursor.transformer())
		if ok && cache == reader.cache {
			if slot := cache.readSlot(key); slot != nil {
				reader.link.slot.Store(slot)
			}
		}
	}
	return out, nil
}
