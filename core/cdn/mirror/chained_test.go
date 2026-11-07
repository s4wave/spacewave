package cdn_mirror_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_mirror "github.com/s4wave/spacewave/core/cdn/mirror"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_checkout "github.com/s4wave/spacewave/db/unixfs/checkout"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// link is an in-memory connection to a block RPC server that the test can
// drop. While it is down, opening a call waits for it to come back, as a call
// over a peer link waits for the next link.
type link struct {
	srv *srpc.Server

	mtx sync.Mutex
	// up is closed while the link is up.
	up chan struct{}
	// conns are the open calls' pipe ends.
	conns []net.Conn
}

// newLink returns a link up to srv.
func newLink(srv *srpc.Server) *link {
	up := make(chan struct{})
	close(up)
	return &link{srv: srv, up: up}
}

// open opens a call on the current link.
func (l *link) open(ctx context.Context, msg srpc.PacketDataHandler, closed srpc.CloseHandler) (srpc.PacketWriter, error) {
	// Wait for the link to be up.
	l.mtx.Lock()
	up := l.up
	l.mtx.Unlock()
	select {
	case <-up:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}

	// Serve the call over a pipe the next drop closes.
	srvPipe, cliPipe := net.Pipe()
	l.mtx.Lock()
	l.conns = append(l.conns, srvPipe, cliPipe)
	l.mtx.Unlock()
	go l.srv.HandleStream(ctx, srvPipe)
	prw := srpc.NewPacketReadWriter(cliPipe)
	go prw.ReadPump(msg, closed)
	return prw, nil
}

// drop closes every open call and holds new ones for down.
func (l *link) drop(down time.Duration) {
	l.mtx.Lock()
	for _, c := range l.conns {
		_ = c.Close()
	}
	l.conns = nil
	up := make(chan struct{})
	l.up = up
	l.mtx.Unlock()
	time.AfterFunc(down, func() { close(up) })
}

// dropOnRead drops the link while the server answers every nth read.
type dropOnRead struct {
	block.StoreOps
	link  *link
	n     int64
	reads atomic.Int64
}

// GetBlock counts the read and drops the link on every nth.
func (s *dropOnRead) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.reads.Add(1)%s.n == 0 {
		s.link.drop(100 * time.Millisecond)
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock counts the read and drops the link on every nth.
func (s *dropOnRead) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if s.reads.Add(1)%s.n == 0 {
		s.link.drop(100 * time.Millisecond)
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// TestChainedCheckoutSurvivesReconnect mirrors one tree to the CDN Space,
// adds a second the Space lacks, and checks out both through a store reading
// the Space first and the World's server second. The link to the server drops
// again and again mid-checkout, and the checkout finishes on the next link.
func TestChainedCheckoutSurvivesReconnect(t *testing.T) {
	// Mirror the first tree, then add a second only the server holds.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	r := writertest.NewRig(t)
	tr := newTree(t, 12)
	if _, err := cdn_mirror.MirrorWorld(ctx, tr.world.Engine, options(r, 4)); err != nil {
		t.Fatal(err)
	}
	newer := randomFiles(t, 12, "new")
	writeObject(t, tr.world, "newer", newer)
	head, _ := tr.head(t)

	// Serve the World's blocks over a link that drops on every seventh read.
	local := make(chan block.StoreOps)
	go func() {
		_ = tr.world.Engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			local <- cursor.GetBlockStore()
			<-ctx.Done()
			return nil
		})
	}()
	served := &dropOnRead{StoreOps: <-local, n: 7}
	mux := srpc.NewMux()
	if err := block_rpc.SRPCRegisterBlockStore(mux, block_rpc_server.NewBlockStore(served)); err != nil {
		t.Fatal(err)
	}
	served.link = newLink(srpc.NewServer(mux))
	rpcStore := block_rpc_client.NewBlockStore(block_rpc.NewSRPCBlockStoreClient(srpc.NewClient(served.link.open)), served.GetHashType(), true)

	// Read the Space first and the server second.
	cdnStore, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL:         r.PackBase(),
		RootPointerBaseURL: r.RootsURL,
		SpaceID:            writertest.Space,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cdnStore.Close()
	chained := cdn_bstore.NewChainedStore(ctx, le, cdnStore, rpcStore)

	// Open the newer head over the chained store.
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, chained, block_transform.NewTransformerWithSteps(nil), head, &bucket.BucketOpArgs{BucketId: head.GetBucketId()}, nil)
	defer cursor.Release()
	eng, err := world_block.NewEngine(ctx, le, cursor, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(eng, false)

	// Check out both trees; each file matches its source.
	for key, files := range map[string]fstest.MapFS{fsKey: tr.files, "newer": newer} {
		checkout(t, ctx, le, ws, key, files)
	}
	if n := served.reads.Load(); n < 4*served.n {
		t.Fatalf("the server answered %d reads, too few to drop the link 4 times", n)
	}
}

// checkout checks out the UnixFS object at key to a new directory and
// requires each of files there.
func checkout(t *testing.T, ctx context.Context, le *logrus.Entry, ws world.WorldState, key string, files fstest.MapFS) {
	// Check the object out to disk.
	fsc := unixfs_world.NewFSCursor(le, ws, key, unixfs_world.FSType_FSType_FS_NODE, nil, false)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		fsc.Release()
		t.Fatal(err)
	}
	defer fsh.Release()
	dir := t.TempDir()
	if err := unixfs_checkout.Checkout(ctx, dir, fsh, nil); err != nil {
		t.Fatalf("check out %s: %v", key, err)
	}

	// Require every file as written.
	for name, file := range files {
		if file.Mode.IsDir() {
			continue
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, file.Data) {
			t.Fatalf("%s/%s differs from its source", key, name)
		}
	}
}
