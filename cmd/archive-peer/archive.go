package main

import (
	"context"
	"errors"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// errReadOnly rejects every write to the archive.
var errReadOnly = errors.New("archive peer is read-only")

// staticAccess grants a request for its whole lifetime: the served peer is
// fixed when the daemon starts.
type staticAccess struct{}

// Check admits the request.
func (staticAccess) Check() error { return nil }

// WaitRevoked waits for ctx: the grant is never revoked.
func (staticAccess) WaitRevoked(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Release does nothing.
func (staticAccess) Release() {}

// archive resolves the bucket lookup for one bucket from the file, and is the
// lookup and its handle.
type archive struct {
	le       *logrus.Entry
	db       *s4db.DB
	keys     *store_kvkey.KVKey
	bucketID string
}

// GetControllerInfo returns the controller info.
func (a *archive) GetControllerInfo() *controller.Info {
	return controller.NewInfo("archive-peer/lookup", controller.MustParseVersion("0.0.1"), "serves a stopped s4db file's blocks")
}

// Execute does nothing: the archive answers directives.
func (a *archive) Execute(ctx context.Context) error { return nil }

// HandleDirective resolves the bucket lookup for the served bucket.
func (a *archive) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(bucket_lookup.BuildBucketLookup)
	if !ok || dir.BuildBucketLookupBucketID() != a.bucketID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bucket_lookup.BuildBucketLookupValue{a}), nil)
}

// Close does nothing: the caller closes the file.
func (a *archive) Close() error { return nil }

// GetDisposed reports false: the handle lasts the process.
func (a *archive) GetDisposed() bool { return false }

// GetBucketConfig returns the served bucket's id.
func (a *archive) GetBucketConfig() *bucket.Config { return &bucket.Config{Id: a.bucketID} }

// GetLookup returns the archive.
func (a *archive) GetLookup(context.Context) (bucket_lookup.Lookup, error) { return a, nil }

// BeginReadOperation returns the archive: each lookup opens its own
// transaction.
func (a *archive) BeginReadOperation(context.Context) (bucket_lookup.Lookup, func(), error) {
	return a, func() {}, nil
}

// LookupBlock reads the block bytes as LookupStoredBlock does.
func (a *archive) LookupBlock(ctx context.Context, ref *block.BlockRef, _ ...bucket_lookup.LookupBlockOption) ([]byte, bool, error) {
	stored, err := a.LookupStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// LookupStoredBlock reads the block and its outgoing block refs from the
// reference graph in one read-only transaction. RefsKnown is set when the
// graph tracked the block or it has children, so an untracked block without
// children is not reported as a leaf.
func (a *archive) LookupStoredBlock(ctx context.Context, ref *block.BlockRef, _ ...bucket_lookup.LookupBlockOption) (*block.StoredBlock, error) {
	// Open a read-only transaction for the block and its edges.
	le := a.le.WithField("ref", ref.MarshalString())
	tx, err := a.db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read the block bytes, copied out of the transaction, and check them
	// against the ref.
	key, err := ref.MarshalVT()
	if err != nil {
		return nil, err
	}
	data, ok, err := tx.Get(ctx, a.keys.GetBlockKey(key))
	if err != nil {
		le.WithError(err).Warn("read block")
		return nil, err
	}
	if !ok {
		le.Info("block not in the archive")
		return nil, nil
	}
	data = slices.Clone(data)
	if err := ref.VerifyData(data, false); err != nil {
		le.WithError(err).Warn("archive block fails its hash")
		return nil, nil
	}

	// Read its outgoing block refs from the reference graph.
	graph := volume_kvtx.OpenRefGraph(kvtx.NewTxStore(tx))
	targets, tracked, err := graph.GetBlockEdges(ctx, block_gc.BlockIRI(ref))
	if err != nil {
		le.WithError(err).Warn("read block refs; serving the bytes without them")
		return &block.StoredBlock{Data: data}, nil
	}

	// Keep the edges that name blocks, once each.
	slices.Sort(targets)
	targets = slices.Compact(targets)
	var refs []*block.BlockRef
	for _, target := range targets {
		if child, ok := block_gc.ParseBlockIRI(target); ok {
			refs = append(refs, child)
		}
	}

	// Report the refs known when the graph tracked the block or it has children.
	known := tracked || len(refs) != 0
	le.WithField("bytes", len(data)).WithField("refs", len(refs)).WithField("refs-known", known).Info("served block")
	return &block.StoredBlock{Data: data, Refs: refs, RefsKnown: known}, nil
}

// LookupBlockExistsBatch reports which blocks the file holds, in one
// read-only transaction.
func (a *archive) LookupBlockExistsBatch(ctx context.Context, refs []*block.BlockRef, _ ...bucket_lookup.LookupBlockOption) ([]bool, error) {
	// Open a read-only transaction for the whole batch.
	tx, err := a.db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Check each block key.
	out := make([]bool, len(refs))
	for i, ref := range refs {
		key, err := ref.MarshalVT()
		if err != nil {
			return nil, err
		}
		if out[i], err = tx.Exists(ctx, a.keys.GetBlockKey(key)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// PutBlock refuses the write: the archive is read-only.
func (a *archive) PutBlock(context.Context, []byte, *block.PutOpts) ([]*bucket.ObjectRef, bool, error) {
	return nil, false, errReadOnly
}

// _ is a type assertion
var (
	_ controller.Controller = (*archive)(nil)
	_ bucket_lookup.Lookup  = (*archive)(nil)
	_ bucket_lookup.Handle  = (*archive)(nil)
)
