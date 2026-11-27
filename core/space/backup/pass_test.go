package space_backup_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/core/session"
	space_backup "github.com/s4wave/spacewave/core/space/backup"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	"github.com/s4wave/spacewave/db/block/store/gdrive/gdrivetest"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// newPass composes the real copier and PackStore over an isolated fake Drive.
func newPass(t *testing.T, fixture *backupFixture, client *block_store_gdrive.Client, local kvtx.Store, destination string) *space_backup.Pass {
	t.Helper()
	return &space_backup.Pass{Log: fixture.log, Snapshot: fixture.snapshot, Source: copyOnlySource{fixture.source},
		Local: local, Destination: destination, Prefix: "space/", RootTypes: rootTypes(),
		ProviderAccountID: "recovery-account", EntityID: "recovery-account", Recipients: []crypto.PubKey{fixture.public},
		Open: func(_ context.Context, index kvtx.Store) (*block_store_s3.PackStore, block_store_s3.ObjectStore, func(), error) {
			store := block_store_s3.NewPackStore(fixture.log, client, "", "space/", &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true, Index: index})
			return store, client, store.Close, nil
		},
	}
}

// restoreFixture proves that no device, Session or account state is required.
func restoreFixture(t *testing.T, fixture *backupFixture, client *block_store_gdrive.Client) {
	// Only the object location and the synthetic PEM entity credential are passed.
	t.Helper()
	destination := restoreDestination(t)
	count, err := space_backup.Restore(t.Context(), space_backup.RestoreOptions{
		Log: fixture.log, Objects: client, Prefix: "space/", Identity: fixture.identity,
		Object: "git-lfs/media", Destination: destination, VerifyLFS: true,
	})
	if err != nil || count != 3 {
		t.Fatalf("device-free restore verified %d files: %v", count, err)
	}
	checkRestoredTree(t, fixture, destination)
}

