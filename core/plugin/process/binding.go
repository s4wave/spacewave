// Package process_binding provides CRUD helpers for process bindings
// in the platform-account ObjectStore.
package process_binding

import (
	"context"
	"strings"

	"github.com/s4wave/spacewave/db/kvtx"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
)

// ProcessBindingKeyPrefix is the prefix for process binding keys.
const ProcessBindingKeyPrefix = "process-binding"

// DefaultObjectStoreID is the default ObjectStore for process bindings.
const DefaultObjectStoreID = "platform-account"

// ProcessBindingKey returns the KV key for a process binding.
// Key format: process-binding/{spaceID}/{objectKey}
func ProcessBindingKey(spaceID, objectKey string) []byte {
	return []byte(strings.Join([]string{
		ProcessBindingKeyPrefix,
		spaceID,
		objectKey,
	}, "/"))
}

// SetProcessBinding writes the ProcessBinding to the store.
func SetProcessBinding(ctx context.Context, store kvtx.Store, spaceID, objectKey string, binding *s4wave_process.ProcessBinding) error {
	data, err := binding.MarshalVT()
	if err != nil {
		return err
	}
	key := ProcessBindingKey(spaceID, objectKey)
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, key, data)
		},
	)
}

// GetProcessBinding reads the ProcessBinding from the store.
// Returns nil, nil if the key is not found.
func GetProcessBinding(ctx context.Context, store kvtx.Store, spaceID, objectKey string) (*s4wave_process.ProcessBinding, error) {
	key := ProcessBindingKey(spaceID, objectKey)
	var binding *s4wave_process.ProcessBinding
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}
			next := &s4wave_process.ProcessBinding{}
			if err := next.UnmarshalVT(data); err != nil {
				return err
			}
			binding = next
			return nil
		},
	)
	return binding, err
}

// DeleteProcessBinding deletes the stored binding of binding's object key if it
// still equals binding, so a newer decision for the key is kept.
func DeleteProcessBinding(ctx context.Context, store kvtx.Store, spaceID string, binding *s4wave_process.ProcessBinding) error {
	key := ProcessBindingKey(spaceID, binding.GetObjectKey())
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, key)
			if err != nil || !found {
				return err
			}
			current := &s4wave_process.ProcessBinding{}
			if err := current.UnmarshalVT(data); err != nil {
				return err
			}
			if !current.EqualVT(binding) {
				return nil
			}
			return tx.Delete(ctx, key)
		},
	)
}

// ListProcessBindings lists all process bindings for a given space.
func ListProcessBindings(ctx context.Context, store kvtx.Store, spaceID string) ([]*s4wave_process.ProcessBinding, error) {
	prefix := []byte(ProcessBindingKeyPrefix + "/" + spaceID + "/")
	var bindings []*s4wave_process.ProcessBinding
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var attemptBindings []*s4wave_process.ProcessBinding
			err := tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
				binding := &s4wave_process.ProcessBinding{}
				if err := binding.UnmarshalVT(value); err != nil {
					return err
				}
				attemptBindings = append(attemptBindings, binding)
				return nil
			})
			if err == nil {
				bindings = attemptBindings
			}
			return err
		},
	)
	return bindings, err
}
