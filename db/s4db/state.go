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
	// crc is the checksum of record seq, which the next record continues.
	crc uint32
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
		crc:     sb.logCrc,
	}
}

// rebase returns st moved onto the later checkpoint sb, keeping the overlay
// entries after it.
func (st *state) rebase(sb superblock, limit int64) *state {
	c := newCarried(sb.seq, limit)
	st.overlay.Scan(c.add)
	return st.onto(sb, c)
}

// onto returns st moved onto the later checkpoint sb under the overlay
// entries c carried over.
func (st *state) onto(sb superblock, c *carried) *state {
	next := *st
	next.ckpt, next.gen, next.sb, next.root = sb.seq, sb.gen, sb, sb.root
	next.overlay, next.filter, next.overlayBytes = c.overlay, c.filter, c.bytes
	return &next
}

// carried collects the overlay entries after a checkpoint, for the state
// moved onto it.
type carried struct {
	// seq is the last commit the checkpoint includes.
	seq uint64
	// overlay holds the entries after seq.
	overlay *btree.BTreeG[overlayItem]
	// filter holds their keys.
	filter *filter
	// bytes estimates their encoded size.
	bytes int64
}

// newCarried returns an empty carry onto the checkpoint through seq, with a
// filter sized for an overlay of limit bytes.
func newCarried(seq uint64, limit int64) *carried {
	return &carried{seq: seq, overlay: newOverlay(), filter: newFilter(limit)}
}

// add keeps it when it follows the checkpoint, replacing an entry of its
// key added before. It returns true to serve as a scan callback.
func (c *carried) add(it overlayItem) bool {
	if e := it.e; e.seq > c.seq {
		if old, ok := c.overlay.Set(it); ok {
			c.bytes -= old.e.size()
		}
		c.filter.add(e.key)
		c.bytes += e.size()
	}
	return true
}

// addRecord adds the changes of commit record r.
func (c *carried) addRecord(r *record) {
	for _, o := range r.ops {
		c.add(newOverlayItem(&oentry{key: o.key, del: o.del, val: o.val, seq: r.seq}))
	}
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
		st.seq, st.crc = r.seq, r.crc
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
