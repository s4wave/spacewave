package boot

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// graphSource is an image store of its own that serves revisions to a
// staging. After limit reads it fails, as a source cut off mid-copy does; a
// negative limit never fails. Its revisions share one transform, as a Space's
// do.
type graphSource struct {
	*volume_s4db.Volume
	transform *block_transform.Config
	limit     int64
	reads     atomic.Int64
}

// newGraphSource opens an empty source store, closed when the test ends.
func newGraphSource(t *testing.T, le *logrus.Entry) *graphSource {
	vol, err := volume_s4db.NewVolume(t.Context(), le, &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	transform, err := os_image.NewTransform()
	if err != nil {
		t.Fatal(err)
	}
	return &graphSource{Volume: vol, transform: transform, limit: -1}
}

// GetStoredBlock serves a stored block until the source's read limit.
func (s *graphSource) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if n := s.reads.Add(1); s.limit >= 0 && n > s.limit {
		return nil, errors.New("source cut off")
	}
	return s.Volume.GetStoredBlock(ctx, ref)
}

// add stores a block with children in the source and returns its ref.
func (s *graphSource) add(t *testing.T, data string, children ...*block.BlockRef) *block.BlockRef {
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := &block.PutBatchEntry{Ref: ref, Data: []byte(data), Refs: children, RefsKnown: true}
	if _, err := s.PutBlockBatch(t.Context(), []*block.PutBatchEntry{entry}); err != nil {
		t.Fatal(err)
	}
	return ref
}

// revisionGraph adds a revision named name to src: a root over fanout
// subtrees of depth leaves each, sharing the leaf "shared" with every other
// revision. It returns the revision's image and the number of blocks only it
// holds.
func revisionGraph(t *testing.T, src *graphSource, name string, fanout int) (*bucket.ObjectRef, int) {
	shared := src.add(t, "shared leaf")
	var children []*block.BlockRef
	for i := range fanout {
		leaf := src.add(t, name+" leaf "+strconv.Itoa(i))
		children = append(children, src.add(t, name+" dir "+strconv.Itoa(i), leaf, shared))
	}
	root := src.add(t, name+" root", children...)
	return &bucket.ObjectRef{RootRef: root, TransformConf: src.transform}, 2*fanout + 1
}

