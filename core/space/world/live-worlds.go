package space_world

import (
	"github.com/s4wave/spacewave/db/block"
)

// worldReaders reports the Worlds the readers of an engine may still read.
type worldReaders interface {
	// ReadRoots returns the World roots the engine's readers may still read.
	ReadRoots() []*block.BlockRef
}

// liveWorlds are the Worlds the adapter may still read, and the roots beside
// them, gathered once for a sweep or a history trim so that the two agree on
// what is live.
type liveWorlds struct {
	// memory are the Worlds whose pages may be kept in memory: the positions'
	// above the floor, the installed head, the writer head, the Worlds a
	// checkpoint is storing and the Worlds the served engine's readers may
	// still read. A trim holds them under the replay cursor root, and a sweep
	// keeps their pages.
	memory []*block.BlockRef
	// served are the Worlds of memory that are no position: the installed
	// head, the writer head, the Worlds a background store is storing and the
	// Worlds the served engine's readers may still read. A cursor save that
	// ends a replay epoch which stored a page holds them, since the floor
	// covers only the positions and a background store may land after the
	// epoch's pages are freed.
	served []*block.BlockRef
	// candidates are the pending candidates' Worlds. A trim holds them under
	// the pending root, and a sweep keeps their pages.
	candidates []*block.BlockRef
	// stored are the Worlds whose pages left memory: the positions' below the
	// floor and the restored positions', which replay computes again or
	// adopts. A trim keeps them where a history holds them but holds none
	// anew, and a sweep drops their pages.
	stored []*block.BlockRef
	// payloads are the payload roots of the positions above the floor and of
	// the candidates.
	payloads []*block.BlockRef
	// retained are the roots the installed head retains.
	retained []*block.BlockRef
}

// gatherLive gathers the live Worlds. The driver goroutine calls it.
func (a *Adapter) gatherLive() liveWorlds {
	// Gather the positions' and the restored positions' Worlds.
	var l liveWorlds
	r := a.replay
	for i, pos := range r.positions {
		root := pos.state.GetHeadRef().GetRootRef()
		if i < r.floor {
			l.stored = append(l.stored, root)
			continue
		}
		l.memory = append(l.memory, root)
		l.payloads = append(l.payloads, pos.payloads...)
	}
	for _, pos := range r.restored {
		l.stored = append(l.stored, pos.state.GetHeadRef().GetRootRef())
	}

	// Add the heads, the candidates and the retained roots.
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.served = append(l.served, a.head.Root, a.writer.Root)
		for _, roots := range a.storing {
			l.served = append(l.served, roots...)
		}
		for _, adm := range a.pending {
			l.candidates = append(l.candidates, adm.candidate.Root)
			l.payloads = append(l.payloads, adm.payloads...)
		}
		for _, root := range a.headState.GetRetainedRoots() {
			l.retained = append(l.retained, root.GetRootRef())
		}
	})

	// Add the engine readers' Worlds.
	l.served = append(l.served, a.readRoots()...)
	l.memory = append(l.memory, l.served...)
	return l
}

// readRoots returns the Worlds the served engine's readers may still read,
// read outside bcast, since the engine takes its own locks.
func (a *Adapter) readRoots() []*block.BlockRef {
	var readers worldReaders
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		readers = a.readers
	})
	if readers == nil {
		return nil
	}
	return readers.ReadRoots()
}
