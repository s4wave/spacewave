// Package cursorcodec opens Space World cursors with authenticated transforms
// and one shared decoded-block cache. It binds implicit references to the
// supplied bucket; callers retain cursor release and linked-file ownership.
package cursorcodec

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/util/blockenc"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// Factory owns cursor construction and its shared decode cache. Its settings
// are immutable; the cache and last page transformer synchronize their access.
// A zero Factory has neither an active decoder nor page accounting.
type Factory struct {
	// le logs transform and cursor operations.
	le *logrus.Entry
	// bus resolves linked bucket references.
	bus bus.Bus
	// steps constructs transform steps.
	steps *block_transform.StepFactorySet
	// decoded shares decoded blocks between every opened cursor.
	decoded *block.DecodedBlockCache
	// countPages enables stored-page accounting for this factory.
	countPages bool
	// mtx guards xfrm, as cursor opens and background stores may overlap.
	mtx sync.Mutex
	// xfrm decodes pages with the last opened World cursor's transform.
	xfrm block.Transformer
}

// New shares decoded or allocates a cache, and selects stored-page accounting.
func New(le *logrus.Entry, b bus.Bus, steps *block_transform.StepFactorySet, decoded *block.DecodedBlockCache, countPages bool) Factory {
	if decoded == nil {
		decoded = block.NewDecodedBlockCache()
	}
	return Factory{le: le, bus: b, steps: steps, decoded: decoded, countPages: countPages}
}

// Open constructs a cursor over store at head, resolving implicit references
// within bucketID. The caller releases the returned cursor.
func (f *Factory) Open(ctx context.Context, store block.StoreOps, bucketID string, head *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
	// Require the head's transform before opening its pages.
	conf := head.GetTransformConf()
	if len(conf.GetSteps()) == 0 {
		return nil, sobject.ErrEmptyTransformConfig
	}
	xfrm, err := f.Transformer(conf)
	if err != nil {
		return nil, err
	}

	// Bind references to the mounted bucket and share decoded pages.
	ref := Bind(head, bucketID)
	cursor := bucket_lookup.NewCursorWithRelease(ctx, f.bus, f.le, f.steps, store, xfrm, ref,
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: bucketID}, conf, nil, bucketID,
	)
	cursor.SetDecodedBlockCache(f.decoded)

	// Keep page accounting on the same transform as the opened cursors.
	f.mtx.Lock()
	f.xfrm = xfrm
	f.mtx.Unlock()
	return cursor, nil
}

// Transformer constructs the authenticated transform used by World pages.
func (f *Factory) Transformer(conf *block_transform.Config) (*block_transform.Transformer, error) {
	if err := ValidateTransform(conf); err != nil {
		return nil, err
	}
	return block_transform.NewTransformer(controller.ConstructOpts{Logger: f.le}, f.steps, conf)
}

// errUnauthenticatedTransform refuses a World transform without authentication.
var errUnauthenticatedTransform = errors.New("world transform requires authenticated encryption")

// ValidateTransform requires a valid authenticated encryption step in conf.
func ValidateTransform(conf *block_transform.Config) error {
	for _, step := range conf.GetSteps() {
		// Read each block encryption step.
		if step.GetId() != transform_blockenc.ConfigID {
			continue
		}
		encConf := &transform_blockenc.Config{}
		if err := block_transform.UnmarshalStepConfig(step.GetConfig(), encConf); err != nil {
			return errors.Wrap(err, "world block encryption config")
		}

		// Accept the first authenticated encryption step.
		switch encConf.GetBlockEnc() {
		case blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			blockenc.BlockEnc_BlockEnc_SECRET_BOX,
			blockenc.BlockEnc_BlockEnc_AES_256_GCM:
			if err := encConf.Validate(); err != nil {
				return errors.Wrap(err, "world block encryption config")
			}
			return nil
		}
	}
	return errUnauthenticatedTransform
}

// Bind copies head and resolves its implicit bucket to bucketID.
func Bind(head *bucket.ObjectRef, bucketID string) *bucket.ObjectRef {
	ref := head.CloneVT()
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	ref.BucketId = bucketID
	return ref
}

// CountStoredPages reports buffered pages the next flush or root move stores,
// by tree and level. Stored pages end the walk unread. Call before storage;
// overlapping stores may count a page twice, and stage writes are not counted.
// Cost per store: one World decode per new root, and one kept-block lookup per
// new page and per ref it holds.
func (f *Factory) CountStoredPages(ctx context.Context, pages *block.PendingStore, roots ...*block.BlockRef) {
	// Capture the current decoder without holding its lock during the walk.
	f.mtx.Lock()
	xfrm := f.xfrm
	f.mtx.Unlock()
	if !f.countPages || len(roots) == 0 || xfrm == nil {
		return
	}

	// Report accounting failures without changing the storage outcome.
	err := world_block.WalkUnflushedPages(ctx, pages, xfrm, roots, world_block.StoredPages.Add)
	if err != nil {
		f.le.WithError(err).Debug("cannot count stored world pages")
	}
}
