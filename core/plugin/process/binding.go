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
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	data, err := binding.MarshalVT()
	if err != nil {
		return err
	}

	key := ProcessBindingKey(spaceID, objectKey)
	if err := tx.Set(ctx, key, data); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetProcessBinding reads the ProcessBinding from the store.
// Returns nil, nil if the key is not found.
func GetProcessBinding(ctx context.Context, store kvtx.Store, spaceID, objectKey string) (*s4wave_process.ProcessBinding, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	key := ProcessBindingKey(spaceID, objectKey)
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	binding := &s4wave_process.ProcessBinding{}
	if err := binding.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return binding, nil
}

// ListProcessBindings lists all process bindings for a given space.
func ListProcessBindings(ctx context.Context, store kvtx.Store, spaceID string) ([]*s4wave_process.ProcessBinding, error) {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	prefix := []byte(ProcessBindingKeyPrefix + "/" + spaceID + "/")
	var bindings []*s4wave_process.ProcessBinding
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		binding := &s4wave_process.ProcessBinding{}
		if err := binding.UnmarshalVT(value); err != nil {
			return err
		}
		bindings = append(bindings, binding)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return bindings, nil
}
