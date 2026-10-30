//go:build !js

package paylog

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestMaintenance checks that maintenance moves the live blocks out of a
// mostly dead segment, removes the emptied segment, and keeps the statistics,
// across a reopen.
func TestMaintenance(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			// Store two blocks in segment 1.
			ctx := t.Context()
			d := device.NewMemory()
			s := reopen(t, e, d, nil)
			refs := putBlocks(t, s, "first block", "other block")
			checkStats(t, s, 2, 22)

			// Remove the first from a reopened store writing segment 2.
			s = reopen(t, e, d, s)
			if err := s.RmBlock(ctx, refs[0]); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			checkStats(t, s, 1, 11)

			// Move the live block, then remove the emptied segment.
			for range 2 {
				if err := s.Maintenance(ctx); err != nil {
					t.Fatal(err)
				}
			}
			checkSegments(t, d, "seg-2")

			// Reopen and check the blocks and statistics.
			s = reopen(t, e, d, s)
			checkBlocks(t, s, refs, "", "other block")
			checkStats(t, s, 1, 11)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestMaintenanceRemoved checks that a block removed while maintenance moves
// it stays removed.
func TestMaintenanceRemoved(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			// Store two blocks and remove the first after a reopen.
			ctx := t.Context()
			d := &hookDevice{Device: device.NewMemory()}
			s := reopen(t, e, d, nil)
			refs := putBlocks(t, s, "first block", "other block")
			s = reopen(t, e, d, s)
			if err := s.RmBlock(ctx, refs[0]); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Sync(ctx); err != nil {
				t.Fatal(err)
			}

			// Remove the second while maintenance reads it for the move. The
			// move publishes only the blocks still in place.
			onRead := func() {
				if err := s.RmBlock(ctx, refs[1]); err != nil {
					t.Error(err)
				}
			}
			d.onRead.Store(&onRead)
			if err := s.Maintenance(ctx); err != nil {
				t.Fatal(err)
			}
			d.onRead.Store(nil)
			checkBlocks(t, s, refs, "", "")
			checkStats(t, s, 0, 0)

			// The next pass removes the old segment. The open one keeps the
			// dead copy.
			if err := s.Maintenance(ctx); err != nil {
				t.Fatal(err)
			}
			checkSegments(t, d, "seg-2")
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// hookDevice is a device that calls onRead, once, before a segment read. An
// index may read its own files from other goroutines meanwhile.
type hookDevice struct {
	device.Device
	// onRead is called before the next segment read, then cleared.
	onRead atomic.Pointer[func()]
}

// Read calls and clears onRead before a segment read, then reads.
func (d *hookDevice) Read(ctx context.Context, reads []device.Read) error {
	segment := slices.ContainsFunc(reads, func(r device.Read) bool {
		return strings.HasPrefix(r.Name, segmentPrefix)
	})
	if segment {
		if fn := d.onRead.Swap(nil); fn != nil {
			(*fn)()
		}
	}
	return d.Device.Read(ctx, reads)
}

// reopen closes s, if set, and opens a store on d with e.
func reopen(t *testing.T, e engine, d device.Device, s *Store) *Store {
	// Close the previous store, then open the next.
	t.Helper()
	if s != nil {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s, err := e.openStore(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// putBlocks stores and publishes a block for each payload.
func putBlocks(t *testing.T, s *Store, payloads ...string) []*block.BlockRef {
	// Put each payload, then publish them all.
	t.Helper()
	refs := make([]*block.BlockRef, len(payloads))
	for i, p := range payloads {
		ref, _, err := s.PutBlock(t.Context(), []byte(p), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	if _, err := s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	return refs
}

// checkBlocks checks the payload of each ref, empty for an absent block.
func checkBlocks(t *testing.T, s *Store, refs []*block.BlockRef, want ...string) {
	t.Helper()
	for i, ref := range refs {
		data, found, err := s.GetBlock(t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if found != (want[i] != "") || string(data) != want[i] {
			t.Fatalf("block %d: found %v data %q, want %q", i, found, data, want[i])
		}
	}
}

// checkStats checks the published block statistics.
func checkStats(t *testing.T, s *Store, count, size uint64) {
	t.Helper()
	gotCount, gotSize, err := s.BlockStats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotCount != count || gotSize != size {
		t.Fatalf("block stats %d blocks %d bytes, want %d blocks %d bytes", gotCount, gotSize, count, size)
	}
}

// checkSegments checks the segment files on d.
func checkSegments(t *testing.T, d device.Device, want ...string) {
	// List the segment files in order and compare.
	t.Helper()
	files, err := d.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		if _, ok := parseSegment(f.Name); ok {
			got = append(got, f.Name)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("segments %v, want %v", got, want)
	}
}
