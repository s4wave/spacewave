package s4db

import (
	"context"
)

// SpaceReport details what the file's chunks hold and what keeps freed space
// allocated, to explain a file longer than the data it holds.
type SpaceReport struct {
	// Chunks counts the chunks of each kind.
	Chunks ChunkCounts
	// ValueFill groups the value chunks by the share of their bytes live
	// values hold, in tenths: ValueFill[i] counts the chunks at least i and
	// under i+1 tenths full, and ValueFill[10] the full ones. The cleaner
	// evacuates the emptiest.
	ValueFill [11]int64
	// IndexFill groups the index chunks by the share of their pages in use,
	// as ValueFill does.
	IndexFill [11]int64
	// ValueEndBytes is the end of the highest chunk holding a live value or
	// index page. Cleaning can shorten the file toward it.
	ValueEndBytes int64
	// PendingValueRuns counts the freed value chunk runs a snapshot may still
	// read.
	PendingValueRuns int
	// PendingPageRuns counts the freed index pages and log chunks a snapshot
	// may still read.
	PendingPageRuns int
	// ReservedBytes is the length of the pages this process's checkpoint is
	// writing into.
	ReservedBytes int64
	// UnpunchedBytes is the length of the chunks freed since the last hole
	// punch.
	UnpunchedBytes int64
	// Slots lists the reader slots holding a pin.
	Slots []SlotPin
	// DiskBytes is the space the file system allocates to the file, or -1
	// when it cannot tell.
	DiskBytes int64
	// FreeDiskBytes is the part of DiskBytes inside free chunks, which holes
	// were not punched for, or -1 when it cannot tell.
	FreeDiskBytes int64
}

// allocator is a storage that can report the bytes it allocates.
type allocator interface {
	// allocated returns the bytes in [lo, hi) the file system holds data
	// for, reporting false when it cannot tell.
	allocated(lo, hi int64) (int64, bool)
}

// SpaceReport reports the layout of the file's free space as of the latest
// commit. It holds the writer lock only to copy the space.
func (db *DB) SpaceReport(ctx context.Context) (SpaceReport, error) {
	// Take the writer lock so the space is current and stable.
	if err := db.w.lock(ctx); err != nil {
		return SpaceReport{}, err
	}
	sp := db.w.sp

	// Count the chunks by kind and fill, collecting the free ones, then
	// release the lock.
	r := SpaceReport{
		PendingValueRuns: len(sp.values),
		PendingPageRuns:  len(sp.pages),
		UnpunchedBytes:   pageOff(sp.releasedPages),
	}
	for _, rr := range sp.reserved {
		r.ReservedBytes += pageOff(rr.n)
	}
	var frees []run
	for c, e := range sp.table {
		at := chunkRun(uint64(c), 1) // #nosec G115 -- slice indexes are not negative.
		switch e.kind {
		case chunkFree, chunkHeld:
			r.Chunks.Free++
			frees = append(frees, at)
			continue
		case chunkLog:
			r.Chunks.Log++
		case chunkIndex:
			r.Chunks.Index++
			r.IndexFill[e.live*10/chunkPages]++
		case chunkValue, chunkTail:
			r.Chunks.Value++
			r.ValueFill[e.live*10/chunkSize]++
		}
		if e.live != 0 {
			r.ValueEndBytes = pageOff(at.end())
		}
	}
	end := pageOff(sp.end())
	db.w.unlock(nil)

	// Read the slots and whether a handle holds each.
	b := make([]byte, pageSize)
	if _, err := db.s.ReadAt(b, slotPage*pageSize); err != nil {
		return SpaceReport{}, err
	}
	for i := range slots {
		p, ok := decodePin(b[i*slotSize:])
		if !ok {
			continue
		}
		own := i == db.slot
		live := own
		if !own {
			held, err := db.s.held(lockSlot + int64(i))
			if err != nil {
				return SpaceReport{}, err
			}
			live = held
		}
		r.Slots = append(r.Slots, SlotPin{Slot: i, Seq: p.seq, Ckpt: p.ckpt, Live: live, Own: own})
	}

	// Ask the file system what it allocates, in the whole file and inside
	// the free chunks.
	r.DiskBytes, r.FreeDiskBytes = -1, -1
	if a, ok := db.s.(allocator); ok {
		if n, ok := a.allocated(0, end); ok {
			r.DiskBytes = n
			r.FreeDiskBytes = 0
			for _, fr := range frees {
				n, ok := a.allocated(pageOff(fr.start), pageOff(fr.start+fr.n))
				if !ok {
					r.FreeDiskBytes = -1
					break
				}
				r.FreeDiskBytes += n
			}
		}
	}
	return r, nil
}
