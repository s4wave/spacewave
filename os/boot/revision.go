package boot

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// The image store's revision roots, as OSRevision.tla models them. A revision
// slot names its authorization block: the signed image root, stored with that
// root as its one child, so one named root holds the revision's whole block
// graph and a single publish moves a revision with its authorization.
const (
	// acceptedRoot names the revision the Device falls back to.
	acceptedRoot = "accepted"
	// trialRoot names a staged revision to boot once.
	trialRoot = "trial"
	// attemptedRoot names the trial's authorization once a boot has tried it.
	attemptedRoot = "attempted"
	// rolledBackRoot names a record of the image root of the last trial that
	// failed. The record has no children, so it keeps none of that revision.
	rolledBackRoot = "rolled-back"
)

// stageReads is the number of source reads a staging keeps in flight. A source
// across the network serves one block per round trip, so the copy needs many
// reads at once; a test sets one to cut a staging at a known block.
var stageReads = block.DefaultGraphCopyReads

// revision is one revision slot: its authorization block and, once verified
// against the pinned signers, the image root it authorizes.
type revision struct {
	// auth is the slot's authorization block, or nil when the slot is free.
	auth *block.BlockRef
	// root is the authorized image root, set only when err is nil.
	root *block.BlockRef
	// signer is the pinned signer that authorized root.
	signer peer.ID
	// err is why the slot's authorization does not verify.
	err error
}

// revisions is one snapshot of the image store's revision roots.
type revisions struct {
	// accepted and trial are the two revision slots.
	accepted, trial revision
	// attempted and rolledBack are the named roots of the same names, nil
	// when free.
	attempted, rolledBack *block.BlockRef
}

// Selection is the revision a boot serves, with the slots it read, so a trial
// is accepted only from the state this boot selected.
type Selection struct {
	// Root is the image root to serve.
	Root *block.BlockRef
	// accepted is the accepted slot's authorization block.
	accepted *block.BlockRef
	// trial is the trial's authorization block while this boot tries it.
	trial *block.BlockRef
}

// OnTrial reports whether this boot serves a trial, which must reach health
// before its deadline or roll back on the next boot.
func (s *Selection) OnTrial() bool {
	return s.trial != nil
}

// AcceptArgs returns the init arguments that accept this trial in the image
// store in dir from another process. The serving process holds the store's
// read-only handle, and s4db keeps one handle per file in a process, so the
// console runs the accept as a child.
func (s *Selection) AcceptArgs(dir string) []string {
	return []string{"accept-trial", dir, s.Root.MarshalString(), s.accepted.MarshalString(), s.trial.MarshalString()}
}

// RunAcceptTrial accepts the trial that the four arguments after accept-trial
// name, as AcceptArgs wrote them: the image directory, the served root, and
// the accepted and trial authorizations the boot selected.
func RunAcceptTrial(args []string) error {
	// Decode the selection; an absent accepted slot is an empty argument.
	sel := &Selection{}
	for i, ref := range []**block.BlockRef{&sel.Root, &sel.accepted, &sel.trial} {
		var err error
		if *ref, err = block.UnmarshalBlockRefB58(args[i+1]); err != nil {
			return err
		}
	}

	// Publish the acceptance against the slots the boot selected from.
	return AcceptTrial(context.Background(), logrus.NewEntry(logrus.New()), args[0], sel)
}

// blockReader reads stored blocks, as a read-only image store does.
type blockReader interface {
	// GetBlock returns the block's bytes and whether the store holds it.
	GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error)
}

