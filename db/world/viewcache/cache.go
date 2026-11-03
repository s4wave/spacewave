package viewcache

import (
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/world"
)

// Cache keeps values derived from World revisions, shared by every
// Store bound to the same World, so the watchers that each read a new
// revision compute them once. A value carries forward to later revisions
// until one changes something it read, or the time it asked to rerun at
// passes. A reader whose snapshot is older than the cache receives the value
// at the newest revision. Values are read-only to their callers. Only read
// transactions use it: a write transaction's own changes are not part of the
// revision it started from.
type Cache struct {
	// engine is the World the values derive from, read without tracking.
	engine world.Engine
	// observer receives optional computation and invalidation metrics.
	observer Observer

	// seqno is the newest revision every entry is known to hold through. It
	// changes only under mtx, and head reads it without mtx.
	seqno atomic.Uint64
	// watched is the newest revision the World's change watch reported
	// (follow), or 0 while no watch feeds the cache and head asks the World.
	watched atomic.Uint64
	// target is the newest revision a background catch-up was asked to
	// reach; the one running catch-up reads it again after each advance.
	target atomic.Uint64
	// catchingUp is set while the one background catch-up runs.
	catchingUp atomic.Bool

	// mtx guards the fields below.
	mtx sync.Mutex
	// entries holds each value that still holds at seqno, by name.
	entries map[string]*revisionEntry
	// advancing is closed when the running catch-up to a newer revision
	// ends; nil when none runs.
	advancing chan struct{}
	// history holds the changes of the latest catch-ups, oldest first, to
	// check a value computed at a revision the cache has since passed.
	history []cacheAdvance
	// typed holds the last key listing of each type typedKeys read, by type.
	typed map[string]*typedKeyIndex
	// index holds the finished values kept in entries by every read set
	// checking them consults, so a catch-up checks only the values its
	// changes name.
	index *world.ChangeIndex[*revisionEntry]
	// faceted holds the indexed values that read a part by facet, whose
	// parts a catch-up may replace.
	faceted map[*revisionEntry]struct{}
	// previous holds, by name, the last value a catch-up dropped among those
	// a reader asked for through recent, until a finished value replaces it.
	previous map[string]previousValue
	// refreshing holds the names a background refresh computes again.
	refreshing map[string]struct{}
	// moved is closed, and cleared, when anything recent would answer
	// changes: seqno advances, or a finished value replaces a previous one.
	// nil until a waiter asks for it (changed).
	moved chan struct{}
}

// New constructs a cache over an untracked World engine. The caller owns
// authorization and must encode every non-World input, including the principal,
// in each value's key. Only immutable read results may enter the cache.
func New(engine world.Engine, observer Observer) *Cache {
	return &Cache{engine: engine, observer: observer}
}

// Engine returns the immutable engine supplying the cached revisions.
func (c *Cache) Engine() world.Engine { return c.engine }

// Seqno returns the newest revision every retained value holds through.
func (c *Cache) Seqno() uint64 { return c.seqno.Load() }
