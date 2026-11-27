package space_backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	auth_password "github.com/s4wave/spacewave/auth/method/password"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/session"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// RestoreOptions selects immutable objects and one UnixFS tree. Identity is an
// entity credential, not a Session credential, so recovery needs no device.
type RestoreOptions struct {
	// Log receives operational messages without credentials.
	Log *logrus.Entry
	// Objects reads the authenticated object store without writes.
	Objects block_store_s3.ObjectStore
	// Bucket selects the object store's bucket, empty for Drive.
	Bucket string
	// Prefix is the source Space's object prefix.
	Prefix string
	// Snapshot selects an immutable key, empty for the newest seqno and ULID.
	Snapshot string
	// Root is an explicitly decrypted export-root file, nil to read a sealed index.
	// It preserves recovery from an existing exported root without a second command.
	Root *s4wave_world.WorldRootSnapshot
	// Identity unlocks the sealed index with a password or PEM entity key.
	Identity *session.EntityCredential
	// Object is the UnixFS World object key to restore.
	Object string
	// Destination must be a path that does not already exist.
	Destination string
	// VerifyLFS requires every file name to be its SHA-256 Git LFS OID.
	VerifyLFS bool
}

// LatestIndex streams immutable snapshot names and chooses by numeric seqno,
// then ULID; lexical comparison alone would rank seqno 9 above seqno 10.
func LatestIndex(ctx context.Context, objects block_store_s3.ObjectStore, bucket, prefix string) (string, error) {
	// Keep only the newest valid name rather than collecting an unbounded history.
	var newest string
	var seq uint64
	var id string
	err := objects.ListObjects(ctx, bucket, prefix+"snapshots/", func(key string, _ int64) error {
		// Ignore unrelated objects and rank only complete immutable index names.
		suffix := strings.TrimPrefix(key, prefix+"snapshots/")
		seqstr, ulid, ok := strings.Cut(suffix, "-")
		if !ok || len(ulid) != 26 {
			return nil
		}
		n, err := strconv.ParseUint(seqstr, 10, 64)
		if err != nil {
			return nil
		}
		if newest == "" || n > seq || n == seq && ulid > id {
			newest, seq, id = key, n, ulid
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if newest == "" {
		return "", errors.New("backup folder contains no complete snapshot index")
	}
	return newest, nil
}

// ResolveIdentity derives the same entity key as local account recovery does.
func ResolveIdentity(accountID string, identity *session.EntityCredential) (crypto.PrivKey, error) {
	// A password is derived with the original account identity, never this device.
	if password := identity.GetPassword(); password != "" {
		_, key, err := auth_password.BuildParametersWithUsernamePassword(accountID, []byte(password))
		return key, err
	}
	if pem := identity.GetPemPrivateKey(); len(pem) != 0 {
		return keypem.ParsePrivKeyPem(pem)
	}
	return nil, errors.New("an account password or PEM entity key is required")
}

// Restore reads a sealed index and copies one tree into a new directory. The
// PackStore is read only and the procedure performs no device or cloud lookup.
func Restore(ctx context.Context, opts RestoreOptions) (int, error) {
	// Refuse to overwrite any existing path, including a symlink.
	if opts.Objects == nil || opts.Object == "" || opts.Destination == "" {
		return 0, errors.New("restore needs storage, a UnixFS object and a new destination")
	}
	if _, err := os.Lstat(opts.Destination); !os.IsNotExist(err) {
		if err != nil {
			return 0, err
		}
		return 0, errors.New("restore destination exists; choose a new directory")
	}
	root := opts.Root
	if root == nil {
		key := opts.Snapshot
		if key == "" {
			var err error
			key, err = LatestIndex(ctx, opts.Objects, opts.Bucket, opts.Prefix)
			if err != nil {
				return 0, err
			}
		}
		snapshot, err := ReadSnapshot(ctx, opts.Objects, opts.Bucket, key, opts.Identity)
		if err != nil {
			return 0, err
		}
		root = snapshot.GetWorld()
	}

	// Open immutable packs without a mutable index, compaction or refresher.
	store := block_store_s3.NewPackStore(opts.Log, opts.Objects, opts.Bucket, opts.Prefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true, SoleWriter: true, ReadOnlyIndex: true,
	})
	defer store.Close()
	engine, err := world_block.OpenSnapshot(ctx, opts.Log, store, root.GetRootRef())
	if err != nil {
		return 0, err
	}
	defer engine.Close()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	// Open the requested tree and copy every file; keep a partial result on failure.
	handle, err := unixfs_world.BuildFSFromUnixfsRef(ctx, opts.Log, tx, "", &unixfs_world.UnixfsRef{ObjectKey: opts.Object}, false, false, time.Time{})
	if err != nil {
		return 0, err
	}
	defer handle.Release()
	typ, err := handle.GetNodeType(ctx)
	if err != nil {
		return 0, err
	}
	if !typ.GetIsDirectory() {
		return 0, errors.New("restore object must be a UnixFS directory")
	}

	// Create the new destination only after the selected tree opened successfully.
	if err := os.Mkdir(opts.Destination, 0o700); err != nil {
		return 0, err
	}
	if err := unixfs_sync.Sync(ctx, opts.Destination, handle, unixfs_sync.DeleteMode_DeleteMode_NONE, nil); err != nil {
		return 0, err
	}
	if !opts.VerifyLFS {
		return 0, nil
	}
	return VerifyLFS(ctx, opts.Destination)
}

// VerifyLFS streams each restored file and verifies its basename as a SHA-256
// OID. It rejects other file names and special files, rather than silently
// reporting an unchecked file as restored correctly.
func VerifyLFS(ctx context.Context, directory string) (int, error) {
	// Anchor verification reads to the restored directory even if paths change.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return 0, err
	}
	defer root.Close()

	// Walk paths and hash one regular file at a time with bounded memory.
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		// Accept only regular files below directories, with a full SHA-256 name.
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.Errorf("LFS restore contains a non-regular file: %s", path)
		}
		oid := entry.Name()
		if decoded, err := hex.DecodeString(oid); err != nil || len(decoded) != sha256.Size {
			return errors.Errorf("LFS file name is not a SHA-256 OID: %s", path)
		}

		// Read the restored file without keeping another copy of its contents.
		file, err := root.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		_, readErr := io.Copy(digest, file)
		closeErr := file.Close()

		// Check both I/O completion and the restored file's content hash.
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(digest.Sum(nil)) != oid {
			return errors.Errorf("LFS SHA-256 does not match its OID: %s", path)
		}
		count++
		return nil
	})
	return count, err
}

// ReadSnapshot reads and unseals one immutable index without device state.
func ReadSnapshot(
	ctx context.Context, objects block_store_s3.ObjectStore, bucket, key string,
	identity *session.EntityCredential,
) (*account_settings.SpaceBackupSnapshot, error) {
	// Recover the root using just the sealed header and an entity credential.
	body, err := objects.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(body, 16<<20))
	_ = body.Close() // The bounded index response is fully read or rejected.
	if err != nil {
		return nil, err
	}

	// Decode the public header before deriving the original account's entity key.
	index := &account_settings.SpaceBackupIndex{}
	if err := index.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode sealed backup index")
	}
	priv, err := ResolveIdentity(index.GetProviderAccountId(), identity)
	if err != nil {
		return nil, err
	}
	snapshot, err := Unseal(index, []crypto.PrivKey{priv})
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}
