package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

// startSFTP serves the real filesystem until the test ends and returns the
// server's address and host key.
func startSFTP(t *testing.T) (string, string) {
	// Accept one login under a fresh host key.
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	conf := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) != "pass" {
				return nil, errors.New("denied")
			}
			return nil, nil
		},
	}
	conf.AddHostKey(signer)

	// Serve each connection's SFTP subsystem.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	go func() {
		for {
			nc, err := lis.Accept()
			if err != nil {
				return
			}
			go serveSFTP(nc, conf)
		}
	}()
	return lis.Addr().String(), string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
}

// serveSFTP answers one SSH connection.
func serveSFTP(nc net.Conn, conf *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nc, conf)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		ch, creqs, err := nch.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				_ = r.Reply(r.Type == "subsystem", nil)
				if r.Type == "subsystem" {
					go func() {
						if srv, err := sftp.NewServer(ch); err == nil {
							_ = srv.Serve()
						}
						_ = ch.Close()
					}()
				}
			}
		}()
	}
}

// buildRef returns a block's ref.
func buildRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestCommandRunThrough runs the command's passes on one real scratch file. The
// source finds its blocks through a copy of its pack index file and holds the
// root with its refs unknown, so the first pass lists the root as a hole and
// reaches nothing under it. The second pass takes the root's refs and the
// missing child from a frame file, and a store opened on the target's pack
// index file then places both blocks without reading any pack. A third pass
// adds a parent and child that only a SQLite Volume file holds, read beside
// the file's open writer into larger packs, and the writer still commits.
func TestCommandRunThrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Fill the source over SFTP with a root packed without its refs, whose
	// child is missing, indexing it in the source's pack index file.
	addr, hostKey := startSFTP(t)
	sourceDir := filepath.Join(dir, "source")
	root, child := []byte("root"), []byte("child")
	rootRef, childRef := buildRef(t, root), buildRef(t, child)
	client, err := block_store_sftp.NewClient(block_store_sftp.Config{Addr: addr, User: "user", Password: "pass", HostKey: hostKey, Conns: 1})
	if err != nil {
		t.Fatal(err)
	}
	const storeID = "p/local/acc/blk/so"
	sourceIndex := filepath.Join(dir, "source-index.db")
	seedIndex, err := store_kvtx_sqlite.OpenWithPragmas(ctx, sourceIndex, defaultIndexTable, store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		t.Fatal(err)
	}
	seed := block_store_s3.NewPackStore(le, client, sourceDir, "world/", &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(seedIndex, block_store_s3.PackIndexPrefix(storeID)),
	})
	entry := &block.PutBatchEntry{Ref: rootRef, Data: root}
	if _, err := seed.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	_ = seedIndex.Close()
	client.Close()

	// Point the command at the source and a bucket.
	bucket, objects := s3test.New(t)
	t.Setenv(sftpPasswordEnv, "pass")
	t.Setenv(keyIDEnv, "key")
	t.Setenv(keyEnv, "secret")
	holes := filepath.Join(dir, "holes.txt")
	args := &Args{
		Scratch:      filepath.Join(dir, "scratch"),
		StoreID:      storeID,
		SourceIndex:  sourceIndex,
		TargetIndex:  filepath.Join(dir, "target-index.db"),
		Width:        2,
		SFTPAddr:     addr,
		SFTPUser:     "user",
		SFTPHostKey:  hostKey,
		SFTPConns:    2,
		SFTPRoot:     sourceDir,
		SourcePrefix: "world/",
		Bucket:       s3test.BucketName,
		Endpoint:     bucket.URL(),
		TargetPrefix: "world/v2/",
		Roots:        *cli.NewStringSlice(rootRef.MarshalString()),
		Holes:        holes,
		Store:        "hyperline-world",
		Reads:        4,
		Uploads:      2,
	}

	// A sample reads the source's one pack through its read-only index.
	if err := args.runSample(&sampleArgs{count: 4, bytes: 16, reads: 2}); err != nil {
		t.Fatal(err)
	}

	// Run a pass. The root is a hole, listed for block-export, and nothing
	// under it is reached.
	if err := args.runCopy(nil); err != nil {
		t.Fatal(err)
	}
	key := hex.EncodeToString(packfile.BlockKey(rootRef.GetHash()))
	list, err := os.ReadFile(holes)
	if err != nil {
		t.Fatal(err)
	}
	if string(list) != "hyperline-world "+key+" refs-unknown\n" {
		t.Fatalf("holes list: %q", list)
	}

	// Run a pass with the pulled root, refs known, and child, and the list
	// empties.
	frame := appendFrame(t, nil, rootRef, root, []*block.BlockRef{childRef})
	frame = appendFrame(t, frame, childRef, child, nil)
	frames := filepath.Join(dir, "frames")
	if err := os.WriteFile(frames, frame, 0o600); err != nil {
		t.Fatal(err)
	}
	args.Frames = *cli.NewStringSlice(frames)
	if err := args.runCopy(nil); err != nil {
		t.Fatal(err)
	}
	if list, err = os.ReadFile(holes); err != nil || len(list) != 0 {
		t.Fatalf("holes list after the pull: %q, %v", list, err)
	}

	// A store on the target's pack index file places both blocks from the
	// index and serves them, the root with its child ref, and the graph stream
	// sits beside it.
	targetIndex, err := store_kvtx_sqlite.OpenWithPragmas(ctx, args.TargetIndex, defaultIndexTable, store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		t.Fatal(err)
	}
	defer targetIndex.Close()
	target := block_store_s3.NewPackStore(le, objects, s3test.BucketName, "world/v2/", &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      kvtx_prefixer.NewPrefixer(targetIndex, block_store_s3.PackIndexPrefix(storeID)),
		SoleWriter: true,
	})
	defer target.Close()
	located, err := target.LocateBlocks(ctx, []*block.BlockRef{rootRef, childRef})
	if err != nil || !located[0] || !located[1] {
		t.Fatalf("target index places %v, %v; want both blocks", located, err)
	}
	for _, ref := range []*block.BlockRef{rootRef, childRef} {
		stored, err := target.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !stored.RefsKnown {
			t.Fatalf("read %s: %v, %v", ref.MarshalString(), stored, err)
		}
	}
	if stored, _ := target.GetStoredBlock(ctx, rootRef); len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(childRef) {
		t.Fatalf("root refs = %v; want its child", stored.Refs)
	}
	if len(bucket.Keys("world/v2/graph/")) == 0 {
		t.Fatal("no graph stream: " + strings.Join(bucket.Keys(""), " "))
	}

	// Write a parent and its child to a Volume file with their child lists,
	// the leaf's empty, and keep the writer open.
	volumeFile := filepath.Join(dir, "volume.db")
	writer, err := store_kvtx_sqlite.Open(ctx, volumeFile, "world")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	volume := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), writer, 0, false)
	leafRef, _, err := volume.PutBlock(ctx, []byte("leaf"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err)
	}
	parentRef, _, err := volume.PutBlock(ctx, []byte("parent"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err)
	}
	listTx, err := writer.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	records := volume_kvtx.OpenRecords(listTx)
	if err := records.PutChildren(ctx, leafRef, nil); err != nil {
		t.Fatal(err)
	}
	if err := records.PutChildren(ctx, parentRef, []*block.BlockRef{leafRef}); err != nil {
		t.Fatal(err)
	}
	if err := listTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Copy it from the file into packs of up to 128 MiB, beside the writer.
	_ = targetIndex.Close()
	target.Close()
	args.Roots = *cli.NewStringSlice(rootRef.MarshalString(), parentRef.MarshalString())
	args.Volume = volumeArgs{files: *cli.NewStringSlice(volumeFile), table: "world", bucket: "hydra"}
	args.PackMiB = 128
	if err := args.runCopy(nil); err != nil {
		t.Fatal(err)
	}
	if list, err = os.ReadFile(holes); err != nil || len(list) != 0 {
		t.Fatalf("holes list after the Volume pass: %q, %v", list, err)
	}

	// The writer commits beside an open reader, which refuses to write and
	// reads the commit, and the target serves the parent with its child's ref.
	reader, err := store_kvtx_sqlite.OpenWithPragmas(ctx, volumeFile, "world", store_kvtx_sqlite.Pragmas{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.NewTransaction(ctx, true); !errors.Is(err, store_kvtx_sqlite.ErrReadOnly) {
		t.Fatalf("read-only write transaction: %v", err)
	}
	tx, err := writer.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("after"), []byte("copy")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rtx, err := reader.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := rtx.Get(ctx, []byte("after")); err != nil || !found {
		t.Fatalf("reader misses the commit: %v, %v", found, err)
	}
	rtx.Discard()
	targetIndex, err = store_kvtx_sqlite.OpenWithPragmas(ctx, args.TargetIndex, defaultIndexTable, store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		t.Fatal(err)
	}
	defer targetIndex.Close()
	target = block_store_s3.NewPackStore(le, objects, s3test.BucketName, "world/v2/", &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      kvtx_prefixer.NewPrefixer(targetIndex, block_store_s3.PackIndexPrefix(storeID)),
		SoleWriter: true,
	})
	defer target.Close()
	stored, err := target.GetStoredBlock(ctx, parentRef)
	if err != nil || stored == nil || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(leafRef) {
		t.Fatalf("parent from the Volume = %v, %v; want its leaf ref", stored, err)
	}
	if stored, err := target.GetStoredBlock(ctx, leafRef); err != nil || stored == nil || !stored.RefsKnown {
		t.Fatalf("leaf from the Volume = %v, %v", stored, err)
	}
}

// appendFrame appends a block-export frame holding the block with its refs
// known.
func appendFrame(t *testing.T, frame []byte, ref *block.BlockRef, data []byte, refs []*block.BlockRef) []byte {
	t.Helper()
	key := packfile.BlockKey(ref.GetHash())
	frame = binary.AppendUvarint(frame, uint64(len(key)))
	frame = append(frame, key...)
	frame = binary.AppendUvarint(frame, 1)
	object, err := block.EncodeBlockObject(data, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	frame = binary.AppendUvarint(frame, uint64(len(object)))
	return append(frame, object...)
}
