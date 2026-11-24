package block

import (
	"iter"
	"strconv"
	"strings"
)

// DecodedBlockResident is one live entry of a decoded-block cache scope.
type DecodedBlockResident struct {
	// RefKey is the block ref's MarshalKey encoding. BlockRef.UnmarshalVT
	// decodes it.
	RefKey string
	// TypeKey is the decoded block's DecodedBlockCacheTypeKey.
	TypeKey string
	// Cost is the budget the entry is charged, its reader metadata included.
	Cost int64
}

// decodedBlockResidentCopy is an entry copied out of a shard for the walk.
type decodedBlockResidentCopy struct {
	// key is the entry's pool key.
	key string
	// epochs are the invalidation epochs the entry was stored under.
	epochs decodedBlockCacheEpochs
	// durableGen is the removal generation of the read that admitted it.
	durableGen uint64
	// cost is the budget the entry is charged.
	cost int64
}

// Resident returns the scope's live entries: each one a Lookup of the same
// key would hit when the walk reaches it. Every entry in the pool came from a
// durable read, so the walk names only blocks a durable store held. A ref
// stored under several types or transforms appears once for each.
//
// The walk visits one shard at a time. It copies the shard's entries of this
// scope out under the shard's read lock and yields them with the lock
// released, so a slow consumer never holds a shard, and it keeps one shard's
// copies at a time. Entries stored or evicted during the walk may or may not
// appear. A closed or disabled scope yields nothing.
func (c *DecodedBlockCache) Resident() iter.Seq[DecodedBlockResident] {
	return func(yield func(DecodedBlockResident) bool) {
		// Yield nothing for a scope that serves no lookups.
		if !c.enabled() {
			return
		}

		// Walk the shards, copying out this scope's entries and yielding the
		// live ones.
		var buf []decodedBlockResidentCopy
		for i := range c.pool.shards {
			buf = c.pool.shards[i].copyScope(c.scope, buf[:0])
			for _, e := range buf {
				resident, ok := c.resident(e)
				if ok && !yield(resident) {
					return
				}
			}
		}
	}
}

// resident parses a copied entry and reports whether a Lookup would hit it:
// its epochs are current and no durable store removed its ref since.
func (c *DecodedBlockCache) resident(e decodedBlockResidentCopy) (DecodedBlockResident, bool) {
	// Split the pool key into the ref and the block type.
	ref, typeKey, ok := parseDecodedBlockCacheKey(strings.TrimPrefix(e.key, c.scope))
	if !ok {
		return DecodedBlockResident{}, false
	}

	// Skip an entry an invalidation or a removal outdated.
	if e.epochs != c.epochs(ref) || removedSince(ref, e.durableGen) {
		return DecodedBlockResident{}, false
	}
	return DecodedBlockResident{RefKey: ref, TypeKey: typeKey, Cost: e.cost}, true
}

// copyScope appends the shard's entries whose pool key starts with scope to
// buf, under the shard's read lock, and returns buf.
func (s *decodedBlockShard) copyScope(scope string, buf []decodedBlockResidentCopy) []decodedBlockResidentCopy {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	for key, e := range s.entries {
		if strings.HasPrefix(key, scope) {
			buf = append(buf, decodedBlockResidentCopy{
				key:        key,
				epochs:     e.value.epochs,
				durableGen: e.value.durableGen,
				cost:       e.cost,
			})
		}
	}
	return buf
}

// parseDecodedBlockCacheKey returns the ref and block type parts of a pool key
// with its scope removed, as cacheKey encodes it. The parts are substrings of
// key.
func parseDecodedBlockCacheKey(key string) (ref, blockType string, ok bool) {
	ref, rest, ok := cutDecodedBlockCacheKeyPart(key)
	if !ok {
		return "", "", false
	}
	blockType, _, ok = cutDecodedBlockCacheKeyPart(rest)
	return ref, blockType, ok
}

// cutDecodedBlockCacheKeyPart cuts one length-prefixed part from the front of
// key and returns it with the rest.
func cutDecodedBlockCacheKeyPart(key string) (part, rest string, ok bool) {
	// Read the decimal length before the colon.
	digits, rest, ok := strings.Cut(key, ":")
	if !ok {
		return "", "", false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || n > len(rest) {
		return "", "", false
	}

	// Split the part from the rest.
	return rest[:n], rest[n:], true
}
