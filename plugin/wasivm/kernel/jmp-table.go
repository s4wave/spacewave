package kernel

import (
	"maps"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// jmpBufEntry is the execution state setjmp records for longjmp: the engine
// snapshot and the C stack, which the engine snapshot does not hold, and the
// kernel's own per-call state.
type jmpBufEntry struct {
	snapshot engine.Snapshot
	// buf is the address of the jmp_buf setjmp filled.
	buf uint32
	// sp is the C stack pointer at the setjmp.
	sp uint32
	// cstack holds the C stack from sp to the heap base.
	cstack []byte
	// handlerDepth is the signal handlers running at the setjmp. A longjmp
	// out of a handler leaves the handlers it skips.
	handlerDepth int
}

// jmpTable holds the states setjmp recorded, each by an id setjmp writes in
// its jmp_buf. A program may copy a jmp_buf aside and back around a nested
// setjmp, as bash does for its function returns, so the id travels with the
// buffer's contents rather than its address.
//
// The C stack grows down, so a setjmp at stack pointer sp finds every frame
// below sp returned: their entries can no longer be jumped to. A setjmp into
// the same buffer from the same frame replaces its last entry. A frame that
// copies a jmp_buf holds it in its own stack frame, so the nested setjmp
// runs at a lower sp and the copied entry survives.
type jmpTable struct {
	entries map[uint32]jmpBufEntry
	// last is the last id given out.
	last uint32
}

// record stores e and returns its id, dropping the entries it supersedes.
func (t *jmpTable) record(e jmpBufEntry) uint32 {
	// Drop the entries of returned frames and the entry e replaces.
	for id, old := range t.entries {
		if old.sp < e.sp || (old.sp == e.sp && old.buf == e.buf) {
			delete(t.entries, id)
		}
	}

	// Store the entry under an id that is never zero.
	if t.entries == nil {
		t.entries = make(map[uint32]jmpBufEntry)
	}
	t.last++
	if t.last == 0 {
		t.last++
	}
	t.entries[t.last] = e
	return t.last
}

// jump returns the entry of id and drops the entries of the frames the jump
// leaves. It reports false for an id no setjmp recorded or a returned frame.
func (t *jmpTable) jump(id uint32) (jmpBufEntry, bool) {
	e, ok := t.entries[id]
	if !ok {
		return jmpBufEntry{}, false
	}
	for id, old := range t.entries {
		if old.sp < e.sp {
			delete(t.entries, id)
		}
	}
	return e, true
}

// clone returns a copy for a fork child, whose memory holds the same ids.
func (t *jmpTable) clone() jmpTable {
	return jmpTable{entries: maps.Clone(t.entries), last: t.last}
}
