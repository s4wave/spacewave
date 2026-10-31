package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// listRefs lists refs in key order from a sorted slice, as a CopyList over
// the volume's block keys does.
func listRefs(refs []*block.BlockRef) CopyList {
	// Sort the refs' keys.
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i], _ = ref.MarshalVT()
	}
	slices.SortFunc(keys, bytes.Compare)
	return func(_ context.Context, after []byte, limit int) ([]*block.BlockRef, []byte, error) {
		// Find the first key past after.
		start, _ := slices.BinarySearchFunc(keys, after, bytes.Compare)
		if after != nil && start < len(keys) && bytes.Equal(keys[start], after) {
			start++
		}

		// Decode up to limit refs from there.
		end := min(start+limit, len(keys))
		out := make([]*block.BlockRef, end-start)
		for i, key := range keys[start:end] {
			out[i] = &block.BlockRef{}
			if err := out[i].UnmarshalVT(bytes.Clone(key)); err != nil {
				return nil, nil, err
			}
		}
		if end == len(keys) {
			return out, nil, nil
		}
		return out, keys[end-1], nil
	}
}

// TestCopyResumes checks that a copy writes every listed block the remote
// lacks, keeps its place across a failure, and records itself complete.
func TestCopyResumes(t *testing.T) {
	// Open a store over an in-memory one.
	ctx := t.Context()
	local := newInmemBlockStore()
	markers := store_kvtx_inmem.NewStore()
	s, err := NewStore(ctx, local, local, markers, true)
	if err != nil {
		t.Fatal(err)
	}

	// Store more blocks than one step lists, one of them already remote.
	remote := newInmemBlockStore()
	var refs []*block.BlockRef
	for i := range 2*copyBatchBlocks + 10 {
		ref, _, err := local.PutBlock(ctx, fmt.Appendf(nil, "block-%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if _, _, err := remote.PutBlock(ctx, []byte("block-0"), nil); err != nil {
		t.Fatal(err)
	}

	// List one ref the local store no longer holds.
	gone, _, err := newInmemBlockStore().PutBlock(ctx, []byte("gone"), nil)
	if err != nil {
		t.Fatal(err)
	}
	list := listRefs(append(slices.Clone(refs), gone))

	// A list failing after the first step stops the copy past that step.
	var calls int
	failing := func(ctx context.Context, after []byte, limit int) ([]*block.BlockRef, []byte, error) {
		if calls++; calls > 1 {
			return nil, nil, errors.New("list failed")
		}
		return list(ctx, after, limit)
	}
	if err := s.Copy(ctx, "db", remote, failing, CopyOptions{}); err == nil {
		t.Fatal("expected copy failure")
	}
	if status, _ := s.GetStatus(); status.Copy.Running || status.Copy.Done || status.Copy.Scanned != copyBatchBlocks {
		t.Fatalf("after failure: %+v", status.Copy)
	}

	// Reopen the store.
	s, err = NewStore(ctx, local, local, markers, true)
	if err != nil {
		t.Fatal(err)
	}

	// The reopened store resumes after the first step and finishes.
	var resumed []byte
	resume := func(ctx context.Context, after []byte, limit int) ([]*block.BlockRef, []byte, error) {
		if resumed == nil {
			resumed = append([]byte{}, after...)
		}
		return list(ctx, after, limit)
	}
	if err := s.Copy(ctx, "db", remote, resume, CopyOptions{MaxBytesPerSecond: 1 << 30}); err != nil {
		t.Fatal(err)
	}
	if len(resumed) == 0 {
		t.Fatal("copy restarted from the first key")
	}
	status, _ := s.GetStatus()
	if c := status.Copy; !c.Done || c.Position != 1 || c.Scanned != copyBatchBlocks+11 {
		t.Fatalf("after resume: %+v", c)
	}
	for _, ref := range refs {
		found, err := remote.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
	}

	// A completed copy lists nothing again.
	calls = 0
	if err := s.Copy(ctx, "db", remote, failing, CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("listed %d times after completion", calls)
	}
}

// TestCopySkipsUnknownRefs checks that a block held without its refs stays
// local and counts as unknown, since the remote would keep it as a leaf, and
// that a listed block the store no longer holds counts as gone.
func TestCopySkipsUnknownRefs(t *testing.T) {
	// Store a block without its refs.
	ctx := t.Context()
	local := newUntrackedBlockStore()
	s, err := NewStore(ctx, local, local, store_kvtx_inmem.NewStore(), true)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := local.PutBlock(ctx, []byte("untracked"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// The copy finishes without writing either.
	gone, _, err := newInmemBlockStore().PutBlock(ctx, []byte("gone"), nil)
	if err != nil {
		t.Fatal(err)
	}
	remote := newInmemBlockStore()
	if err := s.Copy(ctx, "db", remote, listRefs([]*block.BlockRef{ref, gone}), CopyOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Copy.RefsUnknown != 1 || status.Copy.Gone != 1 || status.Copy.Copied != 0 {
		t.Fatalf("status = %+v", status.Copy)
	}
	found, err := remote.GetBlockExists(ctx, ref)
	if err != nil || found {
		t.Fatalf("remote = %v/%v, want absent", found, err)
	}
}
