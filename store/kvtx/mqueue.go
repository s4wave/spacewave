package store_kvtx

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_mqueue "github.com/aperturerobotics/hydra/kvtx/mqueue"
	kvtx_prefixer "github.com/aperturerobotics/hydra/kvtx/prefixer"
	"github.com/aperturerobotics/hydra/mqueue"
	mqueue_store "github.com/aperturerobotics/hydra/mqueue/store"
	"github.com/emirpasic/gods/sets/treeset"
)

// ListMessageQueues lists message queues with a given ID prefix.
//
// Note: if !filled, implementation might not return queues that are empty.
// If filled is set, implementation must only return filled queues.
func (k *KVTx) ListMessageQueues(ctx context.Context, prefix []byte, filled bool) ([][]byte, error) {
	pr := k.buildMQueueMetaKey(prefix)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	ids := treeset.NewWith(func(i, j interface{}) int {
		return bytes.Compare(i.([]byte), j.([]byte))
	})
	err = tx.ScanPrefix(ctx, pr, func(key, value []byte) error {
		meta := &MqueueMeta{}
		if err := meta.UnmarshalVT(value); err != nil {
			// Ignore if we can't parse metadata.
			return nil
			// return err
		}

		id := meta.GetId()
		if ids.Contains(id) {
			return nil
		}
		ids.Add(id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	keys := make([][]byte, 0, ids.Size())
	ids.Each(func(index int, value interface{}) {
		keys = append(keys, value.([]byte))
	})
	return keys, nil
}

// OpenMqueue opens a message queue by ID.
//
// If the message queue does not exist, creates it.
func (k *KVTx) OpenMqueue(ctx context.Context, id []byte) (mqueue.Queue, error) {
	metaID := k.buildMQueueMetaKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	exists, err := tx.Exists(ctx, metaID)
	tx.Discard()
	if err != nil {
		return nil, err
	}
	if !exists {
		tx, err = k.store.NewTransaction(ctx, true)
		if err != nil {
			return nil, err
		}
		exists, err = tx.Exists(ctx, metaID)
		if err != nil {
			tx.Discard()
			return nil, err
		}
		if !exists {
			meta := &MqueueMeta{Id: id}
			dat, err := meta.MarshalVT()
			if err != nil {
				tx.Discard()
				return nil, err
			}
			err = tx.Set(ctx, metaID, dat)
			if err != nil {
				tx.Discard()
				return nil, err
			}
			err = tx.Commit(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			tx.Discard()
		}
	}

	// build the mqueue store
	return kvtx_mqueue.NewMQueue(
		k.buildMQueueStore(id),
		k.conf.GetMqueueConfig(),
	), nil
}

// DelMqueue deletes a mqueue by ID.
//
// If not found, should not return an error.
func (k *KVTx) DelMqueue(ctx context.Context, id []byte) error {
	metaID := k.buildMQueueMetaKey(id)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	exists, err := tx.Exists(ctx, metaID)
	tx.Discard()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	tx, err = k.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	// delete all with prefix
	err = tx.ScanPrefixKeys(ctx, k.kvkey.GetMQueuePrefix(id), func(key []byte) error {
		return tx.Delete(ctx, key)
	})
	if err != nil {
		return err
	}
	err = tx.Delete(ctx, metaID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// buildMQueueMetaKey builds the key for metadata for a given mqueue.
func (k *KVTx) buildMQueueMetaKey(id []byte) []byte {
	return bytes.Join([][]byte{k.kvkey.GetMQueueMetaPrefix(), id}, nil)
}

// buildMQueueStore builds the prefixed mqueue store.
func (k *KVTx) buildMQueueStore(id []byte) kvtx.Store {
	storePrefix := k.kvkey.GetMQueuePrefix(id)
	return kvtx_prefixer.NewPrefixer(k.store, storePrefix)
}

// _ is a type assertion
var _ mqueue_store.Store = ((*KVTx)(nil))
