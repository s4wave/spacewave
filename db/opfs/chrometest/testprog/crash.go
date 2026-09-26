//go:build js

package main

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
)

// crashBackend stops one real publication immediately before or after its root.
type crashBackend struct {
	// backend provides actual browser files and cross-runtime locks.
	backend engine.Backend
	// config identifies the worker whose ready message triggers termination.
	config *config
	// armed excludes initialization from the injected termination boundary.
	armed bool
	// after selects the durable side of the alternate root write.
	after bool
	// blocks requires a payload output before stopping publication.
	blocks bool
	// wrotePack records payload preparation within the publication lock.
	wrotePack bool
}

// Write stops at the selected root boundary after all immutable outputs exist.
func (b *crashBackend) Write(ctx context.Context, name string, data []byte) error {
	if strings.HasPrefix(name, "pack-") {
		b.wrotePack = true
	}
	stop := b.armed && strings.HasPrefix(name, "root-") && (!b.blocks || b.wrotePack)
	if !stop || b.after {
		if err := b.backend.Write(ctx, name, data); err != nil {
			return err
		}
	}
	if stop {
		postReady(b.config)
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

// Read forwards bounded immutable reads without injection.
func (b *crashBackend) Read(ctx context.Context, name string, offset int64, length int) ([]byte, error) {
	return b.backend.Read(ctx, name, offset, length)
}

// Remove forwards retirement and intent cleanup.
func (b *crashBackend) Remove(ctx context.Context, name string) error {
	return b.backend.Remove(ctx, name)
}

// Lock preserves the production cross-runtime lock protocol.
func (b *crashBackend) Lock(ctx context.Context, name string, exclusive bool) (func(), error) {
	return b.backend.Lock(ctx, name, exclusive)
}

// Subscribe preserves publication wakeups.
func (b *crashBackend) Subscribe() (<-chan struct{}, func()) { return b.backend.Subscribe() }

// Notify preserves publication notifications after durability.
func (b *crashBackend) Notify(generation uint64) { b.backend.Notify(generation) }

// Close releases the browser notification resources.
func (b *crashBackend) Close() error { return b.backend.Close() }

// runEngineCrash waits for harness termination with a real publication in flight.
func runEngineCrash(ctx context.Context, c *config, after, blocks bool) error {
	parts := []string{"volume"}
	prefix := c.root + "/volume"
	if blocks {
		parts = nil
		prefix = c.root
	}
	dir, err := openTestDirectory(c.root, parts)
	if err != nil {
		return err
	}
	b := &crashBackend{backend: engine.NewBrowserBackend(opfs.DefaultDriver, dir, prefix), config: c, after: after, blocks: blocks}
	e, err := engine.Open(ctx, b)
	if err != nil {
		return err
	}
	defer e.Close()
	b.armed = true
	if blocks {
		store := engine.NewBlockStore(ctx, e, block.DefaultHashType)
		defer store.Close()
		_, _, err := store.PutBlock(ctx, []byte("uncommitted crash payload"), &block.PutOpts{Sync: true})
		return err
	}
	return e.Apply(ctx, []*engine.Record{{Key: []byte("\x01crash-marker"), Value: []byte("committed")}})
}

// verifyEngineCrash checks recovery through a fresh engine and public KV store.
func verifyEngineCrash(ctx context.Context, c *config, blocks bool) error {
	if !blocks {
		vol, err := openVolume(ctx, c)
		if err != nil {
			return err
		}
		defer vol.Close()
		return verifyMetaValue(ctx, vol.GetKvtxStore(), []byte("crash-marker"), []byte("committed"))
	}
	e, store, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	ref, err := block.BuildBlockRef([]byte("uncommitted crash payload"), nil)
	if err != nil {
		return err
	}
	found, err := store.GetBlockExists(ctx, ref)
	if err != nil {
		return err
	}
	if found {
		return errors.New("unpublished payload became visible after termination")
	}
	if err := e.Maintenance(ctx); err != nil {
		return err
	}
	dir, err := openTestDirectory(c.root, nil)
	if err != nil {
		return err
	}
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return err
	}
	var packs int
	for _, name := range names {
		if strings.HasPrefix(name, "pack-") {
			packs++
		}
	}
	if packs != 1 {
		return errors.Errorf("recovered payload files=%d, want one committed pack", packs)
	}
	return nil
}

// _ verifies the fault injector preserves the backend contract.
var _ engine.Backend = (*crashBackend)(nil)
