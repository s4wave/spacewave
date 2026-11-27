package packmove

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// PackList writes the blocks of cfg.Source that the key list at cfg.ListPath
// lists, in key order, as packs in cfg.Packer. The list is FindUnpacked's
// output: one hex-encoded block key per line, ascending.
//
// It reads each listed block from one snapshot per pack, as Pack does, and
// shares Pack's log, hash check and references: a block whose data does not
// match its hash, or a key that is not a block ref, is recorded as bad and is
// in no pack. A listed key the volume does not hold fails the run, since the
// list came from the volume. The log at cfg.LogPath resumes a killed run after
// its last committed pack.
func PackList(ctx context.Context, cfg PackConfig) (PackProgress, error) {
	// Check the volume's format.
	if err := requireFormat(ctx, cfg); err != nil {
		return PackProgress{}, err
	}
	if cfg.ListPath == "" {
		return PackProgress{}, errors.New("pack-list needs the key list")
	}
	f, err := os.Open(cfg.ListPath)
	if err != nil {
		return PackProgress{}, err
	}
	defer f.Close()

	// Resume from the log.
	run, err := startRun(ctx, cfg)
	if err != nil {
		return PackProgress{}, err
	}
	defer func() { _ = run.log.close() }()

	// Read the listed blocks after the log's last key.
	reader := newListBatchReader(cfg, f, run.after())
	err = run.pipe(ctx, reader.next, nil)
	return run.snapshot(), err
}

// listBatchReader reads the listed blocks of the volume in list order, one
// batch per snapshot.
type listBatchReader struct {
	// src is the volume store.
	src kvtx.Store
	// prefix is the volume's block key prefix.
	prefix []byte
	// list reads the key list.
	list *bufio.Reader
	// resume is the log's last key, relative to prefix: the listed keys up
	// to it are packed already and are passed.
	resume []byte
	// last is the last key read from the list.
	last []byte
	// packBytes is the block data one batch holds.
	packBytes int
	// queue holds the keys read from the list and not yet in a batch.
	queue [][]byte
}

// newListBatchReader binds the input list and resume key to the pack size policy.
func newListBatchReader(cfg PackConfig, list io.Reader, resume []byte) *listBatchReader {
	packBytes := cfg.PackBytes
	if packBytes <= 0 {
		packBytes = DefaultPackBytes
	}
	return &listBatchReader{src: cfg.Source, prefix: cfg.Keys.GetBlockFullPrefix(), list: bufio.NewReader(list), resume: resume, packBytes: packBytes}
}

// next reads the batch after the last one, or returns nil at the end of the
// list. It stops after the block that brings the batch to packBytes or to the
// packfile's block limit.
func (r *listBatchReader) next(ctx context.Context) (*packBatch, error) {
	// Read enough keys from the list for a full batch.
	if err := r.fill(int(writer.DefaultMaxBlocksPerPack)); err != nil { //nolint:gosec // the policy is positive.
		return nil, err
	}
	if len(r.queue) == 0 {
		return nil, nil
	}

	// Read the batch from a snapshot the writer did not pass.
	var batch *packBatch
	var used int
	err := ReadStable(ctx, r.src, func(tx kvtx.Tx) error {
		var err error
		batch, used, err = r.read(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	r.queue = r.queue[used:]
	return batch, nil
}

// read reads the blocks of the queued keys from tx until the batch is full,
// returning the batch and the number of keys it took.
func (r *listBatchReader) read(ctx context.Context, tx kvtx.Tx) (*packBatch, int, error) {
	// Take queued keys until the batch is full.
	batch := &packBatch{frame: frame{kind: framePending}}
	var used, packed int
	for _, key := range r.queue {
		if batch.bytes >= uint64(r.packBytes) || packed >= int(writer.DefaultMaxBlocksPerPack) { //nolint:gosec // the policy is positive.
			break
		}
		used++
		batch.through = key

		// Read the listed block.
		data, found, err := tx.Get(ctx, append(bytes.Clone(r.prefix), key...))
		if err != nil {
			return nil, 0, err
		}
		if !found {
			return nil, 0, errors.Errorf("listed block %x is not in the volume", key)
		}

		// Pack a block whose data matches its hash, and record any other.
		ref := &block.BlockRef{}
		if ref.UnmarshalVT(key) != nil || ref.VerifyData(data, false) != nil {
			batch.entries = append(batch.entries, logEntry{key: key, bad: true})
			continue
		}
		batch.entries = append(batch.entries, logEntry{key: key})
		batch.puts = append(batch.puts, &block.PutBatchEntry{Ref: ref, Data: bytes.Clone(data), RefsKnown: true})
		batch.bytes += uint64(len(data))
		packed++
	}

	// Read the packed blocks' references from the same snapshot.
	if err := attachRefs(ctx, tx, batch); err != nil {
		return nil, 0, err
	}
	return batch, used, nil
}

// fill reads keys from the list into the queue until the queue holds n keys or
// the list ends, passing the keys up to resume and failing on a key out of
// order.
func (r *listBatchReader) fill(n int) error {
	for len(r.queue) < n {
		line, err := r.list.ReadBytes('\n')
		if err == io.EOF && len(line) == 0 {
			return nil
		}
		if err != nil && err != io.EOF {
			return err
		}
		key, err := hex.DecodeString(string(bytes.TrimSpace(line)))
		if err != nil {
			return errors.Wrap(err, "decode listed key")
		}
		if r.last != nil && bytes.Compare(key, r.last) <= 0 {
			return errors.Errorf("listed key %x is out of order", key)
		}
		r.last = key
		if r.resume != nil && bytes.Compare(key, r.resume) <= 0 {
			continue
		}
		r.queue = append(r.queue, key)
	}
	return nil
}
