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
	// link memoizes the current pool slot, when a parent has a typed edge.
	link *decodedBlockReadLink
}

// NewReadCursor borrows source for a nonempty immutable reference. It returns
// nil for writable transactions, missing references, or disabled cache scopes,
// so a caller can keep its existing mutable traversal in those cases.
func NewReadCursor(ctx context.Context, source *Cursor, ref *BlockRef) *ReadCursor {
	// Require an immutable transaction and a stored reference.
	if source == nil || source.t == nil || !source.t.readOnly.Load() || ref.GetEmpty() {
		return nil
	}

	// Capture the source's cache scope without retaining an operation context.
	ctx = source.decodedBlockCacheContext(ctx)
	cache := decodedBlockCacheFromContext(ctx)
	if !cache.enabled() {
		return nil
	}
	return &ReadCursor{source: source, cache: cache, ref: ref.Clone(), link: &decodedBlockReadLink{}}
}

// FollowRef returns a reader at ref. A typed edge of the loaded immutable
// parent shares its memoized slot; raw or unknown edges need no retained link.
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

	// Unmemoized references own a copy of the digest.
	ref := &BlockRef{Hash: &hash.Hash{HashType: hashType, Hash: bytes.Clone(digest)}}
	return ReadCursor{source: r.source, cache: r.cache, ref: ref}
}

// edge returns the parent's memoized link for the typed edge id, or nil when
// the parent is not loaded or has no such edge.
func (r ReadCursor) edge(id uint32) *decodedBlockReadLink {
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

// Fetch reads the referenced bytes through the source's current storage view.
// Raw values are not retained by the decoded-node graph.
func (r ReadCursor) Fetch(ctx context.Context) ([]byte, bool, error) {
	if r.source == nil {
		return nil, false, nil
	}
	cursor := r.loadCursor()
	return cursor.Fetch(ctx)
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
		cache = r.source.t.decodedBlocks.Load()
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
