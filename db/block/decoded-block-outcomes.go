package block

import "sync/atomic"

// DecodedBlockLookupOutcome is the result of one decoded-block lookup.
type DecodedBlockLookupOutcome uint8

const (
	// DecodedBlockLookupFrontHit is a hit in the read operation's front cache.
	DecodedBlockLookupFrontHit DecodedBlockLookupOutcome = iota
	// DecodedBlockLookupHit is a hit on a current entry in the shared pool.
	DecodedBlockLookupHit
	// DecodedBlockLookupAbsent is a miss: the pool holds no entry.
	DecodedBlockLookupAbsent
	// DecodedBlockLookupStaleEpoch is a miss on an entry an invalidation of
	// its scope or ref made stale.
	DecodedBlockLookupStaleEpoch
	// DecodedBlockLookupStaleRemoved is a miss on an entry whose removal
	// stripe a durable store's removal advanced past its read.
	DecodedBlockLookupStaleRemoved
	// DecodedBlockLookupDisabled is a miss in a scope whose pool is disabled.
	DecodedBlockLookupDisabled
	// DecodedBlockLookupOutcomeCount is the number of lookup outcomes.
	DecodedBlockLookupOutcomeCount
)

// decodedBlockLookupOutcomeNames are the metric labels of the outcomes.
var decodedBlockLookupOutcomeNames = [DecodedBlockLookupOutcomeCount]string{
	"front_hit",
	"hit",
	"absent",
	"stale_epoch",
	"stale_removed",
	"disabled",
}

// String returns the outcome's metric label.
func (o DecodedBlockLookupOutcome) String() string {
	if o < DecodedBlockLookupOutcomeCount {
		return decodedBlockLookupOutcomeNames[o]
	}
	return "unknown"
}

// DecodedBlockNotStoredReason is why a decoded block did not enter the shared
// pool.
type DecodedBlockNotStoredReason uint8

const (
	// DecodedBlockNotStoredNotDurable is a read no durable store marked.
	DecodedBlockNotStoredNotDurable DecodedBlockNotStoredReason = iota
	// DecodedBlockNotStoredStaged is a read of staged content, which vetoes
	// admission even when a durable store also marked it.
	DecodedBlockNotStoredStaged
	// DecodedBlockNotStoredUnverified is a read whose stored bytes the cursor
	// did not verify and size.
	DecodedBlockNotStoredUnverified
	// DecodedBlockNotStoredUncacheable is a block with no cache key, such as
	// an empty ref.
	DecodedBlockNotStoredUncacheable
	// DecodedBlockNotStoredUncloneable is a block the cache could not clone.
	DecodedBlockNotStoredUncloneable
	// DecodedBlockNotStoredTokenStale is a read whose store token an
	// invalidation outdated before the store.
	DecodedBlockNotStoredTokenStale
	// DecodedBlockNotStoredRemovedSince is a read a durable store's removal
	// followed before the store.
	DecodedBlockNotStoredRemovedSince
	// DecodedBlockNotStoredCostUnknown is a block whose cost the cache could
	// not compute.
	DecodedBlockNotStoredCostUnknown
	// DecodedBlockNotStoredRejected is an entry larger than a pool shard.
	DecodedBlockNotStoredRejected
	// DecodedBlockNotStoredDisabled is a store into a disabled or closed pool.
	DecodedBlockNotStoredDisabled
	// DecodedBlockNotStoredReasonCount is the number of reasons.
	DecodedBlockNotStoredReasonCount
)

// decodedBlockNotStoredReasonNames are the metric labels of the reasons.
var decodedBlockNotStoredReasonNames = [DecodedBlockNotStoredReasonCount]string{
	"not_durable",
	"staged",
	"unverified",
	"uncacheable",
	"uncloneable",
	"token_stale",
	"removed_since",
	"cost_unknown",
	"rejected",
	"disabled",
}

// String returns the reason's metric label.
func (r DecodedBlockNotStoredReason) String() string {
	if r < DecodedBlockNotStoredReasonCount {
		return decodedBlockNotStoredReasonNames[r]
	}
	return "unknown"
}

// decodedBlockOutcomeStripes is the number of counter stripes of a pool. Ref
// keys end in a digest byte, which spreads concurrent readers of different
// blocks over the stripes.
const decodedBlockOutcomeStripes = 8

// decodedBlockOutcomeStripe is one stripe of a pool's outcome counters, 128
// bytes, so two stripes share at most one cache line.
type decodedBlockOutcomeStripe struct {
	lookups   [DecodedBlockLookupOutcomeCount]atomic.Uint64
	notStored [DecodedBlockNotStoredReasonCount]atomic.Uint64
}

// decodedBlockOutcomes counts a pool's lookup outcomes and not-stored reasons.
// Recording one is an atomic add on a stripe chosen from the ref key, with no
// allocation.
type decodedBlockOutcomes [decodedBlockOutcomeStripes]decodedBlockOutcomeStripe

// stripe returns the stripe of refKey.
func (o *decodedBlockOutcomes) stripe(refKey string) *decodedBlockOutcomeStripe {
	if n := len(refKey); n != 0 {
		return &o[refKey[n-1]%decodedBlockOutcomeStripes]
	}
	return &o[0]
}

// lookup records one lookup outcome. A nil receiver records nothing.
func (o *decodedBlockOutcomes) lookup(refKey string, outcome DecodedBlockLookupOutcome) {
	if o != nil {
		o.stripe(refKey).lookups[outcome].Add(1)
	}
}

// notStored records one not-stored reason. A nil receiver records nothing.
func (o *decodedBlockOutcomes) notStored(refKey string, reason DecodedBlockNotStoredReason) {
	if o != nil {
		o.stripe(refKey).notStored[reason].Add(1)
	}
}

// sum adds up the stripes.
func (o *decodedBlockOutcomes) sum() (lookups [DecodedBlockLookupOutcomeCount]uint64, notStored [DecodedBlockNotStoredReasonCount]uint64) {
	if o == nil {
		return lookups, notStored
	}
	for i := range o {
		s := &o[i]
		for j := range s.lookups {
			lookups[j] += s.lookups[j].Load()
		}
		for j := range s.notStored {
			notStored[j] += s.notStored[j].Load()
		}
	}
	return lookups, notStored
}

// recordLookup records a lookup outcome in the cache's pool. A nil cache
// records nothing.
func (c *DecodedBlockCache) recordLookup(refKey string, outcome DecodedBlockLookupOutcome) {
	if c != nil {
		c.pool.outcomes.lookup(refKey, outcome)
	}
}

// recordNotStored records why a block did not enter the cache's pool. A nil
// cache records nothing.
func (c *DecodedBlockCache) recordNotStored(refKey string, reason DecodedBlockNotStoredReason) {
	if c != nil {
		c.pool.outcomes.notStored(refKey, reason)
	}
}
