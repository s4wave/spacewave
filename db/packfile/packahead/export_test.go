package packahead

import (
	"context"
	"os"
	"testing"
)

// FailAppendWrite runs an append with a read-only handle for the open pack.
// The test must hold off rotation and any sync that would acquire this handle.
func FailAppendWrite(t *testing.T, l *Log, blocks []Block) error {
	// Give the append a handle that rejects positioned writes.
	t.Helper()
	p := l.open
	writable := p.f
	readOnly, err := os.Open(p.path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { p.f = readOnly })
	defer l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { p.f = writable })

	// Let the actual write failure mark the pack for recovery.
	return l.Append(context.Background(), blocks)
}
