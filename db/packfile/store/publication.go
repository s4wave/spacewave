package store

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
)

// getBlock is the top-level engine read path.
//
// Catalog hits and misses follow the same verifyBeforeServe policy. Default
// readers use resident bytes while background verification and writeback run.
// Opt-in readers wait on readyCh. Failed records are removed so reads can retry.
//
// Slow path: load the kvfile index (via the shared ReaderAt, so trailer
// bytes land in the span store), find the target entry, compute the
// semantic neighborhood window, ensure those bytes are resident, admit
// every fully-contained block into the catalog, and either return the target
// block immediately or wait for verification when verifyBeforeServe is set.
//
// Returns nil when the pack does not hold the block.
func (e *PackReader) getBlock(ctx context.Context, key []byte) (*block.StoredBlock, error) {
	keyStr := string(key)

retry:
	// Try fast path repeatedly: a verifying record may resolve while we wait.
	for {
		var data []byte
		var rec *blockRecord
		var readErr error
		var readyCh <-chan struct{}
		var served bool
		var failed bool
		var invalidated bool

		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if e.closed {
				return
			}
			rec = e.lookupBlockLocked(keyStr)
			if rec == nil {
				return
			}
			switch rec.state {
			case blockStateFailed:
				e.removeBlockLocked(rec)
				invalidated = true
				failed = true
				broadcast()
			case blockStateVerifying:
				if e.verifyBeforeServe {
					readyCh = rec.readyCh
					return
				}
				fallthrough
			case blockStateVerified, blockStatePublished:
				data, readErr = rec.readBytes()
				if readErr != nil {
					e.removeBlockLocked(rec)
					invalidated = true
					broadcast()
					return
				}
				served = true
			default:
				readyCh = rec.readyCh
			}
		})

		if failed {
			// Treat failed blocks as a cache miss so the caller can retry.
			break
		}
		if invalidated {
			continue
		}
		if served {
			return decodeBlockValue(data, readErr)
		}
		if rec == nil {
			break
		}
		// Block is loading/verifying; wait.
		if readyCh != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-e.ctx.Done():
				return nil, context.Canceled
			case <-readyCh:
				continue
			}
		}
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
	var firstMiss *blockRecord
	var readyCh <-chan struct{}
	var jobs []func()
	var data []byte
	var readErr error
	var verifyBeforeServe bool
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
				firstMiss = e.blocks[string(entry.GetKey())]
			}
		}
		if firstMiss != nil {
			readyCh = firstMiss.readyCh
			verifyBeforeServe = e.verifyBeforeServe
			if !verifyBeforeServe {
				data, readErr = firstMiss.readBytes()
				if readErr != nil {
					e.removeBlockLocked(firstMiss)
					broadcast()
					return
				}
			}
		}
		if len(jobs) != 0 {
			verifyJobs = e.prepareVerifyJobsLocked(jobs...)
		}
		broadcast()
	})
	e.enqueueVerifyJobs(verifyJobs)

	if firstMiss == nil {
		// The index entry existed but the block could not be admitted.
		// This happens when spans failed to cover the target extent after
		// ensureWindowResident, which usually means a short or truncated
		// transport response.
		return nil, nil
	}

	// Default miss-path callers serve directly from resident spans without
	// blocking on verification. Backend bundle endpoints opt into
	// verify-before-serve so externally visible bytes are hash-checked first.
	if !verifyBeforeServe {
		return decodeBlockValue(data, readErr)
	}

	select {
	case <-readyCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.ctx.Done():
		return nil, context.Canceled
	}
	goto retry
}

// decodeBlockValue decodes a pack value read from the catalog.
func decodeBlockValue(value []byte, readErr error) (*block.StoredBlock, error) {
	if readErr != nil {
		return nil, readErr
	}
	return block.DecodeBlockObject(value)
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
