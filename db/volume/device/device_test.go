//go:build !js

package device

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"testing"

	"github.com/pkg/errors"
)

// TestMemoryPowerLoss checks that a power loss keeps every flushed write and
// leaves each later write whole, absent, or torn at sector boundaries.
func TestMemoryPowerLoss(t *testing.T) {
	// Prepare the flushed and unflushed patterns and the outcome set.
	ctx := context.Background()
	flushed := bytes.Repeat([]byte{'f'}, 2*SectorSize)
	unflushed := bytes.Repeat([]byte{'u'}, 2*SectorSize)
	outcomes := make(map[string]bool)

	// Crash each seed's device after one flushed write and one unflushed write.
	for seed := range uint64(64) {
		m := NewMemory()
		if err := m.Write(ctx, []Write{{Name: "a", Data: flushed}}, true); err != nil {
			t.Fatal(err)
		}
		m.CrashAfter(1)
		if err := m.Write(ctx, []Write{{Name: "a", Offset: SectorSize, Data: unflushed}}, false); err != nil {
			t.Fatal(err)
		}
		err := m.Write(ctx, []Write{{Name: "b", Data: []byte("lost")}}, true)
		if !errors.Is(err, ErrCrashed) {
			t.Fatalf("crashing write: %v", err)
		}
		if err := m.Read(ctx, []Read{{Name: "a", Data: make([]byte, 1)}}); !errors.Is(err, ErrCrashed) {
			t.Fatalf("read after crash: %v", err)
		}
		m.PowerLoss(rand.New(rand.NewPCG(seed, seed)))

		// The flushed first sector survives; each later sector is either
		// version, or zero past the flushed end.
		data := m.files["a"]
		if !bytes.Equal(data[:SectorSize], flushed[:SectorSize]) {
			t.Fatalf("seed %d: flushed sector lost", seed)
		}

		// Classify each later sector as unflushed, flushed, or zero.
		var outcome []byte
		for start := SectorSize; start < len(data); start += SectorSize {
			sector := data[start : start+SectorSize]
			switch {
			case bytes.Equal(sector, unflushed[:SectorSize]):
				outcome = append(outcome, 'u')
			case bytes.Equal(sector, flushed[:SectorSize]):
				outcome = append(outcome, 'f')
			case bytes.Equal(sector, make([]byte, SectorSize)):
				outcome = append(outcome, '0')
			default:
				t.Fatalf("seed %d: sector at %d is torn inside", seed, start)
			}
		}
		outcomes[string(outcome)] = true
	}

	// Across seeds the loss keeps the whole write, drops it, and tears it.
	for _, want := range []string{"uu", "f", "fu", "u0"} {
		if !outcomes[want] {
			t.Errorf("outcome %q never occurred in %v", want, outcomes)
		}
	}
}

// TestHandle checks that a Handle collects writes into one device call per
// Sync, reads its own collected writes, and reports the end of the file.
func TestHandle(t *testing.T) {
	// Open a handle on a fresh memory device.
	ctx := t.Context()
	m := NewMemory()
	h, err := OpenHandle(ctx, m, "f")
	if err != nil {
		t.Fatal(err)
	}

	// Collected writes cost no device call until Sync issues them in one.
	for i, part := range []string{"ab", "cd", "ef"} {
		if _, err := h.WriteAt([]byte(part), int64(2*i)); err != nil {
			t.Fatal(err)
		}
	}
	if m.Calls() != 0 {
		t.Fatalf("%d device calls before Sync", m.Calls())
	}

	// Sync issues exactly one device call.
	if err := h.Sync(); err != nil {
		t.Fatal(err)
	}
	if m.Calls() != 1 {
		t.Fatalf("%d device calls after Sync, want 1", m.Calls())
	}

	// A read sees collected writes and stops at the end of the file.
	if _, err := h.WriteAt([]byte("G"), 6); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := h.ReadAt(buf, 0)
	if n != 7 || !errors.Is(err, io.EOF) || string(buf[:n]) != "abcdefG" {
		t.Fatalf("read %q, %v", buf[:n], err)
	}

	// Close issues the collected write, and a reopened handle finds the
	// file length.
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = OpenHandle(ctx, m, "f")
	if err != nil {
		t.Fatal(err)
	}
	if size, _ := h.Size(); size != 7 {
		t.Fatalf("reopened size %d, want 7", size)
	}
}

// failDevice is a Memory device whose writes fail while fail is set.
type failDevice struct {
	*Memory
	fail bool
}

// Write fails while fail is set and writes to the memory device otherwise.
func (d *failDevice) Write(ctx context.Context, ws []Write, flush bool) error {
	if d.fail {
		return errors.New("device full")
	}
	return d.Memory.Write(ctx, ws, flush)
}

// TestHandleFailedWrite checks that a Handle keeps collected writes when the
// device refuses them: reads still see them and the next Sync issues them.
func TestHandleFailedWrite(t *testing.T) {
	// Collect writes over an existing file, then fail the Sync.
	ctx := t.Context()
	d := &failDevice{Memory: NewMemory()}
	if err := d.Write(ctx, []Write{{Name: "f", Data: []byte("abcd")}}, true); err != nil {
		t.Fatal(err)
	}
	h, err := OpenHandle(ctx, d, "f")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []Write{{Offset: 2, Data: []byte("XYZ")}, {Offset: 7, Data: []byte("q")}} {
		if _, err := h.WriteAt(w.Data, w.Offset); err != nil {
			t.Fatal(err)
		}
	}
	d.fail = true
	if err := h.Sync(); err == nil {
		t.Fatal("Sync succeeded on a failing device")
	}

	// Reads see the device bytes, the collected writes, and zeros between.
	want := "abXYZ\x00\x00q"
	buf := make([]byte, len(want))
	if _, err := h.ReadAt(buf, 0); err != nil || string(buf) != want {
		t.Fatalf("read %q, %v; want %q", buf, err, want)
	}

	// Once the device accepts writes, Sync issues the kept ones.
	d.fail = false
	if err := h.Sync(); err != nil {
		t.Fatal(err)
	}
	clear(buf)
	if err := d.Read(ctx, []Read{{Name: "f", Data: buf}}); err != nil || string(buf) != want {
		t.Fatalf("device holds %q, %v; want %q", buf, err, want)
	}
}
