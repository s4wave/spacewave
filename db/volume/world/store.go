package volume_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// worldStore uses the World transaction's writer lifetime for the entire KV
// operation, including reads that determine subsequent metadata writes.
type worldStore struct {
	engine world.Engine
	b      bus.Bus
	le     *logrus.Entry
	sfs    *block_transform.StepFactorySet
	conf   *Config
}

// NewTransaction opens the current object root while retaining its World writer.
func (s *worldStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	wtx, err := s.engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	t := &worldTransaction{worldTx: wtx, key: s.conf.GetObjectKey()}
	var success bool
	defer func() {
		if !success {
			t.Discard()
		}
	}()
	obj, found, err := wtx.GetObject(ctx, t.key)
	t.object = obj
	if err != nil {
		return nil, err
	}
	ref := s.conf.GetInitHeadRef().Clone()
	if found {
		ref, _, err = obj.GetRootRef(ctx)
		if err != nil {
			return nil, err
		}
	}
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	if bucketID := s.conf.GetBucketId(); bucketID != "" {
		ref.BucketId = bucketID
	}
	if s.conf.GetVolumeId() == "" || ref.GetBucketId() == "" {
		var base *bucket_lookup.Cursor
		base, err = wtx.BuildStorageCursor(ctx)
		if err == nil {
			t.cursor, err = base.FollowRef(ctx, ref)
			base.Release()
		}
	} else {
		t.cursor, err = bucket_lookup.BuildCursor(ctx, s.b, s.le, s.sfs, s.conf.GetVolumeId(), ref, nil)
	}
	if err != nil {
		return nil, err
	}
	var bcs *block.Cursor
	t.blocks, bcs = t.cursor.BuildTransaction(nil)
	t.Tx, err = kvtx_block.BuildKvTransaction(ctx, bcs, write && !wtx.GetReadOnly())
	if err != nil {
		return nil, err
	}
	success = true
	return t, nil
}

// worldTransaction owns the KV cursor and the enclosing World transaction.
// Like kvtx.Tx, its methods must be called serially.
type worldTransaction struct {
	kvtx.Tx
	worldTx   world.Tx
	object    world.ObjectState
	key       string
	cursor    *bucket_lookup.Cursor
	blocks    *block.Transaction
	discarded bool
}

// Commit publishes the KV root before committing the enclosing World head.
func (t *worldTransaction) Commit(ctx context.Context) error {
	if t.discarded {
		return tx.ErrDiscarded
	}
	defer t.Discard()
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	root, _, err := t.blocks.Write(ctx, true)
	if err != nil {
		return err
	}
	ref := t.cursor.GetRefWithOpArgs()
	ref.RootRef = root
	if t.object == nil {
		t.object, err = t.worldTx.CreateObject(ctx, t.key, ref)
	} else {
		_, err = t.object.SetRootRef(ctx, ref)
	}
	if err != nil {
		return err
	}
	return t.worldTx.Commit(ctx)
}

// Discard releases all resources, including the World's writer coordination.
func (t *worldTransaction) Discard() {
	if t.discarded {
		return
	}
	t.discarded = true
	if t.Tx != nil {
		t.Tx.Discard()
	}
	world.ReleaseObjectState(t.object)
	if t.cursor != nil {
		t.cursor.Release()
	}
	t.worldTx.Discard()
}

var _ kvtx.Store = (*worldStore)(nil)
