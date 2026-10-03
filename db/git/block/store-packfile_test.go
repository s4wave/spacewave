package git_block

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	billy "github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	go_git_packfile "github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/memory"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

func TestStoragePackfileWriter(t *testing.T) {
	// Open a block-backed Store for the packfile write.
	ctx, oc, store := newPackfileTestStore(t)
	defer store.Close()

	// Write a pack containing the test blob into the Store.
	packData, blobHash := buildTestPackfile(t, []byte("packed data"))
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the packed blob can be read before committing.
	obj, err := store.EncodedObject(plumbing.BlobObject, blobHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertObjectData(t, obj, []byte("packed data"))

	// Commit the packfile and its metadata to block storage.
	if err := store.Commit(); err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the Store from its committed repository root.
	storeRef := store.GetRef()
	oc.SetRootRef(storeRef)
	btx, bcs := oc.BuildTransaction(nil)
	store, err = NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer store.Close()

	// Verify the reopened Store still reads the packed blob.
	obj, err = store.EncodedObject(plumbing.BlobObject, blobHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertObjectData(t, obj, []byte("packed data"))

	// Verify the reopened Store lists the persisted pack.
	packs, err := store.ObjectPacks()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(packs) != 1 {
		t.Fatalf("expected one pack, got %d", len(packs))
	}
}

// TestStoragePackedLookupOnlyOpensMatchingPack keeps index probes independent
// of pack data readers and checks the shared object cache and reader bound.
func TestStoragePackedLookupOnlyOpensMatchingPack(t *testing.T) {
	// Open a block-backed Store for pack reader cache checks.
	_, _, store := newPackfileTestStore(t)
	defer store.Close()

	// Write more packs than the Store can retain open readers.
	var hashes []plumbing.Hash
	for i := range openPackReaderLimit + 2 {
		// Write a distinct blob pack and retain its hash for lookup.
		data := []byte{byte(i), 'p', 'a', 'c', 'k'}
		packData, hash := buildTestPackfile(t, data)
		writer, err := store.PackfileWriter()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(packData); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
	}

	// Look up one packed blob and probe a missing object hash.
	first, err := store.EncodedObject(plumbing.BlobObject, hashes[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EncodedObject(plumbing.BlobObject, plumbing.NewHash("ffffffffffffffffffffffffffffffffffffffff")); err != plumbing.ErrObjectNotFound {
		t.Fatalf("missing object lookup returned %v", err)
	}
	if len(store.packCache) != len(hashes) {
		t.Fatalf("decoded %d indexes, want %d", len(store.packCache), len(hashes))
	}

	// Verify membership probes decoded indexes and opened only one pack reader.
	var open int
	for _, entry := range store.packCache {
		if entry.idx == nil {
			t.Fatal("missing decoded index")
		}
		if entry.pack != nil {
			open++
			if entry.pack.Index != entry.idx {
				t.Fatal("pack uses another index")
			}
		}
	}
	if open != 1 {
		t.Fatalf("opened %d pack data readers for one lookup, want 1", open)
	}

	// Read the remaining packed blobs to fill the shared object cache.
	for _, hash := range hashes[1:] {
		if _, err := store.EncodedObject(plumbing.BlobObject, hash); err != nil {
			t.Fatal(err)
		}
	}

	// Verify all blobs share the bounded pack reader cache.
	for _, hash := range hashes {
		if _, ok := store.objectCache.Get(hash); !ok {
			t.Fatalf("shared object cache is missing %s", hash)
		}
	}
	if store.packLRU.Len() != openPackReaderLimit {
		t.Fatalf("open pack readers = %d, want %d", store.packLRU.Len(), openPackReaderLimit)
	}
	assertObjectData(t, first, []byte{0, 'p', 'a', 'c', 'k'})
	for _, entry := range store.packCache {
		if entry.pack == nil {
			continue
		}
		if entry.pack.Index != entry.idx {
			t.Fatal("pack uses another index")
		}
	}
}

func TestStoragePackfileWriterReadsCommitTreeAndBlob(t *testing.T) {
	// Open a block-backed Store for commit packfile reads.
	_, _, store := newPackfileTestStore(t)
	defer store.Close()

	// Write a pack containing a commit and its tree and blob.
	packData, commitHash, objectCount := buildTestCommitPackfile(t)
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Read the packed commit and verify its file contents.
	commit, err := object.GetCommit(store, commitHash)
	if err != nil {
		t.Fatal(err.Error())
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err.Error())
	}
	file, err := tree.File("hello.txt")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the packed file contents through the Git object API.
	content, err := file.Contents()
	if err != nil {
		t.Fatal(err.Error())
	}
	if content != "hello from pack" {
		t.Fatalf("unexpected file content: %q", content)
	}

	// Count every packed object through the Store iterator.
	iter, err := store.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer iter.Close()
	var gotCount int
	if err := iter.ForEach(func(plumbing.EncodedObject) error {
		gotCount++
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
	if gotCount != objectCount {
		t.Fatalf("iter count mismatch: got %d want %d", gotCount, objectCount)
	}
}

func TestStoragePackDeleteInvalidatesCachedReader(t *testing.T) {
	// Open a block-backed Store for packfile deletion.
	_, _, store := newPackfileTestStore(t)
	defer store.Close()

	// Write a pack containing the test blob into the Store.
	packData, blobHash := buildTestPackfile(t, []byte("packed data"))
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the pack is listed and open its cached data reader.
	packs, err := store.ObjectPacks()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(packs) != 1 {
		t.Fatalf("expected one pack, got %d", len(packs))
	}
	if _, err := store.EncodedObject(plumbing.BlobObject, blobHash); err != nil {
		t.Fatal(err.Error())
	}

	// Delete the packfile and its cached reader.
	if err := store.DeleteOldObjectPackAndIndex(packs[0], time.Time{}); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the deleted pack and its blob are unavailable.
	if _, err := store.EncodedObject(plumbing.BlobObject, blobHash); err != plumbing.ErrObjectNotFound {
		t.Fatalf("expected deleted pack object to be unavailable, got %v", err)
	}
	packs, err = store.ObjectPacks()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(packs) != 0 {
		t.Fatalf("expected pack deletion, got %d packs", len(packs))
	}
}

func TestStoragePackIterationDedupesLooseObject(t *testing.T) {
	// Open a block-backed Store for loose and packed object deduplication.
	_, _, store := newPackfileTestStore(t)
	defer store.Close()

	// Write the test blob in a packfile.
	data := []byte("dedupe data")
	packData, blobHash := buildTestPackfile(t, data)
	wr, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(packData); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Write the same blob through the loose object writer.
	obj := store.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(data)))
	objWriter, err := obj.Writer()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := objWriter.Write(data); err != nil {
		t.Fatal(err.Error())
	}
	if err := objWriter.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the loose object has the packed blob hash.
	got, err := store.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got != blobHash {
		t.Fatalf("loose object hash = %s, want %s", got, blobHash)
	}

	// Count the blob through the combined loose and packed object iterator.
	iter, err := store.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer iter.Close()
	var seen int
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		if obj.Hash() == blobHash {
			seen++
		}
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
	if seen != 1 {
		t.Fatalf("expected one object for %s, got %d", blobHash, seen)
	}
}

func newPackfileTestStore(t *testing.T) (context.Context, *bucket_lookup.Cursor, *Store) {
	// Prepare the context and logger for the packfile testbed.
	t.Helper()
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the packfile testbed with its in-memory volume.
	testbed.Verbose = false
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty repository cursor in the testbed.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize the repository block and open its Store.
	btx, bcs := oc.BuildTransaction(nil)
	root := NewRepo()
	bcs.SetBlock(root, true)
	store, err := NewStore(ctx, btx, bcs, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ctx, oc, store
}

func buildTestPackfile(t *testing.T, data []byte) ([]byte, plumbing.Hash) {
	// Write the test blob into an in-memory Git object store.
	t.Helper()
	mem := memory.NewStorage()
	obj := mem.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	wr, err := obj.Writer()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wr.Write(data); err != nil {
		t.Fatal(err.Error())
	}
	if err := wr.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Save the in-memory blob and obtain its Git hash.
	blobHash, err := mem.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Encode the in-memory blob into a Git packfile.
	var buf bytes.Buffer
	enc := go_git_packfile.NewEncoder(&buf, mem, false)
	if _, err := enc.Encode([]plumbing.Hash{blobHash}, 0); err != nil {
		t.Fatal(err.Error())
	}
	return buf.Bytes(), blobHash
}

func buildTestCommitPackfile(t *testing.T) ([]byte, plumbing.Hash, int) {
	// Initialize an in-memory Git repository with a working tree.
	t.Helper()
	mem := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(mem, git.WithWorkTree(fs))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit the test file into the in-memory Git repository.
	writeBillyFile(t, fs, "hello.txt", []byte("hello from pack"))
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wt.Add("hello.txt"); err != nil {
		t.Fatal(err.Error())
	}

	// Create the test commit with a fixed author and timestamp.
	commitHash, err := wt.Commit("add hello", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Hydra Test",
			Email: "hydra@example.test",
			When:  time.Unix(1, 0),
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the in-memory repository object iterator.
	iter, err := mem.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer iter.Close()

	// Collect the Git object hashes for packfile encoding.
	hashes := make([]plumbing.Hash, 0)
	if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
		hashes = append(hashes, obj.Hash())
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Encode the commit, tree, and blob into a Git packfile.
	var buf bytes.Buffer
	enc := go_git_packfile.NewEncoder(&buf, mem, false)
	if _, err := enc.Encode(hashes, 0); err != nil {
		t.Fatal(err.Error())
	}
	return buf.Bytes(), commitHash, len(hashes)
}

func writeBillyFile(t *testing.T, fs billy.Filesystem, name string, data []byte) {
	// Create and write the test file in the Git working tree.
	t.Helper()
	f, err := fs.Create(name)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err.Error())
	}
}

func assertObjectData(t *testing.T, obj plumbing.EncodedObject, expected []byte) {
	// Open the Git object data reader for verification.
	t.Helper()
	rc, err := obj.Reader()
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rc.Close()

	// Verify the Git object bytes match the expected contents.
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(data, expected) {
		t.Fatalf("object data mismatch: got %q want %q", data, expected)
	}
}
