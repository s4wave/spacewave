// Package space_backup copies a pinned Space into immutable encrypted packs.
// Its local proofs commit only after packs, and its sealed index is published
// only after every reachable block is proven. It never deletes remote objects
// or writes the source World. This upkeep runs below the Flowgraph, because a
// Space must be recoverable before any Flowgraph can load.
package space_backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	packcopy_world "github.com/s4wave/spacewave/db/block/packcopy/world"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// OpenTarget opens an append-only pack store with the supplied local index.
// The returned release stops its readers and background indexer, never deletes.
type OpenTarget func(context.Context, kvtx.Store) (*block_store_s3.PackStore, block_store_s3.ObjectStore, func(), error)

// Pass is one pinned copy. Its caller keeps Snapshot's roots held locally until
// this returns, and keeps the newest published roots held across future passes.
// No caller supplies proofs separately: their namespace is bound to the exact
// backend, folder and object prefix in Destination, preventing cross-folder reuse.
type Pass struct {
	// Log receives progress without credentials or snapshot decryption material.
	Log *logrus.Entry
	// Snapshot is the pinned durable World and its retained roots.
	Snapshot *account_settings.SpaceBackupSnapshot
	// Source reads the mounted ciphertext block store.
	Source block.StoreOps
	// Local is the device-local transactional store, never a Space object.
	Local kvtx.Store
	// Destination is the backend/folder/prefix identity, including the Space ID.
	Destination string
	// Prefix is the per-Space directory below the backend's root folder.
	Prefix string
	// Pin holds a changed snapshot before any traversal or upload, nil when the caller already holds it.
	Pin func(context.Context) error
	// Open lazily opens the target only after the unchanged-root check.
	Open OpenTarget
	// RootTypes resolves object types whose source store did not record refs.
	RootTypes packcopy_world.RootTypes
	// ProviderAccountID is the username used for entity password derivation.
	ProviderAccountID string
	// EntityID is the principal whose existing recovery keys receive the index.
	EntityID string
	// Recipients are entity public keys, never device or session keys.
	Recipients []crypto.PubKey
}

// Namespace binds proof, pack-index and publication rows to one destination.
func Namespace(destination string) []byte {
	sum := sha256.Sum256([]byte(destination))
	return []byte("space-backup/" + hex.EncodeToString(sum[:]) + "/")
}

