//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
)

// Output layouts.
const (
	// layoutPackStore writes packs/{id} and entries/{id}, the PackStore's
	// own layout.
	layoutPackStore = "packstore"
	// layoutAdopt writes {id} and {id}.entry in one flat directory, the
	// layout hyperline-backend world-adopt-packs reads.
	layoutAdopt = "adopt"
)

// adoptEntrySuffix ends the name of a pack's encoded PackfileEntry.
const adoptEntrySuffix = ".entry"

// adoptPartSuffix ends the name of a staged pack or entry, written in full
// and synced but not yet visible under its final name.
const adoptPartSuffix = ".part"

// adoptObjects is a block_store_s3.ObjectStore on a flat directory in the
// adopt layout: the PackStore's packs/{id} is the file {id} and its
// entries/{id} the file {id}.entry. The PackStore writes a pack before its
// entry; the pack is staged as {id}.part, the entry as {id}.entry.part, and
// both are renamed into place once the entry is staged, the pack first. A
// pack is never visible under its final name half written, and a crash
// between the two renames is finished by recoverAdopt. It never deletes.
type adoptObjects struct{}

// adoptName returns the file name of a PackStore key, or an error for a key
// outside packs/ and entries/.
func adoptName(key string) (string, error) {
	// Map the two directories the PackStore writes to flat names.
	if id, ok := strings.CutPrefix(key, "packs/"); ok && validAdoptID(id) {
		return id, nil
	}
	if id, ok := strings.CutPrefix(key, "entries/"); ok && validAdoptID(id) {
		return id + adoptEntrySuffix, nil
	}
	return "", errors.Errorf("key %q has no place in the adopt layout", key)
}

// validAdoptID reports whether id names a pack file of its own.
func validAdoptID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && !strings.HasPrefix(id, ".") &&
		!strings.HasSuffix(id, adoptEntrySuffix) && !strings.HasSuffix(id, adoptPartSuffix)
}

// PutObject stages a pack, or stages an entry and renames the entry's pack
// and then the entry into place.
func (adoptObjects) PutObject(_ context.Context, bucket, key string, data []byte, _ string) error {
	// Stage the object, synced, beside its final name.
	name, err := adoptName(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		return err
	}
	if err := writeSynced(bucket, name+adoptPartSuffix, data); err != nil {
		return err
	}

	// Publish the pair once the entry is staged: the pack first, so an entry
	// never names a missing pack, then the entry.
	id, isEntry := strings.CutSuffix(name, adoptEntrySuffix)
	if !isEntry {
		return nil
	}
	if _, err := os.Stat(filepath.Join(bucket, id+adoptPartSuffix)); err != nil {
		return errors.Wrap(err, "entry "+id+" has no staged pack")
	}
	return publishAdopt(bucket, id)
}

// publishAdopt renames the staged pack and then the staged entry of id into
// place, skipping a rename already done, and syncs the directory.
func publishAdopt(dir, id string) error {
	// Rename each staged file whose final name is not yet there.
	for _, name := range []string{id, id + adoptEntrySuffix} {
		part := filepath.Join(dir, name+adoptPartSuffix)
		if _, err := os.Stat(part); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := os.Rename(part, filepath.Join(dir, name)); err != nil {
			return err
		}
	}

	// Make the renames durable.
	return syncDir(dir)
}

// recoverAdopt finishes the pairs a crash left between the pack's and the
// entry's rename: every staged entry is published with its pack, and its id
// returned in published, so the caller records the pack's blocks rather than
// fetch them again. A staged pack with no staged entry stays staged,
// invisible to the adopt reader and to world-adopt-packs, which take only
// {id}.entry files: its blocks, never recorded in the ledger, are fetched
// again into a new pack. It is returned in orphans and not removed, since
// the rescue never deletes.
func recoverAdopt(dir string) (published, orphans []string, err error) {
	// Find the staged entries.
	names, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n.Name()] = true
	}

	// Publish each with its pack, staged or already renamed, and list the
	// staged packs with no staged entry.
	for _, n := range names {
		if id, ok := strings.CutSuffix(n.Name(), adoptPartSuffix); ok && validAdoptID(id) && !have[id+adoptEntrySuffix+adoptPartSuffix] {
			orphans = append(orphans, n.Name())
			continue
		}
		id, ok := strings.CutSuffix(n.Name(), adoptEntrySuffix+adoptPartSuffix)
		if !ok || !validAdoptID(id) || (!have[id+adoptPartSuffix] && !have[id]) {
			continue
		}
		if err := publishAdopt(dir, id); err != nil {
			return published, orphans, err
		}
		published = append(published, id)
	}
	return published, orphans, nil
}

// ledgerAdopted records as done every block of the published packs ids in
// dir that the ledger does not already hold as done, read from each pack's
// own index, and syncs the ledger: the pack holds them durably, and the run
// skips them rather than store them twice.
func ledgerAdopted(l *ledger, dir string, ids []string) error {
	for _, id := range ids {
		// Read the pack's index.
		f, err := os.Open(filepath.Join(dir, id))
		if err != nil {
			return err
		}
		st, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		index, err := packfile.ReadIndex(f, uint64(st.Size())) //nolint:gosec // sizes are not negative.
		_ = f.Close()
		if err != nil {
			return errors.Wrap(err, "read the index of recovered pack "+id)
		}

		// Record its blocks.
		for _, ie := range index {
			if l.refs[string(ie.GetKey())] == stateDone {
				continue
			}
			if err := l.ref(ie.GetKey(), stateDone, ""); err != nil {
				return err
			}
		}
	}
	return l.sync()
}

// writeSynced writes data to dir/name through a temporary file, synced and
// renamed into place.
func writeSynced(dir, name string, data []byte) error {
	// Write and sync a temporary file.
	tmp, err := os.CreateTemp(dir, dirTempPrefix)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}

	// Rename it into place.
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	return errors.Wrap(err, "write "+name)
}

// syncDir syncs a directory so the renames in it are durable.
func syncDir(dir string) error {
	// Open the directory, sync it and close it.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// GetObject reads the whole published object.
func (a adoptObjects) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	// Read the object's file.
	name, err := adoptName(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(bucket, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Wrap(block_store_s3.ErrNotFound, key)
	}
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// GetObjectRange reads length bytes at off of a published object.
func (a adoptObjects) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	name, err := adoptName(key)
	if err != nil {
		return nil, err
	}
	return dirObjects{}.GetObjectRange(ctx, bucket, name, off, length)
}

// ListObjects lists the published entries as entries/{id} and their packs as
// packs/{id}, in id order. Staged and temporary files are skipped.
func (adoptObjects) ListObjects(_ context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	// Read the directory, a missing one listing nothing.
	names, err := os.ReadDir(bucket)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	// Report each published pair's files under the prefix.
	for _, n := range names {
		id, ok := strings.CutSuffix(n.Name(), adoptEntrySuffix)
		if !ok || !validAdoptID(id) {
			continue
		}
		for _, f := range []struct{ key, name string }{{"entries/" + id, n.Name()}, {"packs/" + id, id}} {
			if !strings.HasPrefix(f.key, prefix) {
				continue
			}
			info, err := os.Stat(filepath.Join(bucket, f.name))
			if err != nil {
				return err
			}
			if err := fn(f.key, info.Size()); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteObject refuses: the rescue never deletes.
func (adoptObjects) DeleteObject(_ context.Context, _, key string) error {
	return errors.Errorf("refusing to delete %s: the rescue output is append only", key)
}

// _ is a type assertion
var _ block_store_s3.ObjectStore = adoptObjects{}
