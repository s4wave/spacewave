package main

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestArchivePeerServesBlocks serves a stopped file holding a parent block
// with one child edge and an untracked leaf, and reads them from another
// peer through the block exchange: the parent with its child ref, the leaf
// with its refs unknown, a missing block as not found. The file is unchanged
// and a second archive refuses it while the first holds it.
func TestArchivePeerServesBlocks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())

	// Write the stopped file: a child, a parent that refs it, and a leaf the
	// graph does not track.
	dir := t.TempDir()
	file := filepath.Join(dir, "archive.s4wave")
	childRef, parentRef, leafRef := writeArchive(ctx, t, file)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	// Start the reading peer, which admits only the archive once it dials in.
	key := filepath.Join(dir, "archive.pem")
	archiveID, err := createKey(key)
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := transport.NewSessionTransport(le, parent, priv, "", "",
		transport.WithPeerAuthorizer(func(_ context.Context, id peer.ID) error {
			if id != archiveID {
				return errReadOnly
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = reader.Execute(ctx) }()
	if err := reader.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	addr, err := reader.StartUDP(ctx, transport.UDPConfig{
		ListenAddr: "127.0.0.1:0",
		Peers:      map[peer.ID]string{archiveID: ""},
		NamedOnly:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	conf := &dex_solicit.Config{BucketId: "p/local/acc/blk/so", ProtocolContext: []byte("so")}
	exchange, _, exRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx, reader.GetChildBus(), resolver.NewLoadControllerWithConfig(conf), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer exRef.Release()

	// Serve the file to the reading peer.
	args := &Args{
		File:            file,
		Key:             key,
		Listen:          "127.0.0.1:0",
		Peer:            reader.GetPeerID().String() + "@" + addr.String(),
		BucketID:        conf.GetBucketId(),
		ProtocolContext: "so",
	}
	serveCtx, stop := context.WithCancel(ctx)
	served := make(chan error, 1)
	go func() { served <- args.serve(serveCtx, le) }()

	// The parent reads with its child ref, the leaf with its refs unknown.
	store := dex_solicit.NewStore(exchange)
	got, err := store.GetStoredBlock(ctx, parentRef)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.Data) != "parent" || !got.RefsKnown || len(got.Refs) != 1 || !got.Refs[0].EqualsRef(childRef) {
		t.Fatalf("parent = %v; want its bytes and its child ref", got)
	}
	got, err = store.GetStoredBlock(ctx, leafRef)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.Data) != "leaf" || got.RefsKnown {
		t.Fatalf("leaf = %v; want its bytes with refs unknown", got)
	}
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetStoredBlock(ctx, missing); err != nil || got != nil {
		t.Fatalf("missing block = %v, %v; want not found", got, err)
	}

	// A second archive refuses the file the first holds.
	second := *args
	if err := second.serve(ctx, le); err == nil {
		t.Fatal("second archive opened a held file")
	}

	// Stop the archive; the file is unchanged.
	stop()
	<-served
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("serving changed the file")
	}
}

// writeArchive writes a child, a parent with an edge to it, and an untracked
// leaf to a new s4db file, and closes it.
func writeArchive(ctx context.Context, t *testing.T, file string) (child, parent, leaf *block.BlockRef) {
	t.Helper()
	db, err := s4db.Open(file, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	keys := store_kvkey.NewDefaultKVKey()
	put := func(data string) *block.BlockRef {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		key, err := ref.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), []byte(data)); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	child, parent, leaf = put("child"), put("parent"), put("leaf")
	graph := volume_kvtx.OpenRefGraph(kvtx.NewTxStore(tx))
	if err := graph.AddBlockRef(ctx, parent, child); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return child, parent, leaf
}
