//go:build js && tinygo

package opfs

import (
	"context"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/pkg/errors"
)

// webLockOp tracks one Web Lock request from acquisition through release.
type webLockOp struct {
	// done closes when the request completes.
	done chan struct{}
	// released closes when the browser frees an acquired lock.
	released chan struct{}
	// releaseID identifies the JavaScript release callback.
	releaseID uint32
	// acquired is set when the request acquired the lock.
	acquired bool
	// err is set when the request failed before reaching JavaScript.
	err error
	// rejected is set when the JavaScript request rejected.
	rejected bool
	// closed is set once done is closed.
	closed bool
}

var (
	webLockMu     sync.Mutex
	nextWebLockID uint32
	webLockOps    = make(map[uint32]*webLockOp)
)

// WebLockOutcome is the terminal outcome for a browser Web Lock request.
type WebLockOutcome int

const (
	WebLockOutcomeAcquired WebLockOutcome = iota
	WebLockOutcomeUnavailable
	WebLockOutcomeCanceled
	WebLockOutcomeRejected
)

// WebLockResult describes a completed Web Lock acquisition attempt.
type WebLockResult struct {
	// Outcome is the terminal outcome of the request.
	Outcome WebLockOutcome
	// Release asks the browser to release an acquired lock. It returns before
	// the browser frees the lock.
	Release func()
	// Released closes once the browser has freed an acquired lock after
	// Release, so another request for the name can acquire it.
	Released <-chan struct{}
}

//go:wasmimport gojs bldr.opfs.acquireWebLock
func tinygoAcquireWebLock(opID uint32, namePtr unsafe.Pointer, nameLen uint32, exclusive uint32, ifAvailable uint32)

//go:wasmimport gojs bldr.opfs.cancelWebLock
func tinygoCancelWebLock(opID uint32) uint32

//go:wasmimport gojs bldr.opfs.releaseWebLock
func tinygoReleaseWebLock(releaseID uint32) uint32

// AcquireWebLock requests a Web Lock and waits until it is acquired.
func AcquireWebLock(name string, exclusive bool) (func(), error) {
	return AcquireWebLockContext(context.Background(), name, exclusive)
}

// AcquireWebLockContext requests a Web Lock and waits until it is acquired.
func AcquireWebLockContext(ctx context.Context, name string, exclusive bool) (func(), error) {
	result, err := DefaultDriver.AcquireWebLock(ctx, name, exclusive)
	if err != nil {
		return nil, err
	}
	if result.Outcome != WebLockOutcomeAcquired {
		return nil, errors.Errorf("blocking WebLock request %s ended with outcome %d", name, result.Outcome)
	}
	return result.Release, nil
}

// AcquireWebLockIfAvailable requests a Web Lock without waiting.
func AcquireWebLockIfAvailable(name string, exclusive bool) (release func(), acquired bool, err error) {
	result, err := DefaultDriver.AcquireWebLockIfAvailable(context.Background(), name, exclusive)
	if err != nil {
		return nil, false, err
	}
	if result.Outcome != WebLockOutcomeAcquired {
		return nil, false, nil
	}
	return result.Release, true, nil
}

// AcquireWebLock requests a Web Lock and waits until it is acquired.
func (d BrowserDriver) AcquireWebLock(ctx context.Context, name string, exclusive bool) (*WebLockResult, error) {
	return d.acquireWebLock(ctx, name, exclusive, false)
}

// AcquireWebLockIfAvailable requests a Web Lock without waiting.
func (d BrowserDriver) AcquireWebLockIfAvailable(ctx context.Context, name string, exclusive bool) (*WebLockResult, error) {
	return d.acquireWebLock(ctx, name, exclusive, true)
}

