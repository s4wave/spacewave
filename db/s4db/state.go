package s4db

import (
	"github.com/tidwall/btree"
)

// state is one immutable view of the database.
type state struct {
	// seq is the last record applied.
	seq uint64
	// ckpt is the record the tree includes through.
	ckpt uint64
	// gen is the superblock generation of the tree.
	gen uint64
	// sb is the superblock of the tree.
	sb superblock
	// root is the tree root page.
	root uint64
	// count is the number of keys.
	count int64
	// overlay holds changes after ckpt.
	overlay *btree.BTreeG[overlayItem]
	// filter holds every key the overlay has held since ckpt, so lookups of
	// other keys skip the overlay.
	filter *filter
	// overlayBytes estimates the overlay's encoded size.
	overlayBytes int64
	// pos is the offset of the next record.
	pos uint64
	// end is the end of the chunk holding pos.
	end uint64
	// refs counts the snapshots reading the state; publish sets it.
	refs *refCount
}

// newState returns the state of checkpoint sb with no later records, its
// filter sized for an overlay of limit bytes.
func newState(sb superblock, limit int64) *state {
	return &state{
		seq:     sb.seq,
		ckpt:    sb.seq,
		gen:     sb.gen,
		sb:      sb,
		root:    sb.root,
		count:   int64(sb.count), // #nosec G115 -- a file holds under 1<<63 keys.
		overlay: newOverlay(),
		filter:  newFilter(limit),
		pos:     sb.logPos,
		end:     sb.logEnd,
	}
}

// rebase returns st moved onto the later checkpoint sb, keeping the overlay
// entries after it.
func (st *state) rebase(sb superblock, limit int64) *state {
	// Start an empty overlay at the new checkpoint.
	next := *st
	next.ckpt, next.gen, next.sb, next.root = sb.seq, sb.gen, sb, sb.root
	next.overlay = newOverlay()
	next.filter = newFilter(limit)
	next.overlayBytes = 0

	// Carry over the entries the checkpoint does not hold.
	st.overlay.Scan(func(it overlayItem) bool {
		if e := it.e; e.seq > sb.seq {
			next.overlay.Set(it)
			next.filter.add(e.key)
			next.overlayBytes += e.size()
		}
		return true
	})
	return &next
}

// apply applies records in order to st, whose overlay the caller owns.
func (st *state) apply(rs ...*record) {
	// Advance the counters and collect the changes into one allocation.
	n := 0
	for _, r := range rs {
		n += len(r.ops)
	}
	es := make([]oentry, 0, n)
	for _, r := range rs {
		st.seq = r.seq
		if r.kind != kindCommit {
			continue
		}
		st.count += r.delta
		for _, o := range r.ops {
			es = append(es, oentry{key: o.key, del: o.del, val: o.val, seq: r.seq})
		}
	}

	// Insert them in commit order, so a key's later change replaces its
	// earlier ones.
	for i := range es {
		e := &es[i]
		st.filter.add(e.key)
		if old, ok := st.overlay.Set(newOverlayItem(e)); ok {
			st.overlayBytes -= old.e.size()
		}
		st.overlayBytes += e.size()
	}
}
