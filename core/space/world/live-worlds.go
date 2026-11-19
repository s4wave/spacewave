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
	// above the floor and the roots their states retain, the installed head, the writer head, the Worlds a
	// checkpoint is storing and the Worlds the served engine's readers may
	// still read. A trim holds them under the replay cursor root, and a sweep
	// keeps their pages.
	memory []*block.BlockRef
	// candidates are the pending candidates' Worlds. A trim holds them under
	// the pending root, and a sweep keeps their pages.
	candidates []*block.BlockRef
	// stored are the Worlds whose pages left memory: the positions' below the
	// floor and the restored positions', with the roots their states retain,
	// which replay computes again or adopts. A trim keeps them where a history holds them but holds none
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
	// Gather the positions' and the restored positions' Worlds, with the
	// roots each state retains. An applied state may stay uninstalled while
	// replay waits at a missing operation, and a sweep or trim meanwhile must
	// not free a root only it retains, such as a regeneration's base, which
	// the installed head's retained roots do not name yet.
	var l liveWorlds
	r := a.replay
	for i, pos := range r.positions {
		if i < r.floor {
			l.stored = appendStateRoots(l.stored, pos.state)
			continue
		}
		l.memory = appendStateRoots(l.memory, pos.state)
		l.payloads = append(l.payloads, pos.payloads...)
	}
	for _, pos := range r.restored {
		l.stored = appendStateRoots(l.stored, pos.state)
	}

	// Add the heads, the candidates and the retained roots.
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.memory = append(l.memory, a.head.Root, a.writer.Root)
		for _, roots := range a.storing {
			l.memory = append(l.memory, roots...)
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
	l.memory = append(l.memory, a.readRoots()...)
	return l
}

// appendStateRoots appends the World of state and the roots it retains to
// roots.
func appendStateRoots(roots []*block.BlockRef, state *InnerState) []*block.BlockRef {
	roots = append(roots, state.GetHeadRef().GetRootRef())
	for _, root := range state.GetRetainedRoots() {
		roots = append(roots, root.GetRootRef())
	}
	return roots
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
