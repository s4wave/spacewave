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

// packLinger rotates a store's open pack once its first batch is this old, so
// a trickle of writes still finalizes into a pack that can upload.
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
	log, err := packahead.Open(ctx, dir, packahead.Options{Linger: packLinger})
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
