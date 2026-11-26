package packahead

import (
	"container/list"
	"context"
	"time"
)

// locationBatchSize bounds a background pass over locations under bcast.
const locationBatchSize = 64

// trackRotated puts the former open pack in the registry and rotation queue.
// The caller holds bcast.
func (l *Log) trackRotated(p *pack) {
	// Keep all rotated packs open, including poisoned packs that need a restart.
	p.state = packRotated
	l.packs[p.seq] = p
	l.localBytes += p.written

	// Only a healthy pack can run through the maintainer.
	if !p.poisoned {
		p.order = l.rotated.PushBack(p)
	}
	if len(p.entries) != 0 {
		p.held = l.held.PushBack(p)
	}
}

// poisonLocked removes p from the maintainer's queue after a sync failed.
// The pack stays in the registry for recovery on the next Open.
func (l *Log) poisonLocked(p *pack, err error) {
	// Count the failure and keep the pack out of the finalization queue.
	l.syncFailures++
	if !p.poisoned {
		p.poisoned = true
		l.poisoned++
	}
	if p.order != nil {
		l.rotated.Remove(p.order)
		p.order = nil
	}

	// Withdraw the locations the sync never covered.
	l.failLocked(p, err)
}

// applyFinal moves a rotated pack into the finalized lists. At Open it
// instead inserts a pack recovered from disk. The caller holds bcast or has
// yet to share the Log.
func (l *Log) applyFinal(p *pack, fin *finalized) {
	// Transfer the pack and its byte count out of the rotated queue.
	if p.order != nil {
		l.rotated.Remove(p.order)
		l.localBytes -= p.written
	}

	// Install the finalized index metadata and make the pack visible to drains.
	p.state, p.id, p.path = packFinal, fin.id, fin.path
	p.written, p.synced, p.blocks, p.bloom = fin.size, fin.dataEnd, fin.blocks, fin.bloom
	l.trackFinal(p)
}

// trackFinal indexes a finalized pack. The caller holds bcast or is opening.
func (l *Log) trackFinal(p *pack) {
	// Account for local storage and the work awaiting upload.
	l.packs[p.seq] = p
	l.localBytes += p.written
	l.finalBytes += p.written
	l.pendingBytes += p.written
	p.order = l.finals.PushBack(p)
	p.pending = l.pending.PushBack(p)
	l.finalVersion++

	// Recovered packs with no previous location have an unknown write time.
	if p.held == nil {
		p.held = l.held.PushFront(p)
	}
}

// forgetRotated removes a rotated pack that recovery found empty.
// The caller holds bcast.
func (l *Log) forgetRotated(p *pack) {
	// Remove the pack from each index that could still reach it.
	l.rotated.Remove(p.order)
	if p.held != nil {
		l.held.Remove(p.held)
	}
	delete(l.packs, p.seq)
	l.localBytes -= p.written
}

// packInfo copies the public fields of a finalized pack under bcast.
func packInfo(p *pack) PackInfo {
	return PackInfo{Seq: p.seq, ID: p.id, Path: p.path, Size: p.written, Blocks: p.blocks, BloomFilter: p.bloom, FirstAt: p.firstAt, Uploaded: p.uploaded}
}

// GetPacks returns a consistent snapshot of the finalized packs, oldest first,
// and a channel closed when the Log next changes.
func (l *Log) GetPacks() ([]PackInfo, <-chan struct{}) {
	for {
		// Capture the head and version. Allocate the result outside bcast.
		var next *list.Element
		var count int
		var version uint64
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			next, count, version, waitCh = l.finals.Front(), l.finals.Len(), l.finalVersion, getWaitCh()
		})
		if count == 0 {
			return nil, waitCh
		}
		infos := make([]PackInfo, 0, count)

		// Copy one pack per lock hold. A changed version restarts the view
		// before reading a cursor that may have been unlinked.
		restart := false
		for next != nil && !restart {
			var info PackInfo
			l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if restart = version != l.finalVersion; !restart {
					info, next = packInfo(next.Value.(*pack)), next.Next()
				}
			})
			if !restart {
				infos = append(infos, info)
			}
		}
		if !restart {
			return infos, waitCh
		}
	}
}

// DrainStatus counts a Drain's finalized local packs.
type DrainStatus struct {
	// Pending counts the packs not yet known uploaded.
	Pending int
	// PendingBytes is the size of the pending packs.
	PendingBytes int64
	// LocalBytes is the size of every finalized pack still local, uploaded
	// or not.
	LocalBytes int64
	// Oldest is when the oldest pending pack's first batch was written, zero
	// with none pending or for a pack loaded at Open.
	Oldest time.Time
	// Written counts the packs this Drain wrote to the remote.
	Written uint64
	// WrittenBytes is their size.
	WrittenBytes uint64
	// Listed counts the packs this Drain found the remote listed already,
	// such as one uploaded before a restart, and wrote nothing for.
	Listed uint64
	// Failures counts the Run calls that failed other than by their context.
	Failures uint64
}

// watchDrainStatus reports the finalized pack totals and the next Log notification.
func (l *Log) watchDrainStatus() (DrainStatus, <-chan struct{}) {
	var changed <-chan struct{}
	var st DrainStatus
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		st.Pending, st.PendingBytes, st.LocalBytes = l.pending.Len(), l.pendingBytes, l.finalBytes
		if oldest := l.pending.Front(); oldest != nil {
			st.Oldest = oldest.Value.(*pack).firstAt
		}
		changed = getWaitCh()
	})
	return st, changed
}

// drainStep uploads the oldest pending pack, evicts the oldest finalized pack
// when the log exceeds a positive retention budget, or waits for a Log change.
func (l *Log) drainStep(ctx context.Context, keepBytes int64, upload func(PackInfo) error) error {
	// Select one upload or eviction, or watch for the next pack.
	var pending PackInfo
	var haveUpload bool
	var evict *pack
	var changed <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if oldest := l.pending.Front(); oldest != nil {
			pending, haveUpload = packInfo(oldest.Value.(*pack)), true
			return
		}
		if keepBytes > 0 && l.finalBytes > keepBytes {
			evict = l.finals.Front().Value.(*pack)
			return
		}
		changed = getWaitCh()
	})

	// Run disk and network work outside bcast.
	if haveUpload {
		return upload(pending)
	}
	if evict != nil {
		return l.Evict(ctx, evict.seq)
	}

	// A watch closes on the next change to the drained Log.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}
