package store

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// getBlock is the top-level engine read path.
//
// Fast path: serve a catalog record. A record still waiting for background
// verification is hash-checked inline, so every returned block matches its
// ref. Failed records are removed so reads can retry transport.
//
// Slow path: load the kvfile index (via the shared ReaderAt, so trailer
// bytes land in the span store), find the target entry, compute the
// semantic neighborhood window, ensure those bytes are resident, admit
// every fully-contained block into the catalog for background verification
// and writeback, and return the target block after checking its hash.
//
// Returns nil when the pack does not hold the block.
func (e *PackReader) getBlock(ctx context.Context, key []byte) (*block.StoredBlock, error) {
	keyStr := string(key)

	// Fast path: serve a resident catalog record.
	for {
		var rec *blockRecord
		var data []byte
		var readErr error
		var verified bool
		var invalidated bool
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if e.closed {
				return
			}
			rec = e.lookupBlockLocked(keyStr)
			if rec == nil {
				return
			}
			if rec.state == blockStateFailed {
				e.removeBlockLocked(rec)
				rec = nil
				broadcast()
				return
			}
			data, readErr = rec.readBytes()
			if readErr != nil {
				e.removeBlockLocked(rec)
				invalidated = true
				broadcast()
				return
			}
			verified = rec.state != blockStateVerifying
		})
		if invalidated {
			continue
		}
		if rec == nil {
			break
		}
		if verified {
			return block.DecodeBlockObject(data)
		}
		return decodeVerifiedBlock(rec.ref, data)
	}

	// Slow path: ensure the index is loaded, resolve the target entry.
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return nil, err
	}

	var (
		windowStart  int64
		windowEnd    int64
		contained    []*kvfile.IndexEntry
		indexMissing bool
	)
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entry, ok := e.findEntryByKeyLocked(key)
		if !ok {
			indexMissing = true
			return
		}
		windowStart, windowEnd, contained = e.semanticWindowLocked(entry)
	})
	if indexMissing {
		return nil, nil
	}

	// Drive transport fetches to cover the semantic window.
	if err := e.ensureWindowResident(ctx, windowStart, windowEnd); err != nil {
		return nil, err
	}

	// Admit every fully-contained block and gather verify jobs.
	var target *blockRecord
	var jobs []func()
	var data []byte
	var readErr error
	var verifyJobs []func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		for _, entry := range contained {
			off := int64(entry.GetOffset())     //nolint:gosec // the catalog validator bounds offsets by the int64 pack size.
			end := off + int64(entry.GetSize()) //nolint:gosec // validated entry extents cannot overflow or exceed the pack.
			isTarget := bytes.Equal(entry.GetKey(), key)
			job, ok := e.admitBlockLocked(entry, off, end, isTarget)
			if !ok {
				continue
			}
			if job != nil {
				jobs = append(jobs, job)
			}
			if isTarget {
				target = e.blocks[string(entry.GetKey())]
			}
		}
		if target != nil {
			data, readErr = target.readBytes()
			if readErr != nil {
				e.removeBlockLocked(target)
			}
		}
		if len(jobs) != 0 {
			verifyJobs = e.prepareVerifyJobsLocked(jobs...)
		}
		broadcast()
	})
	e.enqueueVerifyJobs(verifyJobs)

	if target == nil {
		// The index entry existed but the block could not be admitted.
		// This happens when spans failed to cover the target extent after
		// ensureWindowResident, which usually means a short or truncated
		// transport response.
		return nil, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	return decodeVerifiedBlock(target.ref, data)
}

// decodeVerifiedBlock decodes a pack value and checks its data against ref.
func decodeVerifiedBlock(ref *block.BlockRef, value []byte) (*block.StoredBlock, error) {
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, err
	}
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, errors.Wrapf(block.ErrBlockRefMismatch, "packfile block %s", ref.MarshalString())
	}
	return stored, nil
}

func (e *PackReader) getBlockExists(ctx context.Context, key []byte) (bool, error) {
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return false, err
	}

	var found bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, found = e.findEntryByKeyLocked(key)
	})
	return found, nil
}

// statBlock reports whether the pack holds the block. The data size is
// unknown without decoding the value, so it is -1.
func (e *PackReader) statBlock(ctx context.Context, key []byte, ref *block.BlockRef) (*block.BlockStat, error) {
	found, err := e.getBlockExists(ctx, key)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
}

// verifyBlock runs hash verification and optional writeback for one record.
//
// On success the record transitions to Verified, or Published when writeback
// is enabled. On mismatch the record transitions to Failed and is removed
// from the catalog so a later read can retry transport.
func (e *PackReader) verifyBlock(rec *blockRecord) {
	var closed bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		closed = e.closed
	})
	if closed {
		e.finishVerify(rec, context.Canceled, nil)
		return
	}

	value, err := rec.readBytes()
	if err != nil {
		e.finishVerify(rec, err, nil)
		return
	}
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		e.finishVerify(rec, err, nil)
		return
	}
	if err := rec.ref.VerifyData(stored.Data, false); err != nil {
		e.finishVerify(rec, block.ErrBlockRefMismatch, nil)
		return
	}

	var writeErr error
	var target block.StoreOps
	var wbCtx context.Context
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !e.closed {
			target = e.writebackTarget
			wbCtx = e.writebackCtx
		}
	})
	if target != nil && wbCtx != nil {
		_, _, writeErr = target.PutBlock(wbCtx, stored.Data, stored.PutOpts(rec.ref))
	}
	e.finishVerify(rec, nil, writeErr)
}

// finishVerify records verify/publish completion for a block record.
//
// A verify error removes the record so the caller can retry transport
// (corruption in flight is not guaranteed to recur). A publish error
// leaves the record in the Verified state but unpublished; callers that
// observed the published state via readBytes are unaffected.
func (e *PackReader) finishVerify(rec *blockRecord, verifyErr, writeErr error) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		dur := time.Duration(0)
		if !rec.enqueueAt.IsZero() {
			dur = time.Since(rec.enqueueAt)
		}
		rec.queued = false
		if verifyErr != nil {
			spans := slices.Clone(rec.spans)
			rec.state = blockStateFailed
			rec.err = verifyErr
			e.verifyFailures++
			e.lastPublishDur = dur
			// Remove the failed record so later reads retry cleanly.
			e.removeBlockLocked(rec)
			e.removeUnpinnedSpansLocked(spans)
			close(rec.readyCh)
			broadcast()
			return
		}
		rec.err = writeErr
		if writeErr == nil && e.writebackTarget != nil {
			rec.state = blockStatePublished
			rec.writtenBack = true
			e.writebackCount++
		} else {
			rec.state = blockStateVerified
		}
		if writeErr != nil {
			e.writebackErrors++
		}
		e.lastPublishDur = dur
		close(rec.readyCh)
		e.evictLocked()
		broadcast()
	})
}
