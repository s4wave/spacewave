package provider_local

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	// nodeRedial is how long a lost connection to the storage node waits to
	// dial again, and how long the drain waits after a failed push.
	// Publications meanwhile commit on the local store alone.
	nodeRedial = 5 * time.Second
	// nodeDrainBatchBytes bounds one push of the drain. A drain push shares
	// the connection with the commits' pushes, so a commit's frame waits
	// behind at most one of these.
	nodeDrainBatchBytes = 1 << 20
	// nodeDrainWait bounds one push of the drain. A push the node does not
	// flush in time is tried again, after nodeRedial.
	nodeDrainWait = 30 * time.Second
)

// errNodePushOff is a drain push refused while the node's switch is off. The
// drain keeps the batch and tries it again once the switch is back on.
var errNodePushOff = errors.New("storage node push off")

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
	// WaitLive, when set and positive, replaces Wait, in nanoseconds, read
	// at each push, so a tunable changes it live.
	WaitLive *atomic.Int64
	// ReadWait bounds one read from the node, before the backend answers it.
	ReadWait time.Duration
	// On is the runtime switch for the push, for clearing queued blocks the
	// node uploaded and for reading through the node. Off, a store commits,
	// uploads and reads as it would without a node, and still adopts what the
	// node already uploaded, which only adds blocks it can read. A nil On stays
	// off.
	On *atomic.Bool
	// ReadsOn turns reads through the node off alone, leaving the push on;
	// reads reach the node only while On and ReadsOn are both set. Nil reads
	// whenever On is set.
	ReadsOn *atomic.Bool
	// Result reports each push, or is nil.
	Result block_store_nodepush.Result
	// Drain pushes the upload backlog through the node, or is nil for no
	// drain.
	Drain *NodeDrainConfig
	// Client is the connection to the node, which the caller runs and
	// shares, such as with the journal of the store's Volume; nil has the
	// store dial and run its own through Dial.
	Client *block_store_nodepush.Client
	// Durable, set with On, keeps pushed blocks off the local disk: in a
	// memory set of HeldBytes until the node reports them uploaded, as
	// block_store_nodepush.Publisher.SetDurable describes. Off, the memory
	// set spills to the local disk with queue entries. Nil never does.
	Durable *atomic.Bool
	// HeldBytes bounds the memory set; a publication past it takes the local
	// path. A tunable changes it live through the store's Held.
	HeldBytes *atomic.Int64
}

