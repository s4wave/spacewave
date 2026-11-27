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
	// memory holds the Worlds whose pages may be kept in memory: the
	// Worlds of the positions above the floor, the installed head, the
	// writer head, the Worlds a checkpoint is storing, the heads the engine
	// may still publish (the reportable head, each head a watch reports and
	// each accepted admission's heads until its Release) and the Worlds the
	// served engine's readers may still read. A trim holds them under the
	// replay cursor root, and a sweep keeps their pages.
	memory []*block.BlockRef
	// candidates are the pending candidates' Worlds. A trim holds them under
	// the pending root, and a sweep keeps their pages.
	candidates []*block.BlockRef
	// stored holds the Worlds whose pages left memory: the Worlds of the
	// positions below the floor and of the restored positions, which
	// replay computes again or adopts. A trim keeps them where a history
	// holds them but holds none anew, and a sweep drops their pages.
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
		// Add the installed, writer and reportable heads and the storing
		// Worlds.
		l.memory = append(l.memory, a.head.Root, a.writer.Root, a.reportable.Root)
		for _, roots := range a.storing {
			l.memory = append(l.memory, roots...)
		}

		// Add the heads the engine may publish after the Space moved on.
		// Each stays here until the engine's readers hold it, which
		// readRoots reads after this section.
		for _, w := range a.watches {
			l.memory = append(l.memory, w.reporting.Root)
		}
		for adm := range a.accepted {
			l.memory = append(l.memory, adm.candidate.Root, adm.head.Root)
		}

		// Add the candidates and the retained roots.
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