// TestBackupRestoreAndUnchanged proves encrypted transfer, no-change short
// circuit, index-only checkpoint movement and restore without any device.
func TestBackupRestoreAndUnchanged(t *testing.T) {
	// Copy a real encrypted UnixFS tree to the fake Drive service.
	fixture := newBackupFixture(t)
	server := gdrivetest.NewServer()
	defer server.Close()
	client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	pass := newPass(t, fixture, client, store_kvtx_inmem.NewStore(), "backend/root/space/")
	published, counts, err := pass.Run(t.Context(), nil)
	if err != nil || published == nil || counts.GetPacks() == 0 {
		t.Fatalf("first pass: %v, %v, %v", published, counts, err)
	}
	before := len(server.Calls())
	again, counts, err := pass.Run(t.Context(), nil)
	if err != nil || again.GetKey() != published.GetKey() || counts != nil || len(server.Calls()) != before {
		t.Fatalf("unchanged pass made remote calls or changed the index: %v", err)
	}

	// A checkpoint seqno change with the same graph writes no pack.
	pass.Snapshot = pass.Snapshot.CloneVT()
	pass.Snapshot.World.Seqno++
	moved, counts, err := pass.Run(t.Context(), nil)
	if err != nil || moved == nil || moved.GetKey() == published.GetKey() || counts.GetPacks() != 0 {
		t.Fatalf("index-only checkpoint: %v, %v", counts, err)
	}
	if err := client.ListObjects(t.Context(), "", "space/", func(key string, _ int64) error {
		body, err := client.GetObject(t.Context(), "", key)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(body)
		_ = body.Close() // The isolated fixture body is completely consumed.
		if err != nil {
			return err
		}
		if bytes.Contains(data, fixture.transformKey) {
			t.Fatalf("Drive received the plaintext transform key in %s", key)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	restoreFixture(t, fixture, client)

	// Rotating the entity key reseals unchanged roots without uploading packs.
	private, public, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPrivKeyPem(private)
	if err != nil {
		t.Fatal(err)
	}
	pass.Recipients = []crypto.PubKey{public}
	rotated, counts, err := pass.Run(t.Context(), nil)
	if err != nil || rotated.GetKey() == moved.GetKey() || counts.GetPacks() != 0 {
		t.Fatalf("recovery-key rotation did not reseal the unchanged graph: %v, %v", counts, err)
	}
	fixture.identity = &session.EntityCredential{Credential: &session.EntityCredential_PemPrivateKey{PemPrivateKey: pem}}
	restoreFixture(t, fixture, client)
	assertNoDelete(t, server)
}

// TestMissingBlockNeverPublishes leaves a reachable hole, then repairs it.
func TestMissingBlockNeverPublishes(t *testing.T) {
	// Hide one reachable block from the copier while retaining valid type metadata.
	fixture := newBackupFixture(t)
	server := gdrivetest.NewServer()
	defer server.Close()
	client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	pass := newPass(t, fixture, client, store_kvtx_inmem.NewStore(), "backend/root/space/")
	pass.Source = hiddenRoot{StoreOps: fixture.source, root: fixture.snapshot.GetWorld().GetRootRef().GetRootRef()}
	published, counts, err := pass.Run(t.Context(), nil)
	if err != nil || published != nil || counts.GetHoles() == 0 {
		t.Fatalf("incomplete copy: %v, %v, %v", published, counts, err)
	}
	if _, err := space_backup.LatestIndex(t.Context(), client, "", "space/"); err == nil {
		t.Fatal("an index was visible with a reachable block missing")
	}

	// Repairing the source lets the same local proof store finish the copy.
	pass.Source = fixture.source
	published, _, err = pass.Run(t.Context(), nil)
	if err != nil || published == nil {
		t.Fatalf("repair: %v", err)
	}
	restoreFixture(t, fixture, client)
	assertNoDelete(t, server)
}

// TestBackupResumesAfterEveryDriveCall cancels after every call position of a
// successful pass; any visible index must restore before and after resuming.
func TestBackupResumesAfterEveryDriveCall(t *testing.T) {
	// Measure the complete call sequence without adding restore calls to it.
	fixture := newBackupFixture(t)
	baseline := gdrivetest.NewServer()
	client, err := block_store_gdrive.NewClient(baseline.HTTP.Client(), "root", baseline.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	pass := newPass(t, fixture, client, store_kvtx_inmem.NewStore(), "backend/root/space/")
	if published, _, err := pass.Run(t.Context(), nil); err != nil || published == nil {
		t.Fatalf("baseline: %v", err)
	}
	calls := len(baseline.Calls())
	baseline.Close()

	// Every interruption gets its own remote folder and persistent local KV.
	for at := 1; at <= calls; at++ {
		t.Run(strconv.Itoa(at), func(t *testing.T) {
			server := gdrivetest.NewServer()
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			transport := &stopTransport{base: server.HTTP.Client().Transport, after: at, cancel: cancel}
			client, err := block_store_gdrive.NewClient(&http.Client{Transport: transport}, "root", server.HTTP.URL)
			if err != nil {
				t.Fatal(err)
			}
			local := store_kvtx_inmem.NewStore()
			pass := newPass(t, fixture, client, local, "backend/root/space/")
			_, _, _ = pass.Run(ctx, nil) // Cancellation may follow a successful final publication.

			// A new process has no client cache, but keeps the durable local proofs.
			fresh, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := space_backup.LatestIndex(t.Context(), fresh, "", "space/"); err == nil {
				restoreFixture(t, fixture, fresh)
			}
			pass = newPass(t, fixture, fresh, local, "backend/root/space/")
			if published, _, err := pass.Run(t.Context(), nil); err != nil || published == nil {
				t.Fatalf("resume after call %d: %v", at, err)
			}
			restoreFixture(t, fixture, fresh)
			assertNoDelete(t, server)
		})
	}
}

// TestProofsAreBoundToFolder makes a completed proof store serve a different
// destination. Every block must be copied there before its index is published.
func TestProofsAreBoundToFolder(t *testing.T) {
	// Publish to the first folder and keep its durable proof store.
	fixture := newBackupFixture(t)
	server := gdrivetest.NewServer()
	defer server.Close()
	local := store_kvtx_inmem.NewStore()
	first, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root-a", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	if published, _, err := newPass(t, fixture, first, local, "backend/root-a/space/").Run(t.Context(), nil); err != nil || published == nil {
		t.Fatalf("first folder: %v", err)
	}

	// The same local proofs cannot assert that the second folder holds anything.
	second, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root-b", server.HTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	published, counts, err := newPass(t, fixture, second, local, "backend/root-b/space/").Run(t.Context(), nil)
	if err != nil || published == nil || counts.GetPacks() == 0 {
		t.Fatalf("reused proofs from another folder: %v, %v", counts, err)
	}
	restoreFixture(t, fixture, second)
	assertNoDelete(t, server)
}

// assertNoDelete checks the server's complete recorder, including refused calls.
func assertNoDelete(t *testing.T, server *gdrivetest.Server) {
	t.Helper()
	for _, call := range server.Calls() {
		if call.Method == http.MethodDelete || call.Method == http.MethodPatch {
			t.Fatalf("backup mutated a Drive object: %+v", call)
		}
	}
}

// TestUploadLimitPublishesNothing injects a missing pack and an upload limit
// at the pack boundary, then proves that resuming reads the entire tree.
func TestUploadLimitPublishesNothing(t *testing.T) {
	fixture := newBackupFixture(t)
	for _, injected := range []error{errors.New("injected missing pack upload"), block_store_gdrive.ErrUploadLimit} {
		// Reject a pack, rather than an index, so premature publication would fail restore.
		server := gdrivetest.NewServer()
		client, err := block_store_gdrive.NewClient(server.HTTP.Client(), "root", server.HTTP.URL)
		if err != nil {
			t.Fatal(err)
		}
		pass := newPass(t, fixture, client, store_kvtx_inmem.NewStore(), "backend/root/space/")
		fault := &rejectPack{ObjectStore: client, err: injected}
		pass.Open = func(_ context.Context, index kvtx.Store) (*block_store_s3.PackStore, block_store_s3.ObjectStore, func(), error) {
			store := block_store_s3.NewPackStore(fixture.log, fault, "", "space/", &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true, Index: index})
			return store, fault, store.Close, nil
		}
		if published, _, err := pass.Run(t.Context(), nil); published != nil || !errors.Is(err, injected) {
			t.Fatalf("failed pack was published: %v, %v", published, err)
		}
		if _, err := space_backup.LatestIndex(t.Context(), client, "", "space/"); err == nil {
			t.Fatal("index visible while its pack was absent")
		}

		// Keep the same proof KV, then finish through the unmodified object store.
		pass = newPass(t, fixture, client, pass.Local, "backend/root/space/")
		if published, _, err := pass.Run(t.Context(), nil); err != nil || published == nil {
			t.Fatalf("resume after rejected pack: %v", err)
		}
		restoreFixture(t, fixture, client)
		assertNoDelete(t, server)
		server.Close()
	}
}