// NodeDrainConfig drains a store's upload backlog through its storage node, as
// block_store_writeback.Drain describes: the store reads its queued blocks and
// pushes them, the node uploads them and reports each pack, and the report
// clears them. The upload still runs beside it.
type NodeDrainConfig struct {
	// Admit paces the drain and holds it while it is switched off. It runs
	// before every batch, so a drain held from the start reads nothing.
	// Required.
	Admit block_store_writeback.UploadAdmit
	// MinAge leaves out the blocks written this recently, which their own
	// push may have sent already.
	MinAge time.Duration
	// Regrow starts another pass once the queued bytes grow this far after
	// one ends, such as after the node was away. Zero runs one pass.
	Regrow int64
	// Report receives the drain's progress, or is nil.
	Report func(block_store_writeback.DrainReport)
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

// localPublisher is a BlockStore's publisher without its node push.
type localPublisher struct {
	*BlockStore
}

// SubmitAtomic admits the publication to the local store.
func (l localPublisher) SubmitAtomic(ctx context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	return l.submitLocal(ctx, p)
}

// nodeAdopter adopts reported packs into the store's open backend and clears
// their blocks from the upload queue.
type nodeAdopter struct {
	t *bstoreTracker
	p *placementState
}

// AdoptPack registers the pack in the open backend, then clears its blocks
// from the upload queue, and returns only once both are done. The node stops
// reporting a pack once it hears it applied, so the answer must wait for the
// clear: a clear dropped behind an answered report leaves its blocks queued,
// unevictable, until the upload reaches them in its turn. Until it returns
// the report stays unanswered on the node's disk, which sends it again, so
// that report is the clear's durable record. A pack already registered clears
// again, so a report repeated after a restart finishes.
//
// The clear keeps two invariants. A queue entry goes only after the pack's
// index entry is stored, which the adopter's AdoptPack does before this
// clears. And ClearLocated drops only the entries the remote's index locates,
// so a key the pack does not hold stays queued.
//
// One goroutine applies the reports, so one pack's hashes are held at a time.
// The clear waits for the count of the upload queue after an open, which can
// take most of an hour on a large queue; meanwhile this report waits, the
// node tries the others in turn and backs off, and the connection's read
// loop, which carries every commit's answers, does not wait on it. The clear
// itself runs behind foreground writers, as ClearLocated describes.
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

	// Clear the pack's blocks against the backend that indexed them.
	hashes := make([]*hash.Hash, len(keys))
	for i, key := range keys {
		h, err := packfile.ParseBlockKey(key)
		if err != nil {
			return errors.Wrap(err, "parse block key")
		}
		hashes[i] = h
	}

	// The index finds the blocks on the remote now: their memory copies can
	// go before the clear, which may wait out the queue's count.
	if held := a.t.held.Load(); held != nil {
		held.Drop(keys)
	}
	return a.p.wb.ClearLocated(ctx, remote, hashes)
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
	return block_store_nodepush.NewReads(conf.ReadWait, conf.On, conf.ReadsOn, &t.nodeClient)
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

	// Take the shared client, or build one, and put the publisher in front of
	// the local store.
	applier := block_store_nodepush.NewPackApplier(nodeAdopter{t: t, p: placement})
	client := conf.Client
	if client != nil {
		client.SetApplier(applier)
	} else {
		client = block_store_nodepush.NewClient(le.WithField("routine", "node-push"), applier)
	}
	handle.push = block_store_nodepush.NewPublisher(localPublisher{handle}, client, conf.Wait, conf.On, conf.Result)
	handle.push.SetWaitLive(conf.WaitLive)
	wb.SetLocateUploaded(conf.On)
	t.nodeClient.Store(client)
	le.Info("pushing publications to the storage node")

	// Keep the connection up until the store unmounts, unless the caller runs
	// it, and drain beside it.
	runCtx, cancel := context.WithCancel(ctx)
	var running sync.WaitGroup
	if conf.Client == nil {
		running.Go(func() { _ = client.Run(runCtx, conf.Dial, nodeRedial) })
	}

	// Keep pushed blocks in memory while durable, spilling what waits too
	// long or what the switch turned back.
	if conf.Durable != nil && conf.HeldBytes != nil {
		held := block_store_nodepush.NewHeld(conf.HeldBytes)
		handle.push.SetDurable(conf.Durable, held, client.Connected)
		t.held.Store(held)
		heldSpills.Store(StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()), func(ctx context.Context) error { return spillAll(ctx, held, wb) })
		running.Go(func() { runSpill(runCtx, le, held, wb, conf.Durable, client.Connected) })
	}
	if conf.Drain != nil && conf.Drain.Admit != nil {
		drain := wb.NewDrain(func(ctx context.Context, entries []*block.PutBatchEntry) error {
			if !conf.On.Load() {
				return errNodePushOff
			}
			ctx, cancel := context.WithTimeout(ctx, nodeDrainWait)
			defer cancel()
			return client.Send(ctx, entries)
		}, block_store_writeback.DrainOptions{
			MinAge:     conf.Drain.MinAge,
			BatchBytes: nodeDrainBatchBytes,
			Regrow:     conf.Drain.Regrow,
			Admit:      conf.Drain.Admit,
			Report:     conf.Drain.Report,
		})
		running.Go(func() { t.runNodeDrain(runCtx, le, placement, drain) })
	}
	return func() {
		// Stop the push and the routines, then spill what memory holds.
		t.nodeClient.Store(nil)
		cancel()
		running.Wait()
		handle.push.Close()
		if conf.Client != nil {
			client.SetApplier(nil)
		}
		t.spillOnClose(le, wb)
	}
}

// spillOnClose writes every block still held to the local disk as the store
// unmounts, so a clean stop leaves none only in the node's log.
func (t *bstoreTracker) spillOnClose(le *logrus.Entry, wb *block_store_writeback.Store) {
	held := t.held.Swap(nil)
	if held == nil {
		return
	}
	heldSpills.Delete(StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()))
	if err := spillAll(context.Background(), held, wb); err != nil {
		le.WithError(err).Warn("unable to spill held blocks as the store unmounts; the storage node holds them")
	}
}

// runNodeDrain runs the drain against the open backend until ctx ends, or its
// one pass ends with Regrow zero. A failed step, such as a push while the node
// is away, waits nodeRedial and resumes where it stopped; meanwhile the upload
// carries the queue alone.
func (t *bstoreTracker) runNodeDrain(ctx context.Context, le *logrus.Entry, placement *placementState, drain *block_store_writeback.Drain) {
	for {
		// Wait for the backend, whose index says what needs no push.
		remote, changed := t.readBackend(placement)
		if remote == nil {
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
			continue
		}

		// Drain until the pass ends, or pause after a failure.
		err := drain.Run(ctx, remote)
		if err == nil || ctx.Err() != nil {
			return
		}
		le.WithError(err).Debug("drain through the storage node paused")
		select {
		case <-ctx.Done():
			return
		case <-time.After(nodeRedial):
		}
	}
}

// _ are type assertions: a remote that stopped satisfying these would turn the
// node's uploads off with no error.
var (
	_ block_store_writeback.BlockLocator = (*block_store_s3.PackStore)(nil)
	_ block_store_nodepush.Adopter       = (*block_store_s3.PackStore)(nil)
)
