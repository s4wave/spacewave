package world_block

import (
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// engineRetirement is an obsolete resource set detached from Engine publication.
// The Engine drains it after unlocking so cleanup can reenter Engine state.
type engineRetirement struct {
	// head is a replaced root and its shared read transaction.
	head *engineHead
	// pub is the Engine's reference to a replaced published head.
	pub *publishedHead
	// readTx is a detached caller-owned read snapshot.
	readTx *Tx
	// readHead is the head that snapshot held for its object reads.
	readHead *heldHead
	// writeTx is the detached mutable transaction.
	writeTx *Tx
	// turn is the detached writer's Authority turn, released after the
	// transaction drains.
	turn world.Turn
	// base is a detached writer's cursor at an unpublished base, released
	// after the transaction drains.
	base *bucket_lookup.Cursor
	// writeTxRel releases the local writer lock after the transaction drains.
	writeTxRel func()
	// watched are heads no longer recorded for change watchers.
	watched []*watchedHead
	// closing marks the retirement of a closing engine, whose replaced head
	// takes no reader pin.
	closing bool
}

// empty reports whether there are no detached resources to drain.
func (r engineRetirement) empty() bool {
	return r.head == nil &&
		r.pub == nil &&
		r.readTx == nil &&
		r.readHead == nil &&
		r.writeTx == nil &&
		r.turn == nil &&
		r.base == nil &&
		r.writeTxRel == nil &&
		len(r.watched) == 0
}