// acquireWebLock requests a Web Lock through the TinyGo runtime imports.
func (BrowserDriver) acquireWebLock(ctx context.Context, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
	// Report an already canceled ctx without a request.
	if err := ctx.Err(); err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, err
	}

	// Register the op and start the JavaScript request.
	opID, op := registerWebLockOp()
	nameBytes := []byte(name)
	if len(nameBytes) == 0 {
		failWebLockOp(opID, errors.New("WebLock name is empty"))
	} else {
		tinygoAcquireWebLock(
			opID,
			unsafe.Pointer(&nameBytes[0]),
			uint32(len(nameBytes)),
			tinyGoBoolUint32(exclusive),
			tinyGoBoolUint32(ifAvailable),
		)
	}

	// Wait for the outcome, canceling the request with ctx.
	select {
	case <-op.done:
	case <-ctx.Done():
		tinygoCancelWebLock(opID)
		deleteWebLockOp(opID)
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, ctx.Err()
	}

	// Report an outcome that holds no lock.
	if !op.acquired {
		deleteWebLockOp(opID)
	}
	if op.err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, op.err
	}
	if op.rejected {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, errors.New("WebLock request rejected")
	}
	if !op.acquired {
		return &WebLockResult{Outcome: WebLockOutcomeUnavailable}, nil
	}

	// The op stays registered until BLDR_OPFS_WEB_LOCK_RELEASED closes
	// op.released.
	var releaseOnce atomic.Bool
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
			if releaseOnce.CompareAndSwap(false, true) {
				if tinygoReleaseWebLock(op.releaseID) == 0 {
					panic("weblock release callback unavailable")
				}
			}
		},
		Released: op.released,
	}, nil
}

// registerWebLockOp registers a new op under a fresh nonzero ID.
func registerWebLockOp() (uint32, *webLockOp) {
	// Hold the registry lock for the allocation and insert.
	webLockMu.Lock()
	defer webLockMu.Unlock()

	// Allocate the next ID, skipping zero on wraparound.
	nextWebLockID++
	if nextWebLockID == 0 {
		nextWebLockID++
	}

	// Register the op under the ID.
	op := &webLockOp{done: make(chan struct{}), released: make(chan struct{})}
	webLockOps[nextWebLockID] = op
	return nextWebLockID, op
}

func deleteWebLockOp(opID uint32) {
	webLockMu.Lock()
	delete(webLockOps, opID)
	webLockMu.Unlock()
}

func completeWebLockOp(opID, releaseID uint32, acquired bool, rejected bool) {
	webLockMu.Lock()
	op := webLockOps[opID]
	if op == nil || op.closed {
		webLockMu.Unlock()
		return
	}
	op.releaseID = releaseID
	op.acquired = acquired
	op.rejected = rejected
	op.closed = true
	close(op.done)
	webLockMu.Unlock()
}

func failWebLockOp(opID uint32, err error) {
	webLockMu.Lock()
	op := webLockOps[opID]
	if op == nil || op.closed {
		webLockMu.Unlock()
		return
	}
	op.err = err
	op.closed = true
	close(op.done)
	webLockMu.Unlock()
}

// Exported WebLock callbacks must not block or allocate: they run from
// JavaScript back into TinyGo only to publish primitive completion. The
// original Go caller owns waiting and error construction.
//
//export BLDR_OPFS_WEB_LOCK_RESOLVE
func tinygoWebLockResolve(opID uint32, releaseID uint32, acquired uint32) {
	completeWebLockOp(opID, releaseID, acquired != 0, false)
}

//export BLDR_OPFS_WEB_LOCK_REJECT
func tinygoWebLockReject(opID uint32, _ uint32) {
	completeWebLockOp(opID, 0, false, true)
}

//export BLDR_OPFS_WEB_LOCK_RELEASED
func tinygoWebLockReleased(opID uint32) {
	// Unregister the op. It is absent when Go canceled the request.
	webLockMu.Lock()
	op := webLockOps[opID]
	delete(webLockOps, opID)
	webLockMu.Unlock()

	// Signal that the browser freed the lock.
	if op != nil && op.acquired {
		close(op.released)
	}
}
