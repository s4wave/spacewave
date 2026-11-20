package boot

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// graphSource is a RevisionSource over blocks held in memory. After limit
// reads it fails, as a source cut off mid-copy does; a negative limit never
// fails.
type graphSource struct {
	blocks map[string]graphBlock
	limit  int
	reads  int
}

// graphBlock is one stored block with its child list.
type graphBlock struct {
	data     []byte
	children []*block.BlockRef
}

// GetBlock returns a held block until the source's read limit.
func (s *graphSource) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, []*block.BlockRef, error) {
	if s.limit >= 0 && s.reads >= s.limit {
		return nil, nil, errors.New("source cut off")
	}
	s.reads++
	b, ok := s.blocks[ref.MarshalString()]
	if !ok {
		return nil, nil, errors.New("source lacks " + ref.MarshalString())
	}
	return b.data, b.children, nil
}

// add stores a block with children in the source and returns its ref.
func (s *graphSource) add(t *testing.T, data string, children ...*block.BlockRef) *block.BlockRef {
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.blocks[ref.MarshalString()] = graphBlock{data: []byte(data), children: children}
	return ref
}

// revisionGraph adds a revision named name to src: a root over fanout
// subtrees of depth leaves each, sharing the leaf "shared" with every other
// revision. It returns the root and the number of blocks only it holds.
func revisionGraph(t *testing.T, src *graphSource, name string, fanout int) (*block.BlockRef, int) {
	shared := src.add(t, "shared leaf")
	var children []*block.BlockRef
	for i := range fanout {
		leaf := src.add(t, name+" leaf "+strconv.Itoa(i))
		children = append(children, src.add(t, name+" dir "+strconv.Itoa(i), leaf, shared))
	}
	return src.add(t, name+" root", children...), 2*fanout + 1
}

