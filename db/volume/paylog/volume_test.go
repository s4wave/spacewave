//go:build !js

package paylog

import (
	"bytes"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestVolumeCollect checks that a Volume in the store's domain collects an
// orphaned block from the store, keeps an owned one, and that both outcomes
// survive a reopen.
func TestVolumeCollect(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			// Publish two blocks and orphan the second.
			ctx := t.Context()
			d := device.NewMemory()
			s := reopen(t, e, d, nil)
			v := openVolume(t, s)
			keep, drop := publish(t, v, "keep"), publish(t, v, "drop")
			orphan(t, v, "drop", drop)

			// Collect the orphan.
			res, err := v.Collect(ctx)
			if err != nil || res.Freed != 1 {
				t.Fatalf("collect result=%+v err=%v", res, err)
			}
			checkCollected(t, s, v, keep, drop)

			// Reopen and check the outcome again.
			closeVolume(t, v)
			s = reopen(t, e, d, s)
			v = openVolume(t, s)
			checkCollected(t, s, v, keep, drop)

			// A flipped byte in the kept block is reported, never served
			// or rewritten, and a layered store serves its second copy.
			checkCorruptRead(t, d, v, keep, []byte("block for keep"))
			closeVolume(t, v)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// openVolume opens a Volume in the store's domain under m/, whose collector
// frees at once.
func openVolume(t *testing.T, s *Store) *volume_kvtx.Volume {
	t.Helper()
	conf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	v, err := volume_kvtx.NewVolume(t.Context(), "test/paylog", store_kvkey.NewDefaultKVKey(), s.Domain([]byte("m/")), conf, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// closeVolume closes v, which leaves its store open.
func closeVolume(t *testing.T, v *volume_kvtx.Volume) {
	t.Helper()
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
}

// publish stores one block held by a root of bucket "owner" named id, in one
// commit, and returns its reference.
func publish(t *testing.T, v *volume_kvtx.Volume, id string) *block.BlockRef {
	// Build the block.
	t.Helper()
	data := []byte("block for " + id)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Store the block and its root in one commit.
	entries := []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}
	if err := v.SetBucketRoots(t.Context(), "owner", entries, []block.NamedRoot{{Name: id, Ref: ref}}); err != nil {
		t.Fatal(err)
	}
	return ref
}

// orphan releases the root id of bucket "owner", which holds ref.
func orphan(t *testing.T, v *volume_kvtx.Volume, id string, ref *block.BlockRef) {
	t.Helper()
	if err := v.SetBucketRoots(t.Context(), "owner", nil, []block.NamedRoot{{Name: id, Previous: ref}}); err != nil {
		t.Fatal(err)
	}
}

// checkCollected checks that keep is stored with its root and child list and
// drop is gone from the store and the liveness records.
func checkCollected(t *testing.T, s *Store, v *volume_kvtx.Volume, keep, drop *block.BlockRef) {
	t.Helper()
	for _, c := range []struct {
		ref  *block.BlockRef
		want bool
	}{{keep, true}, {drop, false}} {
		// Check the payload in the store.
		found, err := s.GetBlockExists(t.Context(), c.ref)
		if err != nil || found != c.want {
			t.Fatalf("block %s stored=%v want %v err=%v", c.ref.MarshalString(), found, c.want, err)
		}

		// Check its child list.
		listed, err := v.HasChildLists(t.Context(), []*block.BlockRef{c.ref})
		if err != nil {
			t.Fatal(err)
		}
		if listed[0] != c.want {
			t.Fatalf("block %s listed %v, want %v", c.ref.MarshalString(), listed[0], c.want)
		}
	}
}

// checkCorruptRead flips one byte of ref's payload data on d and checks that a
// read through v returns ErrBlockCorrupt naming the ref and v, leaves the
// device image unchanged, and counts the read in v's health; and that a
// read-through store with v as its primary and a fill serves data from its
// lower store without writing v.
func checkCorruptRead(t *testing.T, d *device.Memory, v *volume_kvtx.Volume, ref *block.BlockRef, data []byte) {
	// Flip one byte of the only stored copy of data.
	t.Helper()
	ctx := t.Context()
	files, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	var at int64 = -1
	for _, f := range files {
		buf := readFile(t, d, f)
		if i := bytes.Index(buf, data); i >= 0 {
			if at >= 0 {
				t.Fatalf("payload stored twice: %s and %s", name, f.Name)
			}
			name, at = f.Name, int64(i)
		}
	}
	if at < 0 {
		t.Fatal("payload not found on the device")
	}
	flipped := []byte{data[0] ^ 0x01}
	if err := d.Write(ctx, []device.Write{{Name: name, Offset: at, Data: flipped}}, true); err != nil {
		t.Fatal(err)
	}
	before := snapshotDevice(t, d)

	// The volume reports the mismatch with the ref and its own id.
	_, _, err = v.GetBlock(ctx, ref)
	var corrupt *block.CorruptError
	if !errors.Is(err, block.ErrBlockCorrupt) || !errors.As(err, &corrupt) {
		t.Fatalf("read of a flipped byte: %v, want ErrBlockCorrupt", err)
	}
	if !corrupt.Ref.EqualsRef(ref) || corrupt.StoreID != v.GetID() {
		t.Fatalf("corrupt error names %s in %q", corrupt.Ref.MarshalString(), corrupt.StoreID)
	}
	h, _ := v.GetHealthSnapshotWithWait()
	if h.GetCorruptReads() != 1 || len(h.GetCorruptRefs()) != 1 || h.GetCorruptRefs()[0] != ref.MarshalString() {
		t.Fatalf("health corrupt reads %d refs %v", h.GetCorruptReads(), h.GetCorruptRefs())
	}

	// A layered store serves the good copy from its lower store.
	lower := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()), ref.GetHash().GetHashType())
	if _, _, err := lower.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err)
	}
	layered := block_store.NewStoreReadThrough(
		func() block.StoreOps { return v },
		func() block.StoreOps { return lower },
		block_store.CacheFill,
	)
	got, found, err := layered.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("layered read: found=%v err=%v data=%q", found, err, got)
	}
	stored, err := layered.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("layered stored read: %v err=%v", stored, err)
	}

	// Neither read wrote the device: the flipped byte stays for repair.
	after := snapshotDevice(t, d)
	if len(after) != len(before) {
		t.Fatalf("device files %d, were %d", len(after), len(before))
	}
	for name, buf := range before {
		if !bytes.Equal(after[name], buf) {
			t.Fatalf("device file %s changed after a corrupt read", name)
		}
	}
}

// readFile reads all of f from d.
func readFile(t *testing.T, d device.Device, f device.File) []byte {
	t.Helper()
	buf := make([]byte, f.Size)
	if err := d.Read(t.Context(), []device.Read{{Name: f.Name, Data: buf}}); err != nil {
		t.Fatal(err)
	}
	return buf
}

// snapshotDevice returns every file on d by name.
func snapshotDevice(t *testing.T, d device.Device) map[string][]byte {
	t.Helper()
	files, err := d.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][]byte, len(files))
	for _, f := range files {
		out[f.Name] = readFile(t, d, f)
	}
	return out
}