// Run copies the pinned graph and returns an unpublished incomplete result or
// a fully published index. It makes no remote call for unchanged roots.
func (p *Pass) Run(
	ctx context.Context,
	progress func(*account_settings.SpaceBackupProgress),
) (*account_settings.SpaceBackupPublished, *account_settings.SpaceBackupProgress, error) {
	// Require a complete pin and a destination before opening any storage.
	ctx = block.WithBackground(ctx)
	if p.Snapshot.GetWorld().GetRootRef().GetRootRef().GetEmpty() || p.Local == nil || p.Source == nil || p.Open == nil || p.Destination == "" {
		return nil, nil, errors.New("backup requires a pinned World, local state, source and destination")
	}
	local := kvtx_prefixer.NewPrefixer(p.Local, Namespace(p.Destination))
	previous, err := ReadPublished(ctx, local)
	if err != nil {
		return nil, nil, err
	}

	// Bind no-change reuse to the account's current recovery recipients.
	if len(p.Recipients) == 0 || p.ProviderAccountID == "" || p.EntityID == "" {
		return nil, nil, errors.New("backup needs account entity recovery keys; configure a password or PEM auth method before running")
	}
	fingerprint, err := recipientFingerprint(p.ProviderAccountID, p.EntityID, p.Recipients)
	if err != nil {
		return nil, nil, err
	}
	if previous.GetSnapshot().EqualVT(p.Snapshot) && bytes.Equal(previous.GetRecipientFingerprint(), fingerprint) {
		return previous, nil, nil
	}

	// An unchanged snapshot is already retained by its published hold.
	if p.Pin != nil {
		if err := p.Pin(ctx); err != nil {
			return nil, nil, err
		}
	}

	// Read through the source's copy view instead of filling its per-block cache.
	source := p.Source
	if copySource, ok := source.(block.CopySource); ok {
		source = copySource.GetCopySource()
	}

	// Type only changed heads; source reads stay bounded and outside user writes.
	heads := []*bucket.ObjectRef{p.Snapshot.GetWorld().GetRootRef()}
	roots := []*block.BlockRef{heads[0].GetRootRef()}
	for _, retained := range p.Snapshot.GetRetainedRoots() {
		heads = append(heads, &bucket.ObjectRef{RootRef: retained, TransformConf: heads[0].GetTransformConf()})
		roots = append(roots, retained)
	}

	// Open the typed graph reader and the destination's local pack index.
	typer, err := packcopy_world.New(ctx, p.Log, source, heads, p.RootTypes)
	if err != nil {
		return nil, nil, err
	}
	index := kvtx_prefixer.NewPrefixer(local, []byte("index/"))
	target, objects, release, err := p.Open(ctx, index)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	// The copier commits a proof only after its pack and every child are durable.
	reportedHoles := 0
	result, err := packcopy.Copy(block.WithBackground(ctx), &packcopy.Options{
		Roots: roots, Target: target, Sources: []packcopy.Source{source},
		Proofs: kvtx_prefixer.NewPrefixer(local, []byte("proofs/")), Typer: typer,
		Reads: 2, Uploads: 1,
		Holes: func(ref *block.BlockRef, reason string) error {
			// Report a bounded sample while the copier retains only reason counts.
			if reportedHoles < 5 && p.Log != nil {
				p.Log.WithFields(logrus.Fields{"block": ref.MarshalString(), "reason": reason}).Warn("backup source block is unavailable")
			}
			reportedHoles++
			return nil
		},
		Progress: func(result packcopy.Result) {
			if progress != nil {
				progress(copyProgress(&result))
			}
		},
	})
	counts := copyProgress(result)
	if err != nil {
		return nil, counts, err
	}
	if !result.Complete || counts.GetHoles() != 0 {
		return nil, counts, nil
	}

	// Seal all decryption material before the first index byte leaves this device.
	sealed, err := Seal(p.Snapshot, p.ProviderAccountID, p.EntityID, p.Recipients)
	if err != nil {
		return nil, counts, err
	}
	key := p.Prefix + "snapshots/" + strconv.FormatUint(p.Snapshot.GetWorld().GetSeqno(), 10) + "-" + ulid.NewULID()
	if err := objects.PutObject(ctx, "", key, sealed, "application/octet-stream"); err != nil {
		return nil, counts, err
	}

	// Record publication last; a crash before this commit only leaves an extra index.
	published := &account_settings.SpaceBackupPublished{
		Key: key, Snapshot: p.Snapshot.CloneVT(), PublishedUnixSeconds: time.Now().Unix(),
		RecipientFingerprint: fingerprint,
	}
	data, err := published.MarshalVT()
	if err != nil {
		return nil, counts, err
	}
	open := func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, true) }
	if err := kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte("published"), data)
	}); err != nil {
		return nil, counts, err
	}
	return published, counts, nil
}

// ReadPublished reads one bounded local publication row, without contacting Drive.
func ReadPublished(ctx context.Context, local kvtx.Store) (*account_settings.SpaceBackupPublished, error) {
	// Copy the encoded row within one short read transaction.
	tx, err := local.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte("published"))
	if err != nil || !found {
		return nil, err
	}
	published := &account_settings.SpaceBackupPublished{}
	if err := published.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return published, nil
}

// copyProgress bounds readback to counts instead of retaining every missing ref.
func copyProgress(result *packcopy.Result) *account_settings.SpaceBackupProgress {
	// An early copy failure may return no counters.
	if result == nil {
		return &account_settings.SpaceBackupProgress{}
	}
	counts := &account_settings.SpaceBackupProgress{ReadBytes: result.ReadBytes, WrittenBytes: result.WrittenBytes, Packs: result.Packs}
	for _, holes := range result.Holes {
		counts.Holes += holes
	}
	return counts
}
