package s4db

import (
	"bytes"
	"context"
	"time"
)

// Stats describes the file and this handle, from counters the writer and
// the tail keep, so it takes no lock.
type Stats struct {
	// FileBytes is the file length.
	FileBytes int64
	// LiveBytes is the value bytes current references point at.
	LiveBytes int64
	// DeadBytes is the value bytes in value chunks no reference points at.
	DeadBytes int64
	// FreeBytes is the free chunks below the end.
	FreeBytes int64
	// PendingBytes is the freed chunks a snapshot may still read.
	PendingBytes int64
	// KeyCount is the number of keys.
	KeyCount int64
	// CollectBytes bounds the disk that deleting keys takes before the space
	// they free returns.
	CollectBytes int64
	// Chunks counts the chunks by kind.
	Chunks ChunkCounts
	// Runs counts the runs per level.
	Runs []int
	// MemtableBytes estimates the memory of the entries since the last flush.
	MemtableBytes int64
	// Written is the bytes this handle wrote to the file since open.
	Written int64
	// UserBytes is the key and value bytes this handle's commits carried.
	UserBytes int64
	// BackgroundWaitMax is the longest a commit of this handle waited on
	// background work: a background step's hold of the writer lock while the
	// commit waited for it, or a wait at the memtable ceiling.
	BackgroundWaitMax time.Duration
	// Slots lists the live reader slots and their pins.
	Slots []SlotPin
	// Families is the flow of each of Options.Families in order, then of the
	// keys under none.
	Families []Flow
}

// Stats reports the file and this handle as of the published state.
func (db *DB) Stats(ctx context.Context) (Stats, error) {
	// Measure the file and walk the table.
	st := db.cur.Load()
	size, err := db.s.size()
	if err != nil {
		return Stats{}, err
	}
	s := Stats{
		FileBytes:         size,
		KeyCount:          st.m.count,
		MemtableBytes:     st.mem.bytes,
		Written:           db.written.Load(),
		UserBytes:         db.userBytes.Load(),
		BackgroundWaitMax: time.Duration(db.waitMax.Load()),
		CollectBytes:      4*db.budget + runTarget*(1+levelRatio) + 4*chunkSize,
	}
	for c, e := range st.m.table.entries {
		if c == 0 {
			continue
		}
		switch e.kind {
		case chunkFree:
			s.Chunks.Free++
			s.FreeBytes += chunkSize
		case chunkLog:
			s.Chunks.Log++
		case chunkValue:
			s.Chunks.Value++
			s.LiveBytes += int64(e.live)
			s.DeadBytes += int64(e.used) - int64(e.live)
		case chunkRun:
			s.Chunks.Run++
		case chunkManifest:
			s.Chunks.Manifest++
		case chunkReserved:
			s.Chunks.Reserved++
		case chunkPending:
			s.Chunks.Pending++
			s.PendingBytes += chunkSize
		}
	}
	for _, r := range st.m.runs {
		for len(s.Runs) <= int(r.level) {
			s.Runs = append(s.Runs, 0)
		}
		s.Runs[r.level]++
	}

	// Read the slots and the flows.
	s.Slots, err = db.slotPins()
	if err != nil {
		return Stats{}, err
	}
	s.Families = make([]Flow, len(db.flows))
	for i := range db.flows {
		s.Families[i] = Flow{Written: db.flows[i].written.Load(), Freed: db.flows[i].freed.Load()}
	}
	return s, nil
}

// slotPins lists the live reader slots and their pins.
func (db *DB) slotPins() ([]SlotPin, error) {
	// Read the slots.
	b := make([]byte, pageSize)
	if _, err := db.s.ReadAt(b, slotPage*pageSize); err != nil {
		return nil, err
	}

	// Report each slot whose process holds its lock.
	var pins []SlotPin
	var empty [slotSize]byte
	for i := range slots {
		rec := b[i*slotSize : (i+1)*slotSize]
		if bytes.Equal(rec, empty[:]) {
			continue
		}
		live := i == db.slot
		if !live {
			var err error
			if live, err = db.s.held(lockSlot + int64(i)); err != nil {
				return nil, err
			}
		}
		if !live {
			continue
		}
		p, _ := decodePin(rec)
		pins = append(pins, SlotPin{Slot: i, Seq: p.seq, Version: p.version, Own: i == db.slot})
	}
	return pins, nil
}

// family returns the index of the family key counts under: the first of
// Options.Families it starts with, or the last index for none.
func (db *DB) family(key []byte) int {
	for i, p := range db.opts.Families {
		if bytes.HasPrefix(key, p) {
			return i
		}
	}
	return len(db.opts.Families)
}

// addFlows adds a commit's flows to the handle's counters.
func (db *DB) addFlows(flows []Flow) {
	for i, f := range flows {
		db.flows[i].written.Add(f.Written)
		db.flows[i].freed.Add(f.Freed)
	}
}

// recordWait records that a commit waited d on background work.
func (db *DB) recordWait(d time.Duration) {
	for {
		cur := db.waitMax.Load()
		if int64(d) <= cur || db.waitMax.CompareAndSwap(cur, int64(d)) {
			return
		}
	}
}
