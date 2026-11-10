package provider_local

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// nodeRedial is how long a lost connection to the storage node waits to dial
// again. Publications meanwhile commit on the local store alone.
const nodeRedial = 5 * time.Second

// nodeClearQueue bounds the reported packs waiting to clear from the upload
// queue, and so the memory their block hashes hold. A pack past it clears in
// its turn in the upload instead.
const nodeClearQueue = 32

// NodePushConfig sends the publications of an account's block stores to a
// storage node.
type NodePushConfig struct {
	// StoreID names the one block store to push: a node serves one connection,
	// so a second store, on any account, would keep replacing the first. The
	// node uploads to one backend, so the store must not move to another while
	// the push runs: a report in flight could clear a block the new backend
	// lacks, which only the old one would then hold.
	StoreID string
	// Dial connects to the node. It must return when ctx ends.
	Dial func(ctx context.Context) (net.Conn, error)
	// Wait is the least a commit waits for the node's flush. A slower local
	// commit gives the node all of its time.
	Wait time.Duration
	// ReadWait bounds one read from the node, before the backend answers it.
	ReadWait time.Duration
	// On is the runtime switch for the push, for clearing queued blocks the
	// node uploaded and for reading through the node. Off, a store commits,
	// uploads and reads as it would without a node, and still adopts what the
	// node already uploaded, which only adds blocks it can read. A nil On stays
	// off.
	On *atomic.Bool
	// Result reports each push, or is nil.
	Result block_store_nodepush.Result
	// NoPeers drops the store's peer reads, for a host with no peers to ask:
	// a block every other path misses is not found at once rather than
	// waiting for a peer session.
	NoPeers bool
}

// nodePushes maps the storage volume ID of an account to the NodePushConfig
// SetNodePush registered for it.
var nodePushes sync.Map

// SetNodePush registers conf for the account providerID/accountID. It must be
// called before the account starts. A nil conf removes it.
func SetNodePush(providerID, accountID string, conf *NodePushConfig) {
	id := StorageVolumeID(providerID, accountID)
	if conf == nil {
		nodePushes.Delete(id)
		return
	}
	nodePushes.Store(id, conf)
}

// nodeAdopter adopts reported packs into the store's open backend and hands
// their blocks to runNodeClears.
type nodeAdopter struct {
	t *bstoreTracker
	p *placementState
	// clears takes the block hashes of each adopted pack.
	clears chan<- []*hash.Hash
}

// AdoptPack registers the pack in the open backend. It answers without waiting
// for the clear, which waits behind foreground writers: one goroutine applies
// every report, and a report answered late is sent again. A pack already
// registered clears again, so a report repeated after a restart finishes.
func (a nodeAdopter) AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error {
	// A report that arrives before the backend opens is tried again.
	remote, _ := a.t.readBackend(a.p)
	if remote == nil {
		return errors.New("storage backend not open")
	}

	// Only a pack store adopts.
	adopter, ok := remote.(block_store_nodepush.Adopter)
	if !ok {
		return errors.New("storage backend adopts no packs")
	}
	if err := adopter.AdoptPack(ctx, entry, keys); err != nil {
		return err
	}

	// Name the pack's blocks for the clear. They stay queued for the upload
	// when the clear is too far behind to take them.
	hashes := make([]*hash.Hash, len(keys))
	for i, key := range keys {
		h, err := packfile.ParseBlockKey(key)
		if err != nil {
			return errors.Wrap(err, "parse block key")
		}
		hashes[i] = h
	}
	select {
	case a.clears <- hashes:
	default:
	}
	return nil
}

// runNodeClears clears each adopted pack's blocks from the upload queue, ahead
// of any backlog there, until ctx ends. A pack it misses, through a failure or
// a restart, leaves its entries for the upload, which finds them in the index.
func (t *bstoreTracker) runNodeClears(ctx context.Context, le *logrus.Entry, p *placementState, clears <-chan []*hash.Hash) {
	for {
		// Take the next pack's blocks.
		var hashes []*hash.Hash
		select {
		case <-ctx.Done():
			return
		case hashes = <-clears:
		}

		// Clear them against the backend open now.
		remote, _ := t.readBackend(p)
		if remote == nil {
			continue
		}
		if err := p.wb.ClearLocated(ctx, remote, hashes); err != nil && ctx.Err() == nil {
			le.WithError(err).Warn("unable to clear a reported pack from the upload queue")
		}
	}
}

