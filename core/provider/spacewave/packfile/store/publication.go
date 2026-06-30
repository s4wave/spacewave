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
// Fast path: block catalog hit for a verified/published record returns
// immediately from resident spans. A record still verifying causes the
// caller to wait on its readyCh. A failed record is recoverable; the record
// is unpublished so the caller falls through to a fresh fetch.
//
// Slow path: load the kvfile index (via the shared ReaderAt, so trailer
// bytes land in the span store), find the target entry, compute the
// semantic neighborhood window, ensure those bytes are resident, admit
// every fully-contained block into the catalog, and either return the target
// bytes immediately or wait for verification when the owner requires it.
func (e *PackReader) getBlock(ctx context.Context, key []byte) ([]byte, bool, error) {
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
			return data, true, readErr
		}
		if rec == nil {
			break
		}
		// Block is loading/verifying; wait.
		if readyCh != nil {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case <-readyCh:
				continue
			}
		}
	}

	// Slow path: ensure the index is loaded, resolve the target entry.
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return nil, false, err
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
		return nil, false, nil
	}

	// Drive transport fetches to cover the semantic window.
	if err := e.ensureWindowResident(ctx, windowStart, windowEnd); err != nil {
		return nil, false, err
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
		for _, entry := range contained {
			off := int64(entry.GetOffset())
			end := off + int64(entry.GetSize())
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
		return nil, false, nil
	}

	// Default miss-path callers serve directly from resident spans without
	// blocking on verification. Backend bundle endpoints opt into
	// verify-before-serve so externally visible bytes are hash-checked first.
	if !verifyBeforeServe {
		if readErr != nil {
			return nil, false, readErr
		}
		return data, true, nil
	}

	select {
	case <-readyCh:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	goto retry
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

func (e *PackReader) statBlock(ctx context.Context, key []byte, ref *block.BlockRef) (*block.BlockStat, error) {
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return nil, err
	}

	var size int64
	var found bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entry, ok := e.findEntryByKeyLocked(key)
		if !ok {
			return
		}
		size = int64(entry.GetSize())
		found = true
	})
	if !found {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// verifyBlock runs hash verification and optional writeback for one record.
//
// On success the record transitions to Verified, or Published when writeback
// is enabled. On mismatch the record transitions to Failed and is removed
// from the catalog so a later read can retry transport.
func (e *PackReader) verifyBlock(rec *blockRecord) {
	data, err := rec.readBytes()
	if err != nil {
		e.finishVerify(rec, err, nil)
		return
	}

	ref, err := block.BuildBlockRef(data, &block.PutOpts{
		ForceBlockRef: rec.ref.Clone(),
	})
	if err != nil {
		e.finishVerify(rec, err, nil)
		return
	}
	if !ref.EqualsRef(rec.ref) {
		e.finishVerify(rec, block.ErrBlockRefMismatch, nil)
		return
	}

	var writeErr error
	var target block.StoreOps
	var wbCtx context.Context
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		target = e.writebackTarget
		wbCtx = e.writebackCtx
	})
	if target != nil && wbCtx != nil {
		_, _, writeErr = target.PutBlock(wbCtx, data, &block.PutOpts{
			ForceBlockRef: rec.ref.Clone(),
		})
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