// SelectRevision chooses the revision this boot serves from the image store in
// dir, verifying each authorization against the pinned signers. A pending trial
// that no boot has tried is marked attempted, durably, before it is returned;
// a trial already tried, or one no pinned signer authorized, is dropped and
// recorded as rolled back, and the accepted revision is served. The store is
// written only in those two cases, so an ordinary boot changes no byte.
func SelectRevision(ctx context.Context, le *logrus.Entry, dir string, pinned []peer.ID) (*Selection, error) {
	// Read and verify the revision roots in one read-only snapshot.
	revs, err := readRevisions(ctx, dir, pinned)
	if err != nil {
		return nil, err
	}

	// Try a pending authorized trial once: mark it before serving it.
	trial := revs.trial
	if trial.auth != nil && trial.err == nil && !trial.auth.EqualsRef(revs.attempted) {
		err := setRevisionRoots(ctx, le, dir, nil, []block.NamedRoot{{Name: attemptedRoot, Previous: revs.attempted, Ref: trial.auth}})
		if err != nil {
			return nil, err
		}
		println("SPACEWAVE_OS_TRIAL root=" + trial.root.MarshalString())
		println("SPACEWAVE_OS_AUTHORIZED signer=" + trial.signer.String() + " root=" + trial.root.MarshalString())
		return &Selection{Root: trial.root, accepted: revs.accepted.auth, trial: trial.auth}, nil
	}

	// Drop a trial already tried or not authorized, recording its root.
	if trial.auth != nil {
		if err := rollBack(ctx, le, dir, revs); err != nil {
			return nil, err
		}
	}

	// Serve the accepted revision.
	accepted := revs.accepted
	if accepted.auth == nil {
		return nil, errors.New("image store holds no accepted revision; install a card first")
	}
	if accepted.err != nil {
		return nil, accepted.err
	}
	println("SPACEWAVE_OS_AUTHORIZED signer=" + accepted.signer.String() + " root=" + accepted.root.MarshalString())
	return &Selection{Root: accepted.root, accepted: accepted.auth}, nil
}

// AcceptTrial makes the trial sel serves the accepted revision, in one publish
// that also frees the trial and attempted names. It refuses, and writes
// nothing, when the store no longer holds the slots sel was selected from.
func AcceptTrial(ctx context.Context, le *logrus.Entry, dir string, sel *Selection) error {
	// Accept only a trial this boot is serving.
	if !sel.OnTrial() {
		return errors.New("accept: this boot serves no trial")
	}
	err := setRevisionRoots(ctx, le, dir, nil, []block.NamedRoot{
		{Name: acceptedRoot, Previous: sel.accepted, Ref: sel.trial},
		{Name: trialRoot, Previous: sel.trial},
		{Name: attemptedRoot, Previous: sel.trial},
	})
	if err != nil {
		return err
	}
	println("SPACEWAVE_OS_ACCEPTED root=" + sel.Root.MarshalString())
	return nil
}

// AuthorizeImage makes the image a seed built the accepted revision, with
// signed as its authorization, in one publish that also frees the built
// image's name. The caller verifies signed against the signers its card
// pins; root must be the root signed names and the store's built root.
func AuthorizeImage(ctx context.Context, le *logrus.Entry, dir string, signed *peer.SignedMsg, root *block.BlockRef) error {
	// Build the authorization block that will hold the built root.
	auth, err := authorizationBlock(signed, root)
	if err != nil {
		return err
	}

	// Install it as the accepted revision only while no revision is accepted.
	return setRevisionRoots(ctx, le, dir, []*block.PutBatchEntry{auth}, []block.NamedRoot{
		{Name: acceptedRoot, Ref: auth.Ref},
		{Name: builtImageRoot, Previous: root},
	})
}

// StageRevision copies the revision signed authorizes from src, a store that
// serves each block with its child list, such as another image store or a
// Space's bucket, into the image store in dir and publishes it as the trial,
// with its authorization, in one commit. It refuses while another trial is
// pending. It returns the number of blocks it copied.
func StageRevision(ctx context.Context, le *logrus.Entry, dir string, src block.StoreOps, signed *peer.SignedMsg, root *block.BlockRef) (_ int, retErr error) {
	// Build the authorization before copying, so a bad one copies nothing.
	auth, err := authorizationBlock(signed, root)
	if err != nil {
		return 0, err
	}
	vol, err := openImageWriter(ctx, le, dir)
	if err != nil {
		return 0, err
	}
	defer func() { retErr = errors.Join(retErr, vol.Close()) }()

	// Copy the blocks the store lacks, then publish the trial.
	copied, err := copyRevision(ctx, vol, src, root)
	if err != nil {
		return copied, err
	}
	err = vol.SetBucketRoots(ctx, imageBucket, []*block.PutBatchEntry{auth}, []block.NamedRoot{{Name: trialRoot, Ref: auth.Ref}})
	if err != nil {
		return copied, err
	}
	println("SPACEWAVE_OS_STAGED root=" + root.MarshalString())
	return copied, nil
}

