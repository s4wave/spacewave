package cdn_mirror_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_mirror "github.com/s4wave/spacewave/core/cdn/mirror"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// fsKey is the World object holding the mirrored tree.
const fsKey = "media"

// maxPack is the pack bound the tests set, small enough to cut the tree into
// several packs.
const maxPack = 96 << 10

// tree is a local World holding a UnixFS tree.
type tree struct {
	// world is the open World.
	world *publisher.LocalWorld
	// files maps each file's path to its content.
	files fstest.MapFS
}

// newTree writes n files of random content in a new local World.
func newTree(t *testing.T, n int) *tree {
	w, err := publisher.OpenLocalWorld(t.Context(), logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "source.s4wave"), "mirror-source", "mirror-source")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	files := randomFiles(t, n, "dir")
	writeObject(t, w, fsKey, files)
	return &tree{world: w, files: files}
}

// randomFiles makes n files of random content under directories named with
// prefix.
func randomFiles(t *testing.T, n int, prefix string) fstest.MapFS {
	// Make the directories writable, so a checkout can fill them.
	files := fstest.MapFS{}
	for i := range min(n, 3) {
		files[fmt.Sprintf("%s%d", prefix, i)] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
	}

	// Fill them with files of distinct sizes.
	for i := range n {
		data := make([]byte, 24<<10+i*512)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		files[fmt.Sprintf("%s%d/file%d.bin", prefix, i%3, i)] = &fstest.MapFile{Data: data, Mode: 0o644}
	}
	return files
}

// writeObject writes files as one UnixFS object at key in w.
func writeObject(t *testing.T, w *publisher.LocalWorld, key string, files fstest.MapFS) {
	ctx := t.Context()
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return unixfs_block.CreateFromFS(ctx, bcs, files, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(ctx, key, ref)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// head returns the World's head and the number of blocks under it.
func (tr *tree) head(t *testing.T) (*bucket.ObjectRef, int) {
	var head *bucket.ObjectRef
	sink := &countSink{}
	err := tr.world.Engine.AccessWorldState(t.Context(), nil, func(cursor *bucket_lookup.Cursor) error {
		head = cursor.GetRefWithOpArgs()
		return block.CopyGraph(t.Context(), cursor.GetBlockStore(), sink, head.GetRootRef(), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	return head, sink.n
}

// countSink counts the blocks a graph copy writes and keeps their refs.
type countSink struct {
	block.StoreOps
	// n counts the blocks.
	n int
	// entries are the written blocks.
	entries []*block.PutBatchEntry
}

// PutBlockBatch counts and keeps the blocks.
func (s *countSink) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	s.n += len(entries)
	s.entries = append(s.entries, entries...)
	return make([]bool, len(entries)), nil
}

// requireClosed requires that every block of tr r's Space lists has each of
// its children listed too, so a resume may skip it with its subtree.
func requireClosed(t *testing.T, r *writertest.Rig, tr *tree) {
	// List every block of the tree with its children.
	sink := &countSink{}
	err := tr.world.Engine.AccessWorldState(t.Context(), nil, func(cursor *bucket_lookup.Cursor) error {
		return block.CopyGraph(t.Context(), cursor.GetBlockStore(), sink, cursor.GetRefWithOpArgs().GetRootRef(), nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Ask the Space which it lists.
	refs := make([]*block.BlockRef, len(sink.entries))
	index := make(map[string]int, len(sink.entries))
	for i, e := range sink.entries {
		refs[i] = e.Ref
		index[e.Ref.MarshalString()] = i
	}
	exists := spaceHolds(t, r, r.Writer.Root(writertest.Space).GetPacks(), refs)

	// Require each listed block's children listed.
	for i, e := range sink.entries {
		if !exists[i] {
			continue
		}
		for _, child := range e.Refs {
			if j, ok := index[child.MarshalString()]; ok && !exists[j] {
				t.Fatalf("the Space lists %s without its child %s", e.Ref.MarshalString(), child.MarshalString())
			}
		}
	}
}

// options returns mirror options writing r's Space.
func options(r *writertest.Rig, uploads int) cdn_mirror.Options {
	return cdn_mirror.Options{
		Publish: cdn_publish.Options{
			Client:      r.Client(r.Owner),
			DstSpaceID:  writertest.Space,
			OwnerKeyPem: r.OwnerPem,
			CdnBaseURL:  r.PackBase(),
		},
		Uploads:      uploads,
		MaxPackBytes: maxPack,
	}
}

// readTree mounts r's Space as a reader does and requires every file of tr.
func readTree(t *testing.T, r *writertest.Rig, tr *tree) {
	// Mount the Space's World through the pointer handler.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conf := cdn_world_controller.NewConfig("mirror", writertest.Space, r.PackBase())
	conf.RootPointerBaseUrl = r.RootsURL
	ctrl := cdn_world_controller.NewController(logrus.NewEntry(logrus.New()), nil, conf)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read each file through a UnixFS handle on the mirrored object.
	ws := world.NewEngineWorldState(eng, false)
	fsc := unixfs_world.NewFSCursor(logrus.NewEntry(logrus.New()), ws, fsKey, unixfs_world.FSType_FSType_FS_NODE, nil, false)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		fsc.Release()
		t.Fatal(err)
	}
	defer fsh.Release()
	for name, file := range tr.files {
		if file.Mode.IsDir() {
			continue
		}
		child, _, err := fsh.LookupPath(ctx, name)
		if err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}
		got, err := unixfs.ReadFile(ctx, child)
		child.Release()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, file.Data) {
			t.Fatalf("%s differs from its source", name)
		}
	}
}

// spaceHolds reports which of refs the packs hold, read as a reader of r's
// Space reads them.
func spaceHolds(t *testing.T, r *writertest.Rig, packs []*packfile.PackfileEntry, refs []*block.BlockRef) []bool {
	store := packfile_store.NewPackfileStore(cdn_bstore.NewAnonymousOpener(nil, r.PackBase(), writertest.Space), nil)
	defer store.Close()
	store.UpdateManifest(packs)
	exists, err := store.GetBlockExistsBatch(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

// TestMirrorTwice mirrors a tree into bounded packs with its root in the last
// one, reads it back as a reader, and mirrors it again uploading nothing.
func TestMirrorTwice(t *testing.T) {
	// Mirror the tree.
	ctx := t.Context()
	r := writertest.NewRig(t)
	tr := newTree(t, 24)
	head, total := tr.head(t)
	res, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, 4))
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocks != int64(total) || res.Checkpoint == nil {
		t.Fatalf("mirrored %d of %d blocks, checkpoint %v", res.Blocks, total, res.Checkpoint)
	}

	// Every pack is within the bound, which by default is within 256 MiB.
	packs := r.Writer.Root(writertest.Space).GetPacks()
	if len(packs) < 3 || res.Packs != len(packs) {
		t.Fatalf("committed %d packs, listed %d", res.Packs, len(packs))
	}
	for _, pack := range packs {
		if pack.GetSizeBytes() > maxPack {
			t.Fatalf("pack %s holds %d bytes", pack.GetId(), pack.GetSizeBytes())
		}
	}
	if delta.DefaultMaxChunkBytes > 256<<20 {
		t.Fatalf("the default pack bound is %d bytes", delta.DefaultMaxChunkBytes)
	}

	// The root lands in the last pack and in no earlier one.
	root := []*block.BlockRef{head.GetRootRef()}
	last := len(packs) - 1
	if !spaceHolds(t, r, packs[last:], root)[0] {
		t.Fatal("the last pack lacks the root")
	}
	if spaceHolds(t, r, packs[:last], root)[0] {
		t.Fatal("an earlier pack holds the root")
	}

	// A reader mounting the Space reads every file.
	readTree(t, r, tr)

	// Mirroring again uploads and posts nothing.
	var puts atomic.Int64
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		puts.Add(1)
		return req, nil
	})
	before := r.Pointer()
	again, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, 4))
	if err != nil {
		t.Fatal(err)
	}
	if again.Blocks != 0 || again.Packs != 0 || again.Checkpoint != nil || puts.Load() != 0 {
		t.Fatalf("mirroring again uploaded %d blocks in %d packs with %d PUTs", again.Blocks, again.Packs, puts.Load())
	}
	if r.Pointer().ETag != before.ETag {
		t.Fatal("mirroring again changed the pointer")
	}
}

