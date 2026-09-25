//go:build !js

package device

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/pkg/errors"
)

// TestDevices checks the Device contract on every implementation.
func TestDevices(t *testing.T) {
	devices := map[string]func(t *testing.T) Device{
		"memory": func(*testing.T) Device { return NewMemory() },
		"dir": func(t *testing.T) Device {
			d, err := OpenDir(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = d.Close() })
			return d
		},
	}
	for name, open := range devices {
		t.Run(name, func(t *testing.T) {
			testDevice(t, open(t))
		})
	}
}

// testDevice runs the contract checks on one device.
func testDevice(t *testing.T, d Device) {
	ctx := t.Context()

	// Writes apply in order, and a write past the end zero fills the gap.
	writes := []Write{
		{Name: "a", Offset: 0, Data: []byte("hello")},
		{Name: "a", Offset: 3, Data: []byte("LO world")},
		{Name: "b", Offset: 4, Data: []byte("x")},
	}
	if err := d.Write(ctx, writes, true); err != nil {
		t.Fatal(err)
	}
	expectFile(t, d, "a", []byte("helLO world"))
	expectFile(t, d, "b", []byte("\x00\x00\x00\x00x"))

	// A batch of reads fills each range, and a range past the end fails.
	reads := []Read{
		{Name: "a", Offset: 6, Data: make([]byte, 5)},
		{Name: "b", Offset: 4, Data: make([]byte, 1)},
	}
	if err := d.Read(ctx, reads); err != nil {
		t.Fatal(err)
	}
	if string(reads[0].Data) != "world" || string(reads[1].Data) != "x" {
		t.Fatalf("read %q and %q", reads[0].Data, reads[1].Data)
	}
	err := d.Read(ctx, []Read{{Name: "a", Offset: 8, Data: make([]byte, 4)}})
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("read past end: %v", err)
	}
	err = d.Read(ctx, []Read{{Name: "missing", Data: make([]byte, 1)}})
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("read of missing file: %v", err)
	}

	// Truncate shrinks and zero fills growth.
	if err := d.Truncate(ctx, "a", 3); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(ctx, "a", 5); err != nil {
		t.Fatal(err)
	}
	expectFile(t, d, "a", []byte("hel\x00\x00"))

	// Remove ignores missing files, and List reports what remains.
	if err := d.Remove(ctx, []string{"b", "missing"}); err != nil {
		t.Fatal(err)
	}
	files, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []File{{Name: "a", Size: 5}}) {
		t.Fatalf("list %v", files)
	}

	// Names must be flat.
	if err := d.Write(ctx, []Write{{Name: "x/y", Data: []byte("z")}}, false); err == nil {
		t.Fatal("nested name accepted")
	}
}

// expectFile checks a file's full contents.
func expectFile(t *testing.T, d Device, name string, want []byte) {
	t.Helper()
	files, err := d.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(files, func(f File) bool { return f.Name == name })
	if i < 0 || files[i].Size != int64(len(want)) {
		t.Fatalf("%s: listed %v, want size %d", name, files, len(want))
	}
	got := make([]byte, len(want))
	if err := d.Read(t.Context(), []Read{{Name: name, Data: got}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: %q, want %q", name, got, want)
	}
}

// TestDirReopen checks that flushed files survive reopening a directory.
func TestDirReopen(t *testing.T) {
	root := t.TempDir()
	d, err := OpenDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Write(t.Context(), []Write{{Name: "a", Data: []byte("kept")}}, true); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = OpenDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	expectFile(t, d, "a", []byte("kept"))
}

// TestMemoryPowerLoss checks that a power loss keeps every flushed write and
// leaves each later write whole, absent, or torn at sector boundaries.
func TestMemoryPowerLoss(t *testing.T) {
	ctx := context.Background()
	flushed := bytes.Repeat([]byte{'f'}, 2*SectorSize)
	unflushed := bytes.Repeat([]byte{'u'}, 2*SectorSize)
	outcomes := make(map[string]bool)
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

	// A reopened handle finds the file length.
	h, err = OpenHandle(ctx, m, "f")
	if err != nil {
		t.Fatal(err)
	}
	if size, _ := h.Size(); size != 7 {
		t.Fatalf("reopened size %d, want 7", size)
	}
}