// copyRevision copies every block of root's graph that vol lacks from src and
// returns how many it wrote. block.CopyGraph walks the graph, stops at every
// block vol already holds and writes each block after its children; the
// buffer commits those writes to vol in batches, in the same order. So a held
// block always holds its whole graph: a repeated staging copies nothing, and a
// staging cut short keeps what it completed and resumes where it stopped.
func copyRevision(ctx context.Context, vol *volume_s4db.Volume, src block.StoreOps, root *block.BlockRef) (int, error) {
	buf := block.NewBufferedStore(ctx, vol)
	var copied int
	err := block.CopyGraph(ctx, src, buf, root, &block.GraphCopyOptions{
		Reads: stageReads,
		Known: buf.GetBlockExistsBatch,
		Complete: func(context.Context, *block.BlockRef) error {
			copied++
			return nil
		},
		Forget: true,
	})
	return copied, errors.Join(err, buf.Flush(ctx))
}

// OpenImageStore opens the image store in dir read-only, as a staging's
// source, and returns it with the root its seed built. Close releases it.
func OpenImageStore(ctx context.Context, le *logrus.Entry, dir string) (_ *volume_s4db.Volume, _ *block.BlockRef, retErr error) {
	// Open the store without its writer lock; it refuses a store that needs
	// a conversion or a format upgrade.
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:          filepath.Join(dir, "image.s4wave"),
		NoGenerateKey: true,
		ReadOnly:      true,
	})
	if err != nil {
		return nil, nil, err
	}

	// Read the root the seed built.
	root, err := vol.GetBucketRoot(ctx, imageBucket, builtImageRoot)
	if err == nil && root == nil {
		err = errors.New("boot root " + builtImageRoot + " absent; store preserved")
	}
	if err != nil {
		return nil, nil, errors.Join(err, vol.Close())
	}
	return vol, root, nil
}

// rollBack drops the pending trial and records its root as rolled back, in
// one publish, and reports the rollback on the console.
func rollBack(ctx context.Context, le *logrus.Entry, dir string, revs *revisions) error {
	// Free the trial and attempted names.
	trial := revs.trial
	roots := []block.NamedRoot{{Name: trialRoot, Previous: trial.auth}}
	if revs.attempted != nil {
		roots = append(roots, block.NamedRoot{Name: attemptedRoot, Previous: revs.attempted})
	}

	// Say why the trial rolled back.
	reason := "attempted"
	if trial.err != nil {
		reason = trial.err.Error()
	}

	// Record the root the trial named, verified or not, so the node that
	// staged it does not stage it again.
	var entries []*block.PutBatchEntry
	root := trial.root
	if root == nil {
		root = claimedRoot(ctx, dir, trial.auth)
	}
	if root != nil {
		data, err := root.MarshalVT()
		if err != nil {
			return err
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			return err
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		roots = append(roots, block.NamedRoot{Name: rolledBackRoot, Previous: revs.rolledBack, Ref: ref})
	}
	if err := setRevisionRoots(ctx, le, dir, entries, roots); err != nil {
		return err
	}
	println("SPACEWAVE_OS_ROLLED_BACK root=" + root.MarshalString() + " reason=" + reason)
	return nil
}

// readRevisions reads the revision roots of the store in dir from one
// read-only snapshot and verifies each slot's authorization against pinned.
func readRevisions(ctx context.Context, dir string, pinned []peer.ID) (_ *revisions, retErr error) {
	// Open the store read-only and refuse a format this boot does not serve.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	if err := checkImageFormat(ctx, db); err != nil {
		return nil, err
	}

	// Read the four names in one transaction.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	records := volume_kvtx.OpenRecords(tx)
	read := func(name string) *block.BlockRef {
		if err != nil {
			return nil
		}
		var ref *block.BlockRef
		ref, err = readNamedRoot(ctx, records, imageBucket, name)
		return ref
	}
	revs := &revisions{
		accepted:   revision{auth: read(acceptedRoot)},
		trial:      revision{auth: read(trialRoot)},
		attempted:  read(attemptedRoot),
		rolledBack: read(rolledBackRoot),
	}
	if err != nil {
		return nil, err
	}

	// Verify each occupied slot; a slot that fails keeps its reason.
	store := store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil)
	for _, slot := range []*revision{&revs.accepted, &revs.trial} {
		if slot.auth != nil {
			slot.root, slot.signer, slot.err = verifyRevision(ctx, store, slot.auth, pinned)
		}
	}
	return revs, nil
}

