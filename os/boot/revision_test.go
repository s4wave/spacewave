package boot

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
	os_release "github.com/s4wave/spacewave/os/release"
	os_system "github.com/s4wave/spacewave/os/system"
	os_system_controller "github.com/s4wave/spacewave/os/system/controller"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
	"github.com/sirupsen/logrus"
)

// graphSource is a block store that serves revisions to a staging: an image
// store of its own, or a Space's. After limit reads it fails, as a source cut
// off mid-copy does; a negative limit never fails. Its revisions share one
// transform, as a Space's do.
type graphSource struct {
	block.StoreOps
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
	return newStoreSource(t, vol)
}

// newStoreSource serves revisions added to store.
func newStoreSource(t *testing.T, store block.StoreOps) *graphSource {
	transform, err := os_image.NewTransform()
	if err != nil {
		t.Fatal(err)
	}
	return &graphSource{StoreOps: store, transform: transform, limit: -1}
}

// GetStoredBlock serves a stored block until the source's read limit.
func (s *graphSource) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if n := s.reads.Add(1); s.limit >= 0 && n > s.limit {
		return nil, errors.New("source cut off")
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
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

	// The node still serves C, so the stager must skip it rather than boot
	// it again in a loop, and skip the accepted A as well.
	for image, want := range map[*bucket.ObjectRef]string{imageC: "rolled-back", imageA: "accepted"} {
		if skip, err := revs.skipRelease(image); err != nil || skip != want {
			t.Fatalf("stager skipped %q, want %q: %v", skip, want, err)
		}
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

// TestStageReleaseFromNode stages the releases a placed os/system node serves,
// as a Device's console does: the stage-release child reaches the node through
// the daemon socket's os.system fallback and copies the revision from the
// Space's blocks. A release that is already accepted is skipped while the
// child keeps watching, and writing the next release to the object stages it.
func TestStageReleaseFromNode(t *testing.T) {
	// Hold revisions A, B and C in the Space.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	var imageA, imageB, imageC *bucket.ObjectRef
	err = world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		cs, err := ws.BuildStorageCursor(ctx)
		if err != nil {
			return err
		}
		defer cs.Release()
		space := newStoreSource(t, cs.GetBlockStore())
		imageA, _ = revisionGraph(t, space, "A", 2)
		imageB, _ = revisionGraph(t, space, "B", 2)
		imageC, _ = revisionGraph(t, space, "C", 3)

		// The Space keeps only blocks its objects reach, as it keeps a
		// build Step's image.
		for name, image := range map[string]*bucket.ObjectRef{"A": imageA, "B": imageB, "C": imageC} {
			obj, err := ws.CreateObject(ctx, "os/image/"+name, image)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Install A, and pin its signer in the data Volume as a first boot does.
	// The socket's path must fit a sockaddr, so the data directory is short.
	system := t.TempDir()
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(system, "image.s4wave")})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := tb.Engine.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedRevision(t, vol, newStoreSource(t, cs.GetBlockStore()), imageA)
	cs.Release()
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	key, owner := newSigner(t)
	pinned := []peer.ID{owner}
	if err := AuthorizeImage(ctx, le, system, signImage(t, key, imageA), imageA); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp("", "os-data")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(data) })
	if err := os.MkdirAll(filepath.Join(data, "daemon"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os_image.WriteSigners(filepath.Join(data, pinnedSigners), pinned); err != nil {
		t.Fatal(err)
	}

	// Place the node over the Space's release object and serve the daemon
	// socket with the os.system fallback, as serve does.
	const releaseKey = "os/release"
	publish := func(image *bucket.ObjectRef) {
		r, err := os_release.NewRelease(signImage(t, key, image))
		if err != nil {
			t.Fatal(err)
		}
		if err := os_release.WriteRelease(ctx, tb.WorldState, releaseKey, r); err != nil {
			t.Fatal(err)
		}
	}
	publish(imageB)
	node := os_system_controller.NewController(le, tb.Bus, &os_system.Config{EngineId: tb.EngineID, ObjectKey: releaseKey})
	releaseNode, err := tb.Bus.AddController(ctx, node, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseNode)
	lis, err := net.Listen("unix", filepath.Join(data, "daemon", daemonSocket))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	// Serve each connection at once, as the daemon does:
	// srpc.AcceptMuxedListener serves one connection at a time.
	srv := srpc.NewServer(srpc.NewMux(os_system.NewInvoker(tb.Bus)))
	go func() {
		for {
			nc, err := lis.Accept()
			if err != nil {
				return
			}
			mc, err := srpc.NewMuxedConn(nc, false, nil)
			if err != nil {
				_ = nc.Close()
				continue
			}
			go func() { _ = srv.AcceptMuxedConn(ctx, mc) }()
		}
	}()

	// stage runs the console's child and returns the trial it staged.
	stage := func() *bucket.ObjectRef {
		if err := RunStageRelease(StageReleaseArgs(system, data)[1:]); err != nil {
			t.Fatal(err)
		}
		revs, err := readRevisions(ctx, system, pinned)
		if err != nil {
			t.Fatal(err)
		}
		return revs.trial.image
	}

	// Watch the revisions the child reports to the node, as the daemon's
	// update producer does.
	conn, err := net.Dial("unix", filepath.Join(data, "daemon", daemonSocket))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	revStrm, err := os_system_stager.NewSRPCStagerClient(client).WatchRevision(ctx, &os_system_stager.WatchRevisionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	reports := make(chan *os_system_stager.Revision, 64)
	go func() {
		defer close(reports)
		for {
			r, err := revStrm.Recv()
			if err != nil {
				return
			}
			reports <- r
		}
	}()
	expect := func(want *os_system_stager.Revision) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case r, ok := <-reports:
				if !ok {
					t.Fatal("the revision watch ended")
				}
				if r.EqualVT(want) {
					return
				}
			case <-deadline:
				t.Fatalf("the node never reported %v", want)
			}
		}
	}

	// The node's release B stages as the trial, and a second child refuses
	// while that trial is pending.
	if trial := stage(); !trial.EqualVT(imageB) {
		t.Fatal("the child did not stage the node's release")
	}
	expect(&os_system_stager.Revision{
		Accepted: imageA.GetRootRef(),
		Update:   &os_system_stager.Revision_Trial{Trial: imageB.GetRootRef()},
	})
	if err := RunStageRelease(StageReleaseArgs(system, data)[1:]); err == nil {
		t.Fatal("a second child staged over a pending trial")
	}

	// B boots on trial and is accepted.
	sel, err := SelectRevision(ctx, le, system, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !sel.OnTrial() || !sel.Image.EqualVT(imageB) {
		t.Fatal("the staged release did not boot on trial")
	}
	if err := AcceptTrial(ctx, le, system, sel); err != nil {
		t.Fatal(err)
	}

	// The node still serves B, now accepted, so the child skips it and keeps
	// watching; publishing C stages C.
	staged := make(chan *bucket.ObjectRef, 1)
	go func() { staged <- stage() }()
	select {
	case trial := <-staged:
		t.Fatalf("the child staged %s while the node served the accepted release", trial.GetRootRef().MarshalString())
	case <-time.After(500 * time.Millisecond):
	}
	expect(&os_system_stager.Revision{Accepted: imageB.GetRootRef()})
	publish(imageC)
	if trial := <-staged; !trial.EqualVT(imageC) {
		t.Fatal("the child did not stage the next release")
	}

	// C boots on trial and never gets healthy, so the next boot rolls it
	// back, and the child reports the rollback.
	if sel, err := SelectRevision(ctx, le, system, pinned); err != nil || !sel.OnTrial() {
		t.Fatalf("C did not boot on trial: %v", err)
	}
	if sel, err := SelectRevision(ctx, le, system, pinned); err != nil || sel.OnTrial() {
		t.Fatalf("C did not roll back: %v", err)
	}
	go func() { staged <- stage() }()
	expect(&os_system_stager.Revision{
		Accepted: imageB.GetRootRef(),
		Update:   &os_system_stager.Revision_RolledBack{RolledBack: imageC.GetRootRef()},
	})

	// Staging the next release frees the rollback's record, so the roots
	// again tell what the last update did.
	publish(imageA)
	if trial := <-staged; !trial.EqualVT(imageA) {
		t.Fatal("the child did not stage the release after the rollback")
	}
	revs, err := readRevisions(ctx, system, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if revs.rolledBack != nil {
		t.Fatal("staging a trial kept the rollback's record")
	}
	expect(&os_system_stager.Revision{
		Accepted: imageB.GetRootRef(),
		Update:   &os_system_stager.Revision_Trial{Trial: imageA.GetRootRef()},
	})

	// Deleting the release shows the node waiting for one again.
	if _, err := tb.WorldState.DeleteObject(ctx, releaseKey); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		_, detail, changed := node.GetNodeStatus()
		if detail == "waiting for a release" {
			break
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("the node still shows %q after the release was deleted", detail)
		}
	}
}
