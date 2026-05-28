//go:build js && tinygo

package opfs

import (
	"context"
	"sync"
	"unsafe"

	"github.com/pkg/errors"
)

type webLockOp struct {
	done      chan struct{}
	releaseID uint32
	acquired  bool
	err       error
	rejected  bool
	closed    bool
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
	Outcome WebLockOutcome
	Release func()
}

//go:wasmimport gojs bldr.opfs.acquireWebLock
func tinygoAcquireWebLock(opID uint32, namePtr unsafe.Pointer, nameLen uint32, exclusive uint32, ifAvailable uint32)

//go:wasmimport gojs bldr.opfs.cancelWebLock
func tinygoCancelWebLock(opID uint32) uint32

//go:wasmimport gojs bldr.opfs.releaseWebLock
func tinygoReleaseWebLock(releaseID uint32) uint32

// AcquireWebLock requests a Web Lock and waits until it is acquired.
func AcquireWebLock(name string, exclusive bool) (func(), error) {
	result, err := DefaultDriver.AcquireWebLock(context.Background(), name, exclusive)
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

func (BrowserDriver) acquireWebLock(ctx context.Context, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
	if err := ctx.Err(); err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, err
	}
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
	select {
	case <-op.done:
	case <-ctx.Done():
		tinygoCancelWebLock(opID)
		deleteWebLockOp(opID)
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, ctx.Err()
	}
	deleteWebLockOp(opID)
	if op.err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, op.err
	}
	if op.rejected {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, errors.New("WebLock request rejected")
	}
	if !op.acquired {
		return &WebLockResult{Outcome: WebLockOutcomeUnavailable}, nil
	}

	var releaseOnce sync.Once
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
			releaseOnce.Do(func() {
				if tinygoReleaseWebLock(op.releaseID) == 0 {
					panic("weblock release callback unavailable")
				}
			})
		},
	}, nil
}

func registerWebLockOp() (uint32, *webLockOp) {
	webLockMu.Lock()
	defer webLockMu.Unlock()
	nextWebLockID++
	if nextWebLockID == 0 {
		nextWebLockID++
	}
	op := &webLockOp{done: make(chan struct{})}
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