// TestMirrorResumes cuts a mirror after its first packs commit, then resumes
// it: the resumed copy uploads only what the cut one did not commit, and a
// reader reads the whole tree.
func TestMirrorResumes(t *testing.T) {
	// Hold the first upload after the first window, long enough for every
	// later upload to land and, if commits ran out of order, to commit, then
	// fail it.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	r := writertest.NewRig(t)
	tr := newTree(t, 24)
	_, total := tr.head(t)
	const uploads = 2
	var puts atomic.Int64
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		if puts.Add(1) != uploads+1 {
			return req, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return nil, errors.New("cut")
		}
	})

	// The cut mirror commits some packs, each with its subtree, and leaves
	// the checkpoint.
	genesis := r.Writer.Root(writertest.Space).GetCheckpoint().CloneVT()
	if _, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, uploads)); err == nil {
		t.Fatal("the cut mirror succeeded")
	}
	landed := len(r.Writer.Root(writertest.Space).GetPacks())
	if landed == 0 {
		t.Fatal("the cut mirror committed no pack")
	}
	if !r.Writer.Root(writertest.Space).GetCheckpoint().EqualVT(genesis) {
		t.Fatal("the cut mirror moved the checkpoint")
	}
	requireClosed(t, r, tr)

	// The resumed mirror uploads only the rest, and a reader reads the tree.
	r.Transport.SetPut(nil)
	res, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, uploads))
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocks == 0 || res.Blocks >= int64(total) || res.Checkpoint == nil {
		t.Fatalf("the resumed mirror uploaded %d of %d blocks", res.Blocks, total)
	}
	readTree(t, r, tr)
}

// TestMirrorUploadsConcurrently holds each upload until a window of four is
// in flight at once.
func TestMirrorUploadsConcurrently(t *testing.T) {
	// Hold the first four uploads until all four arrive.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	r := writertest.NewRig(t)
	tr := newTree(t, 24)
	const uploads = 4
	var arrived atomic.Int64
	all := make(chan struct{})
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		n := arrived.Add(1)
		if n == uploads {
			close(all)
		}
		if n > uploads {
			return req, nil
		}
		select {
		case <-all:
			return req, nil
		case <-ctx.Done():
			return nil, errors.New("the uploads did not overlap")
		}
	})

	// The mirror completes only if four uploads were in flight together.
	if _, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, uploads)); err != nil {
		t.Fatal(err)
	}
	readTree(t, r, tr)
}
