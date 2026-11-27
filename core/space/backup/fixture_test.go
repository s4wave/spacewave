package space_backup_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/block"
	packcopy_world "github.com/s4wave/spacewave/db/block/packcopy/world"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// backupFixture is a pure block-World fixture without an account or device.
type backupFixture struct {
	// source holds encrypted blocks with their recorded refs.
	source block.StoreOps
	// engine remains available for measured foreground writes.
	engine *world_block.Engine
	// snapshot names the fixture's committed World.
	snapshot *account_settings.SpaceBackupSnapshot
	// identity is the synthetic entity credential, not a device key.
	identity *session.EntityCredential
	// public is the entity's public recipient key.
	public crypto.PubKey
	// files is the expected UnixFS directory tree.
	files fs.FS
	// transformKey is checked absent from every uploaded object.
	transformKey []byte
	// log is the isolated fixture logger.
	log *logrus.Entry
}

// newBackupFixture builds an encrypted UnixFS tree using the production writers.
func newBackupFixture(t *testing.T) *backupFixture {
	files := fstest.MapFS{"empty": &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	for i := range 3 {
		data := bytes.Repeat([]byte{byte(i + 1)}, (i+1)*(64<<10))
		oid := sha256.Sum256(data)
		name := hex.EncodeToString(oid[:])
		files[name[:2]+"/"+name] = &fstest.MapFile{Data: data, Mode: 0o644, ModTime: time.Unix(1700000000, 0)}
	}
	return buildBackupFixture(t, files)
}

// buildBackupFixture uses the same encrypted writers over memory or real disk.
func buildBackupFixture(t *testing.T, files fs.FS, opts ...testbed.Option) *backupFixture {
	// Build a standalone block store, without constructing an account or Session.
	t.Helper()
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	transform, err := block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305, Key: key,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, tb.Bus, le, tb.StepFactorySet, tb.BucketId, tb.Volume.GetID(), transform, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	engine, err := world_block.NewEngine(ctx, le, cursor, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})

	// Write a nested content-addressed file tree through the real UnixFS writer.
	ws := world.NewEngineWorldState(engine, true)
	if _, _, err := unixfs_world.FsInit(ctx, ws, "", "git-lfs/media", unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	writer, _ := unixfs_world.NewFSCursorWithWriterContext(ctx, le, ws, "git-lfs/media", unixfs_world.FSType_FSType_FS_NODE, "")
	defer writer.Release()
	handle, err := unixfs.NewFSHandle(writer)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()
	if err := unixfs_sync.SyncFromFS(ctx, handle, files, unixfs_sync.DeleteMode_DeleteMode_NONE, nil); err != nil {
		t.Fatal(err)
	}

	// Flush the source root and carry its transform inline, as a backup must.
	if _, err := engine.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	root := engine.GetRootRef()
	root.TransformConf, root.TransformConfRef = transform.CloneVT(), nil
	seq, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	priv, public, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		t.Fatal(err)
	}
	return &backupFixture{source: tb.Volume, engine: engine, log: le, public: public, files: files, transformKey: key,
		snapshot: &account_settings.SpaceBackupSnapshot{World: &s4wave_world.WorldRootSnapshot{RootRef: root, Seqno: seq}},
		identity: &session.EntityCredential{Credential: &session.EntityCredential_PemPrivateKey{PemPrivateKey: pem}},
	}
}

// rootTypes supplies the UnixFS root decoder to the production graph typer.
func rootTypes() packcopy_world.RootTypes {
	return packcopy_world.RootTypes{unixfs_world.FSNodeTypeID: unixfs_block.NewFSNodeBlock}
}

// restoreDestination prepares cleanup for a tree whose restored directories
// may be read-only. Permission changes happen only after its assertions finish.
func restoreDestination(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "restored")
	t.Cleanup(func() {
		// Make only this test's directories removable, without following symlinks.
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	})
	return directory
}

// checkRestoredTree compares file bytes, modes and empty directories with the source.
func checkRestoredTree(t *testing.T, fixture *backupFixture, directory string) {
	// Walk both trees through io/fs, so a missing empty directory also fails.
	t.Helper()
	if err := fs.WalkDir(fixture.files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		path := filepath.Join(directory, filepath.FromSlash(name))
		expectedInfo, err := entry.Info()
		if err != nil {
			return err
		}
		actualInfo, err := os.Stat(path)
		if err != nil || actualInfo.Mode().Perm() != expectedInfo.Mode().Perm() {
			t.Fatalf("restored mode for %s differs: %v", name, err)
		}
		if entry.IsDir() {
			if !actualInfo.IsDir() {
				t.Fatalf("restored directory %s is not a directory", name)
			}
			return nil
		}
		expected, err := fs.ReadFile(fixture.files, name)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("restored file %s differs: %v", name, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Reject additional restored entries as well as missing or changed entries.
	if err := fs.WalkDir(os.DirFS(directory), ".", func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		_, err = fs.Stat(fixture.files, name)
		return err
	}); err != nil {
		t.Fatalf("restored tree contains an unexpected path: %v", err)
	}
}
