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
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/s4wave/spacewave/db/packfile"
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

// TestCommandRunThrough runs the command's passes on one real scratch file: an
// index, a pass that finds a hole, and a pass that fills it from a frame file.
func TestCommandRunThrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Fill the source over SFTP with a root whose child is missing.
	addr, hostKey := startSFTP(t)
	sourceDir := filepath.Join(dir, "source")
	root, child := []byte("root"), []byte("child")
	rootRef, childRef := buildRef(t, root), buildRef(t, child)
	client, err := block_store_sftp.NewClient(block_store_sftp.Config{Addr: addr, User: "user", Password: "pass", HostKey: hostKey, Conns: 1})
	if err != nil {
		t.Fatal(err)
	}
	seed := block_store_s3.NewPackStore(le, client, sourceDir, "world/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	entry := &block.PutBatchEntry{Ref: rootRef, Data: root, Refs: []*block.BlockRef{childRef}, RefsKnown: true}
	if _, err := seed.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	client.Close()

	// Point the command at the source and a bucket.
	bucket, objects := s3test.New(t)
	t.Setenv(sftpPasswordEnv, "pass")
	t.Setenv(keyIDEnv, "key")
	t.Setenv(keyEnv, "secret")
	holes := filepath.Join(dir, "holes.txt")
	args := &Args{
		Scratch:      filepath.Join(dir, "scratch"),
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

	// Index, then run a pass. The child is a hole, listed for block-export.
	if err := args.runIndex(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runCopy(nil); err != nil {
		t.Fatal(err)
	}
	key := hex.EncodeToString(packfile.BlockKey(childRef.GetHash()))
	list, err := os.ReadFile(holes)
	if err != nil {
		t.Fatal(err)
	}
	if string(list) != "hyperline-world "+key+" missing\n" {
		t.Fatalf("holes list: %q", list)
	}

	// Run a pass with the pulled block, and the list empties.
	frame := binary.AppendUvarint(nil, uint64(len(packfile.BlockKey(childRef.GetHash()))))
	frame = append(frame, packfile.BlockKey(childRef.GetHash())...)
	frame = binary.AppendUvarint(frame, 1)
	object, err := block.EncodeBlockObject(child, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	frame = binary.AppendUvarint(frame, uint64(len(object)))
	frame = append(frame, object...)
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

	// The clean store serves both blocks, and the graph stream sits beside it.
	target := block_store_s3.NewPackStore(le, objects, s3test.BucketName, "world/v2/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer target.Close()
	for _, ref := range []*block.BlockRef{rootRef, childRef} {
		stored, err := target.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil {
			t.Fatalf("read %s: %v, %v", ref.MarshalString(), stored, err)
		}
	}
	if len(bucket.Keys("world/v2/graph/")) == 0 {
		t.Fatal("no graph stream: " + strings.Join(bucket.Keys(""), " "))
	}
}