// nodePushConfig returns this store's NodePushConfig, or nil.
func (t *bstoreTracker) nodePushConfig() *NodePushConfig {
	// Find the account's config.
	value, ok := nodePushes.Load(StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()))
	if !ok {
		return nil
	}

	// It applies only to the stores it names.
	conf := value.(*NodePushConfig)
	if conf.StoreID != t.id {
		return nil
	}
	return conf
}

// nodeReads returns this store's read cache on its storage node, or nil. A nil
// interface matters: a typed nil would look like a cache to the pack store.
func (t *bstoreTracker) nodeReads() block_store_s3.RangeCache {
	conf := t.nodePushConfig()
	if conf == nil || conf.On == nil {
		return nil
	}
	return block_store_nodepush.NewReads(conf.ReadWait, conf.On, &t.nodeClient)
}

// startNodePush starts pushing handle's publications when the account names
// this store, and returns the function that stops it.
func (t *bstoreTracker) startNodePush(
	ctx context.Context,
	le *logrus.Entry,
	wb *block_store_writeback.Store,
	placement *placementState,
	handle *BlockStore,
) func() {
	// Push only a store the config names.
	conf := t.nodePushConfig()
	if conf == nil || conf.On == nil {
		return func() {}
	}

	// Build the client and put the publisher in front of the local store.
	clears := make(chan []*hash.Hash, nodeClearQueue)
	client := block_store_nodepush.NewClient(le.WithField("routine", "node-push"), block_store_nodepush.NewPackApplier(nodeAdopter{t: t, p: placement, clears: clears}))
	handle.push = block_store_nodepush.NewPublisher(client, conf.Wait, conf.On, conf.Result)
	t.nodeClient.Store(client)
	le.Info("pushing publications to the storage node")

	// Keep the connection and the clears up until the store unmounts. The
	// unbounded queue starts each routine at once and reports idle when every
	// one returned.
	runCtx, cancel := context.WithCancel(ctx)
	running := conc.NewConcurrentQueue(0)
	running.Enqueue(func() { _ = client.Run(runCtx, conf.Dial, nodeRedial) })
	running.Enqueue(func() { t.runNodeClears(runCtx, le, placement, clears) })
	return func() {
		t.nodeClient.Store(nil)
		cancel()
		_ = running.WaitIdle(context.Background(), nil)
		handle.push.Close()
	}
}

// startNodeReads connects the store to its storage node for reads alone, in
// serve mode: it pushes nothing, applies no reported pack and drains nothing.
// The pack store then reads through the node's range cache as nodeReads
// describes. The release disconnects it.
func (t *bstoreTracker) startNodeReads(ctx context.Context, le *logrus.Entry) func() {
	// Read only a store the config names.
	conf := t.nodePushConfig()
	if conf == nil || conf.On == nil {
		return func() {}
	}

	// Run a client without an applier until the store unmounts.
	client := block_store_nodepush.NewClient(le.WithField("routine", "node-reads"), nil)
	runCtx, cancel := context.WithCancel(ctx)
	running := conc.NewConcurrentQueue(0)
	running.Enqueue(func() { _ = client.Run(runCtx, conf.Dial, nodeRedial) })
	t.nodeClient.Store(client)
	le.Info("reading the storage node, serve mode pushes nothing")
	return func() {
		t.nodeClient.Store(nil)
		cancel()
		_ = running.WaitIdle(context.Background(), nil)
	}
}

// _ are type assertions: a remote that stopped satisfying these would turn the
// node's uploads off with no error.
var (
	_ block_store_writeback.BlockLocator = (*block_store_s3.PackStore)(nil)
	_ block_store_nodepush.Adopter       = (*block_store_s3.PackStore)(nil)
)
