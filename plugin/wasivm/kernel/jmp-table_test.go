package kernel

import "testing"

// TestJmpTableCopiedBuffer checks the pattern of bash's function returns: a
// jmp_buf copied aside around a nested setjmp into the same buffer, then
// copied back, jumps to the outer setjmp's state.
func TestJmpTableCopiedBuffer(t *testing.T) {
	// The outer function sets the buffer, then saves its contents, the id.
	var tbl jmpTable
	const buf = 0x100
	outer := tbl.record(jmpBufEntry{buf: buf, sp: 0x8000})
	saved := outer

	// The inner function, deeper on the stack, sets the same buffer.
	inner := tbl.record(jmpBufEntry{buf: buf, sp: 0x7000})
	if inner == outer {
		t.Fatalf("inner id %d repeats the outer one", inner)
	}

	// The inner function returns, restoring the buffer, and the outer one
	// jumps through it to its own state.
	e, ok := tbl.jump(saved)
	if !ok || e.sp != 0x8000 {
		t.Fatalf("jump = %+v, %t; want the outer state", e, ok)
	}

	// The jump left the inner frame, so its state is gone.
	if _, ok := tbl.jump(inner); ok {
		t.Fatal("the inner state outlived its frame")
	}
}

// TestJmpTableReplaces checks a setjmp into the same buffer from the same
// frame replaces the last entry, so a loop that sets one buffer keeps one
// entry.
func TestJmpTableReplaces(t *testing.T) {
	// Set one buffer from one frame many times.
	var tbl jmpTable
	var id uint32
	for range 100 {
		id = tbl.record(jmpBufEntry{buf: 0x100, sp: 0x8000})
	}

	// Only the last setjmp remains, and it can be jumped to.
	if n := len(tbl.entries); n != 1 {
		t.Fatalf("%d entries, want 1", n)
	}
	if _, ok := tbl.jump(id); !ok {
		t.Fatal("the last state is gone")
	}
}