// verifyRevision reads the authorization block auth from store and returns
// the image root a pinned signer authorized, once the store holds that root.
func verifyRevision(ctx context.Context, store blockReader, auth *block.BlockRef, pinned []peer.ID) (*block.BlockRef, peer.ID, error) {
	// Decode and verify the authorization.
	signed, err := readAuthorization(ctx, store, auth)
	if err != nil {
		return nil, "", err
	}
	root, signer, err := os_image.VerifyImageRoot(signed, pinned)
	if err != nil {
		return nil, "", err
	}

	// The slot's root must be stored, as its authorization block's child.
	if _, found, err := store.GetBlock(ctx, root); err != nil || !found {
		return nil, "", errors.Join(errors.New("revision root "+root.MarshalString()+" missing; store preserved"), err)
	}
	return root, signer, nil
}

// claimedRoot returns the image root the authorization block auth names,
// without verifying it, or nil when it names none. It serves only the
// rolled-back record of a trial that failed verification.
func claimedRoot(ctx context.Context, dir string, auth *block.BlockRef) (root *block.BlockRef) {
	// Read the block from a fresh read-only handle.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil
	}
	defer db.Close()
	signed, err := readAuthorization(ctx, store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil), auth)
	if err != nil {
		return nil
	}

	// Decode the signed body as the root it claims.
	root = &block.BlockRef{}
	if root.UnmarshalVT(signed.GetData()) != nil || root.Validate(false) != nil {
		return nil
	}
	return root
}

// readAuthorization reads and decodes the authorization block auth.
func readAuthorization(ctx context.Context, store blockReader, auth *block.BlockRef) (*peer.SignedMsg, error) {
	data, found, err := store.GetBlock(ctx, auth)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("revision authorization " + auth.MarshalString() + " missing; store preserved")
	}
	return peer.UnmarshalSignedMsg(data)
}

// authorizationBlock returns the block that stores signed with root as its
// one child, after checking that signed names root.
func authorizationBlock(signed *peer.SignedMsg, root *block.BlockRef) (*block.PutBatchEntry, error) {
	// Refuse an authorization of another root.
	want, err := os_image.MarshalImageRoot(root)
	if err != nil {
		return nil, err
	}
	if string(signed.GetData()) != string(want) {
		return nil, errors.New("image authorization: does not name root " + root.MarshalString())
	}

	// Store the signed message as the block that holds its root.
	data, err := signed.MarshalVT()
	if err != nil {
		return nil, err
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{root}, RefsKnown: true}, nil
}

// setRevisionRoots stores entries and moves the image bucket's named roots in
// one commit, refusing with block.ErrRootMoved when a root does not hold its
// Previous.
func setRevisionRoots(ctx context.Context, le *logrus.Entry, dir string, entries []*block.PutBatchEntry, roots []block.NamedRoot) (retErr error) {
	vol, err := openImageWriter(ctx, le, dir)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, vol.Close()) }()
	return vol.SetBucketRoots(ctx, imageBucket, entries, roots)
}

// openImageWriter opens the image store in dir as its one writer, keeping its
// stored identity. Readers may hold read-only handles meanwhile.
func openImageWriter(ctx context.Context, le *logrus.Entry, dir string) (*volume_s4db.Volume, error) {
	return volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:          filepath.Join(dir, "image.s4wave"),
		NoGenerateKey: true,
		NoWriteKey:    true,
	})
}
