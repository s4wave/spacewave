package provider_local

import (
	"context"
	"net/url"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// packLinger is the default PackaheadConfig.Linger.
const packLinger = 30 * time.Second

// PackaheadConfig keeps the new values of an account's block stores in
// packahead logs, one per store, whose finalized packs upload to the store's
// backend as they are.
type PackaheadConfig struct {
	// Dir holds each store's log in a subdirectory named by the store id.
	Dir string
	// DivertStoreIDs lists the stores whose new values append to their log,
	// landing only their child lists in the Volume. A store absent from the
	// list writes to the Volume, and keeps reading and uploading the packs
	// its log holds: the switch back.
	DivertStoreIDs []string
	// KeepBytes is the size of uploaded packs each store keeps local as a
	// read cache. Beyond it the oldest uploaded packs are deleted, and their
	// blocks read from the backend. Zero keeps every pack.
	KeepBytes int64
	// Linger rotates a store's open pack once its first batch is this old, so
	// a trickle of writes still finalizes into a pack that can upload. Zero
	// uses 30 seconds.
	Linger time.Duration
	// ReserveBytes is the free disk each store's log leaves: a write that
	// would leave less fails with packahead.ErrReserve and stores nothing, so
	// the logs never take the space the Volume needs. Zero keeps none.
	ReserveBytes int64
}

// packaheads maps the storage volume ID of an account to the
// PackaheadConfig SetPackahead registered for it.
var packaheads sync.Map

// SetPackahead keeps the new values of the block stores of the account
// providerID/accountID in packahead logs under conf. It must be called before
// the account starts. A nil conf removes it, and a store mounted after that
// opens no log: a log that holds packs not yet uploaded needs its config to
// stay, so they upload.
func SetPackahead(providerID, accountID string, conf *PackaheadConfig) {
	id := StorageVolumeID(providerID, accountID)
	if conf == nil {
		packaheads.Delete(id)
		return
	}
	packaheads.Store(id, conf)
}

// storePacks is a block store's open packahead log.
type storePacks struct {
	// log holds the store's packed values.
	log *packahead.Log
	// packed reads the log before the Volume's bucket, and takes the store's
	// writes, diverting them to the log when the config lists the store.
	packed *block_store_writeback.PackedStore
	// drain uploads the log's finalized packs to the store's backend.
	drain *packahead.Drain
}

// openPacks opens the store's packahead log over local, its bucket on the
// Volume, when its account registered a PackaheadConfig, or returns nil.
// Close the log when the store unmounts.
func (t *bstoreTracker) openPacks(ctx context.Context, le *logrus.Entry, local block.StoreOps) (*storePacks, error) {
	// Open no log without a config.
	value, ok := packaheads.Load(StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()))
	if !ok {
		return nil, nil
	}
	conf := value.(*PackaheadConfig)

	// Open the store's log in its own directory.
	dir := filepath.Join(conf.Dir, url.PathEscape(t.id))
	linger := conf.Linger
	if linger <= 0 {
		linger = packLinger
	}
	log, err := packahead.Open(ctx, dir, packahead.Options{Linger: linger, ReserveBytes: conf.ReserveBytes})
	if err != nil {
		return nil, errors.Wrap(err, "open packahead")
	}

	// Divert a listed store, and upload the packs paced as the queued writes
	// are.
	divert := slices.Contains(conf.DivertStoreIDs, t.id)
	le.WithField("divert", divert).WithField("pack-dir", dir).Info("opened packahead")
	return &storePacks{
		log:    log,
		packed: block_store_writeback.NewPackedStore(log, local, divert),
		drain:  packahead.NewDrain(log, packahead.DrainOptions{KeepBytes: conf.KeepBytes, Admit: t.admitUpload}),
	}, nil
}

// admitUpload paces one upload by the account's upload admit, when it has
// one.
func (t *bstoreTracker) admitUpload(ctx context.Context, lastBytes int64) error {
	if fn := t.a.uploadAdmit.Load(); fn != nil {
		return (*fn)(ctx, lastBytes)
	}
	return nil
}

// PackaheadStats is a block store's packahead log: whether it diverts new
// values, its drain's finalized packs and counts, and its write counts. The
// counts restart with each mount.
type PackaheadStats struct {
	// Divert is set while the store diverts new values to the log.
	Divert bool
	// DrainStatus counts the log's finalized packs and the drain's uploads.
	packahead.DrainStatus
	// PackedStats counts the writes the log took.
	block_store_writeback.PackedStats
}

// GetPackaheadStats returns the store's packahead log counts, or false when
// the store keeps no log.
func (b *BlockStore) GetPackaheadStats() (PackaheadStats, bool) {
	p := b.placement
	if p == nil || p.drain == nil || p.packed == nil {
		return PackaheadStats{}, false
	}
	drain, _ := p.drain.Status()
	return PackaheadStats{Divert: p.packed.GetDivert(), DrainStatus: drain, PackedStats: p.packed.GetStats()}, true
}
