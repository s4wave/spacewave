package store

import (
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
// ref.
//
// Slow path: fetchBlock. Fetched target bytes can be evicted before the read
// copies them, so the slow path runs at most twice.
//
// Returns nil when the pack index does not hold the block.
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
			if rec == nil || rec.state == blockStatePublished {
				rec = nil
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

	for range 2 {
		stored, lost, err := e.fetchBlock(ctx, key)
		if !lost {
			return stored, err
		}
	}
	return nil, errors.Errorf("packfile block %x not resident after fetch", key)
}

// fetchBlock loads the kvfile index (via the shared ReaderAt, so trailer
// bytes land in the span store), finds the target entry, ensures its
// semantic neighborhood window is resident, admits every fully-contained
// block into the catalog for background verification and writeback, and
// returns the target block read from the resident spans after checking its
// hash.
//
// Fetched spans promote their blocks at once, so background verification can
// reject the target bytes and drop them before this read copies them; that
// returns ErrBlockRefMismatch like an inline check would. lost reports that
// the target bytes were evicted instead and the read may retry.
func (e *PackReader) fetchBlock(ctx context.Context, key []byte) (stored *block.StoredBlock, lost bool, err error) {
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return nil, false, err
	}

	var (
		targetRef    *block.BlockRef
		refErr       error
		targetOff    int64
		targetEnd    int64
		windowStart  int64
		windowEnd    int64
		contained    []*kvfile.IndexEntry
		indexMissing bool
		failures     uint64
	)
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entry, ok := e.findEntryByKeyLocked(key)
		if !ok {
			indexMissing = true
			return
		}
		targetRef, refErr = parseBlockRef(entry)
		targetOff = int64(entry.GetOffset())           //nolint:gosec // the catalog validator bounds offsets by the int64 pack size.
		targetEnd = targetOff + int64(entry.GetSize()) //nolint:gosec // validated entry extents cannot overflow or exceed the pack.
		windowStart, windowEnd, contained = e.semanticWindowLocked(entry)
		failures = e.verifyFailures
	})
	if indexMissing {
		return nil, false, nil
	}
	if refErr != nil {
		return nil, false, refErr
	}

	// Drive transport fetches to cover the semantic window.
	if err := e.ensureResident(ctx, windowStart, windowEnd, false); err != nil {
		return nil, false, err
	}

	// Admit every fully-contained block, gather verify jobs, and copy the
	// target bytes.
	var data []byte
	var jobs []func()
	var verifyJobs []func()
	var rejected bool
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		for _, entry := range contained {
			off := int64(entry.GetOffset())     //nolint:gosec // the catalog validator bounds offsets by the int64 pack size.
			end := off + int64(entry.GetSize()) //nolint:gosec // validated entry extents cannot overflow or exceed the pack.
			if job, ok := e.admitBlockLocked(entry, off, end); ok && job != nil {
				jobs = append(jobs, job)
			}
		}
		if spans, covered := e.collectSpansLocked(targetOff, targetEnd); covered {
			data = make([]byte, targetEnd-targetOff)
			copySpans(data, spans, targetOff)
		}
		rejected = e.verifyFailures != failures
		if len(jobs) != 0 {
			verifyJobs = e.prepareVerifyJobsLocked(jobs...)
		}
		broadcast()
	})
	e.enqueueVerifyJobs(verifyJobs)

	switch {
	case data == nil && rejected:
		return nil, false, errors.Wrapf(block.ErrBlockRefMismatch, "packfile block %x", key)
	case data == nil:
		return nil, true, nil
	}
	stored, err = decodeVerifiedBlock(targetRef, data)
	return stored, false, err
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
// is enabled. On mismatch the record is removed from the catalog so a later
// read can retry transport.
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
// (corruption in flight is not guaranteed to recur). Publishing releases the
// record's spans: the writeback target serves the block from now on, so the
// bytes become evictable. A publish error leaves the record Verified and
// resident.
func (e *PackReader) finishVerify(rec *blockRecord, verifyErr, writeErr error) {
	var budget *residentBudget
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		budget = e.budget
		if !rec.enqueueAt.IsZero() {
			e.lastPublishDur = time.Since(rec.enqueueAt)
		}
		rec.queued = false
		switch {
		case verifyErr != nil:
			spans := slices.Clone(rec.spans)
			e.verifyFailures++
			e.removeBlockLocked(rec)
			e.removeUnpinnedSpansLocked(spans)
		case writeErr != nil:
			rec.state = blockStateVerified
			e.writebackErrors++
		case e.writebackTarget != nil:
			rec.state = blockStatePublished
			e.writebackCount++
			e.releaseSpansLocked(rec.spans)
			rec.spans = nil
		default:
			rec.state = blockStateVerified
		}
		broadcast()
	})
	budget.reclaim()
}