// signRoot signs root with key.
func signRoot(t *testing.T, key crypto.PrivKey, root *block.BlockRef) *peer.SignedMsg {
	signed, err := os_image.SignImageRoot(key, root)
	if err != nil {
		t.Fatal(err)
	}
	return signed
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
	src := &graphSource{blocks: make(map[string]graphBlock), limit: -1}
	rootA, _ := revisionGraph(t, src, "A", 2)
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(dir, "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copyRevision(ctx, vol, src, rootA); err != nil {
		t.Fatal(err)
	}
	err = vol.SetBucketRoots(ctx, imageBucket, nil, []block.NamedRoot{{Name: builtImageRoot, Ref: rootA}})
	if err := errors.Join(err, vol.Close()); err != nil {
		t.Fatal(err)
	}

	// Install A with the owner's authorization; a second install is refused.
	key, owner := newSigner(t)
	pinned := []peer.ID{owner}
	if err := AuthorizeImage(ctx, le, dir, signRoot(t, key, rootA), rootA); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeImage(ctx, le, dir, signRoot(t, key, rootA), rootA); !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("second install returned %v", err)
	}

	// An ordinary boot serves A and changes no byte of the store.
	before := hashStore(t, dir)
	sel, err := SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if sel.OnTrial() || !sel.Root.EqualsRef(rootA) {
		t.Fatal("ordinary boot did not serve the accepted revision")
	}
	if hashStore(t, dir) != before {
		t.Fatal("ordinary boot wrote the store")
	}

	// Stage C, a revision that never reaches health; only its own blocks copy.
	rootC, ownC := revisionGraph(t, src, "C", 3)
	copied, err := StageRevision(ctx, le, dir, src, signRoot(t, key, rootC), rootC)
	if err != nil || copied != ownC {
		t.Fatalf("staging copied %d blocks, want %d: %v", copied, ownC, err)
	}

	// The first boot tries C; one that never accepts it boots back to A.
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !sel.OnTrial() || !sel.Root.EqualsRef(rootC) {
		t.Fatal("first boot after staging did not try the trial")
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if sel.OnTrial() || !sel.Root.EqualsRef(rootA) {
		t.Fatal("second boot did not roll back to the accepted revision")
	}
	revs, err := readRevisions(ctx, dir, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if revs.trial.auth != nil || revs.attempted != nil || revs.rolledBack == nil {
		t.Fatal("rollback left a trial pending or recorded nothing")
	}

	// Stage B in one-block batches, cut off midway; the retry copies the rest.
	revisionBatchBytes = 1
	t.Cleanup(func() { revisionBatchBytes = 4 << 20 })
	rootB, ownB := revisionGraph(t, src, "B", 4)
	src.limit, src.reads = 4, 0
	cut, err := StageRevision(ctx, le, dir, src, signRoot(t, key, rootB), rootB)
	if err == nil || cut == 0 {
		t.Fatalf("cut staging copied %d blocks: %v", cut, err)
	}
	src.limit = -1
	rest, err := StageRevision(ctx, le, dir, src, signRoot(t, key, rootB), rootB)
	if err != nil || cut+rest != ownB {
		t.Fatalf("resumed staging copied %d+%d blocks, want %d: %v", cut, rest, ownB, err)
	}

	// A second staging while B is pending copies nothing and is refused.
	again, err := StageRevision(ctx, le, dir, src, signRoot(t, key, rootB), rootB)
	if again != 0 || !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("staging over a pending trial copied %d blocks: %v", again, err)
	}

	// B is tried and accepted through the console's child arguments; a
	// selection read before the accept is stale.
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || !sel.OnTrial() || !sel.Root.EqualsRef(rootB) {
		t.Fatalf("boot did not try B: %v", err)
	}
	if err := RunAcceptTrial(sel.AcceptArgs(dir)[1:]); err != nil {
		t.Fatal(err)
	}
	if err := AcceptTrial(ctx, le, dir, sel); !errors.Is(err, block.ErrRootMoved) {
		t.Fatalf("stale accept returned %v", err)
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || sel.OnTrial() || !sel.Root.EqualsRef(rootB) {
		t.Fatalf("boot after accept did not serve B: %v", err)
	}

	// D, signed by a key the Device never pinned, rolls back unserved.
	otherKey, _ := newSigner(t)
	rootD, _ := revisionGraph(t, src, "D", 1)
	if _, err := StageRevision(ctx, le, dir, src, signRoot(t, otherKey, rootD), rootD); err != nil {
		t.Fatal(err)
	}
	sel, err = SelectRevision(ctx, le, dir, pinned)
	if err != nil || sel.OnTrial() || !sel.Root.EqualsRef(rootB) {
		t.Fatalf("unpinned trial was not rolled back to B: %v", err)
	}

	// Stage E from another image store that shares A's blocks: only E's own
	// blocks copy, and a reader of that store sees no change.
	srcDir := t.TempDir()
	rootE, ownE := revisionGraph(t, src, "E", 2)
	vol, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(srcDir, "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []*block.BlockRef{rootA, rootE} {
		if _, err := copyRevision(ctx, vol, src, root); err != nil {
			t.Fatal(err)
		}
	}
	err = vol.SetBucketRoots(ctx, imageBucket, nil, []block.NamedRoot{{Name: builtImageRoot, Ref: rootE}})
	if err := errors.Join(err, vol.Close()); err != nil {
		t.Fatal(err)
	}
	image, built, err := OpenImageSource(ctx, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	copied, err = StageRevision(ctx, le, dir, image, signRoot(t, key, rootE), built)
	if err := errors.Join(err, image.Close()); err != nil || copied != ownE || !built.EqualsRef(rootE) {
		t.Fatalf("staging from an image store copied %d blocks, want %d: %v", copied, ownE, err)
	}

	// An authorization of another root is refused before copying.
	if _, err := StageRevision(ctx, le, dir, src, signRoot(t, key, rootA), rootD); err == nil || !strings.Contains(err.Error(), "does not name root") {
		t.Fatalf("mismatched authorization returned %v", err)
	}
}
