package provider_local

import (
	"context"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// nodeRedial is how long a lost connection to the storage node waits to dial
// again. Publications meanwhile commit on the local store alone.
const nodeRedial = 5 * time.Second

// NodePushConfig sends the publications of an account's block stores to a
// storage node.
type NodePushConfig struct {
	// StoreIDs lists the block stores to push.
	StoreIDs []string
	// Dial connects to the node.
	Dial func(ctx context.Context) (net.Conn, error)
	// Wait is the least a commit waits for the node's flush. A slower local
	// commit gives the node all of its time.
	Wait time.Duration
	// On is the runtime switch for the push and for clearing queued blocks the
	// node uploaded. Off, a store commits and uploads as it would without a
	// node.
	On *atomic.Bool
	// Result reports each push, or is nil.
	Result block_store_nodepush.Result
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

// nodeAdopter adopts reported packs into the store's open backend.
type nodeAdopter struct {
	t *bstoreTracker
	p *placementState
}

// AdoptPack registers the pack in the open backend.
func (a nodeAdopter) AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error {
	// A report that arrives before the backend opens is tried again.
	remote, _ := a.t.readBackend(a.p)
	if remote == nil {
		return errors.New("storage backend not open")
	}
	adopter, ok := remote.(block_store_nodepush.Adopter)
	if !ok {
		return errors.New("storage backend adopts no packs")
	}
	return adopter.AdoptPack(ctx, entry, keys)
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
	value, ok := nodePushes.Load(StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()))
	if !ok {
		return func() {}
	}
	conf := value.(*NodePushConfig)
	if !slices.Contains(conf.StoreIDs, t.id) {
		return func() {}
	}

	// Build the client and put the publisher in front of the local store.
	client := block_store_nodepush.NewClient(le.WithField("routine", "node-push"), block_store_nodepush.NewPackApplier(nodeAdopter{t: t, p: placement}))
	handle.push = block_store_nodepush.NewPublisher(client, conf.Wait, conf.On, conf.Result)
	wb.SetLocateUploaded(conf.On)
	le.Info("pushing publications to the storage node")

	// Keep the connection up until the store unmounts.
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.Run(runCtx, conf.Dial, nodeRedial)
	}()
	return func() {
		cancel()
		<-done
		handle.push.Close()
	}
}

// _ are type assertions: a remote that stopped satisfying these would turn the
// node's uploads off with no error.
var (
	_ block_store_writeback.BlockLocator = (*block_store_s3.PackStore)(nil)
	_ block_store_nodepush.Adopter       = (*block_store_s3.PackStore)(nil)
)