// signImage signs image with key.
func signImage(t *testing.T, key crypto.PrivKey, image *bucket.ObjectRef) *peer.SignedMsg {
	signed, err := os_image.SignImage(key, image)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// seedRevision copies image from src into vol and names it the built image,
// as a seed leaves it.
func seedRevision(t *testing.T, vol *volume_s4db.Volume, src *graphSource, image *bucket.ObjectRef) {
	if _, err := copyRevision(t.Context(), vol, src, image.GetRootRef()); err != nil {
		t.Fatal(err)
	}
	built, err := imageBlock(image)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishBuiltImage(t.Context(), vol, built); err != nil {
		t.Fatal(err)
	}
}

// newSigner returns a fresh signing key and its peer ID.
func newSigner(t *testing.T) (crypto.PrivKey, peer.ID) {
	p, key, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	return key, p.GetPeerID()
}

// hashStore returns a digest of the image store's file.
func hashStore(t *testing.T, dir string) [32]byte {
	data, err := os.ReadFile(filepath.Join(dir, "image.s4wave"))
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

// TestRevisionLifecycle walks the image store through OSRevision.tla's moves:
// install, an ordinary boot that writes nothing, a staged trial tried once and
// rolled back, a resumed staging accepted on health, and a trial signed by an
// unpinned key that rolls back without being served.
func TestRevisionLifecycle(t *testing.T) {
	// Build an image store holding revision A as its built image.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()
	src := newGraphSource(t, le)
	imageA, _ := revisionGraph(t, src, "A", 2)
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(dir, "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	seedRevision(t, vol, src, imageA)
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Install A with the owner's authorization; a second install is refused.
	key, owner := newSigner(t)
	pinned := []peer.ID{owner}
	if err := AuthorizeImage(ctx, le, dir, signImage(t, key, imageA), imageA); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeImage(ctx, le, dir, signImage(t, key, imageA), imageA); !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("second install returned %v", err)
	}

	// An ordinary boot serves A and changes no byte of the store.
	before := hashStore(t, dir)
	sel, err := SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if sel.OnTrial() || !sel.Image.EqualVT(imageA) {
		t.Fatal("ordinary boot did not serve the accepted revision")
	}
	if hashStore(t, dir) != before {
		t.Fatal("ordinary boot wrote the store")
	}

	// Stage C, a revision that never reaches health; only its own blocks copy.
	imageC, ownC := revisionGraph(t, src, "C", 3)
	copied, err := StageRevision(ctx, le, dir, src, signImage(t, key, imageC), imageC)
	if err != nil || copied != ownC {
		t.Fatalf("staging copied %d blocks, want %d: %v", copied, ownC, err)
	}

	// The first boot tries C; one that never accepts it boots back to A.
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !sel.OnTrial() || !sel.Image.EqualVT(imageC) {
		t.Fatal("first boot after staging did not try the trial")
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if sel.OnTrial() || !sel.Image.EqualVT(imageA) {
		t.Fatal("second boot did not roll back to the accepted revision")
	}
	revs, err := readRevisions(ctx, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if revs.trial.auth != nil || revs.attempted != nil || revs.rolledBack == nil {
		t.Fatal("rollback left a trial pending or recorded nothing")
	}

	// Stage B one read at a time, cut off after four reads, which complete
	// one subtree: the cut keeps it, and the retry copies the rest.
	stageReads = 1
	t.Cleanup(func() { stageReads = block.DefaultGraphCopyReads })
	imageB, ownB := revisionGraph(t, src, "B", 4)
	src.limit = 4
	src.reads.Store(0)
	cut, err := StageRevision(ctx, le, dir, src, signImage(t, key, imageB), imageB)
	if err == nil || cut == 0 {
		t.Fatalf("cut staging copied %d blocks: %v", cut, err)
	}
	src.limit = -1
	rest, err := StageRevision(ctx, le, dir, src, signImage(t, key, imageB), imageB)
	if err != nil || cut+rest != ownB {
		t.Fatalf("resumed staging copied %d+%d blocks, want %d: %v", cut, rest, ownB, err)
	}

	// A second staging while B is pending copies nothing and is refused.
	again, err := StageRevision(ctx, le, dir, src, signImage(t, key, imageB), imageB)
	if again != 0 || !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("staging over a pending trial copied %d blocks: %v", again, err)
	}

	// B is tried and accepted through the console's child arguments; a
	// selection read before the accept is stale.
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || !sel.OnTrial() || !sel.Image.EqualVT(imageB) {
		t.Fatalf("boot did not try B: %v", err)
	}
	if err := RunAcceptTrial(sel.AcceptArgs(dir)[1:]); err != nil {
		t.Fatal(err)
	}
	if err := AcceptTrial(ctx, le, dir, sel); !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("stale accept returned %v", err)
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || sel.OnTrial() || !sel.Image.EqualVT(imageB) {
		t.Fatalf("boot after accept did not serve B: %v", err)
	}

	// D, signed by a key the Device never pinned, rolls back unserved.
	otherKey, _ := newSigner(t)
	imageD, _ := revisionGraph(t, src, "D", 1)
	if _, err := StageRevision(ctx, le, dir, src, signImage(t, otherKey, imageD), imageD); err != nil {
		t.Fatal(err)
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || sel.OnTrial() || !sel.Image.EqualVT(imageB) {
		t.Fatalf("unpinned trial was not rolled back to B: %v", err)
	}

	// Stage E from another image store that shares A's blocks: only E's own
	// blocks copy, and a reader of that store sees no change.
	srcDir := t.TempDir()
	imageE, ownE := revisionGraph(t, src, "E", 2)
	vol, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(srcDir, "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyRevision(ctx, vol, src, imageA.GetRootRef()); err != nil {
		t.Fatal(err)
	}
	seedRevision(t, vol, src, imageE)
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	image, built, err := OpenImageStore(ctx, le, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	copied, err = StageRevision(ctx, le, dir, image, signImage(t, key, imageE), built)
	if err := errors.Join(err, image.Close()); err != nil || copied != ownE || !built.EqualVT(imageE) {
		t.Fatalf("staging from an image store copied %d blocks, want %d: %v", copied, ownE, err)
	}

	// An authorization of another root is refused before copying.
	if _, err := StageRevision(ctx, le, dir, src, signImage(t, key, imageA), imageD); err == nil || !strings.Contains(err.Error(), "does not name root") {
		t.Fatalf("mismatched authorization returned %v", err)
	}
}
