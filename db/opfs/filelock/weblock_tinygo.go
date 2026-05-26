//go:build js && tinygo

package filelock

import (
	"sync"
	"unsafe"

	"github.com/pkg/errors"
)

type webLockOp struct {
	done      chan struct{}
	releaseID uint32
	acquired  bool
	err       error
	closed    bool
}

var (
	webLockMu     sync.Mutex
	nextWebLockID uint32
	webLockOps    = make(map[uint32]*webLockOp)
)

//go:wasmimport gojs bldr.opfs.acquireWebLock
func tinygoAcquireWebLock(opID uint32, namePtr unsafe.Pointer, nameLen uint32, exclusive uint32, ifAvailable uint32)

//go:wasmimport gojs bldr.opfs.releaseWebLock
func tinygoReleaseWebLock(releaseID uint32) uint32

// AcquireWebLock requests a WebLock via navigator.locks.request.
// The returned function releases the lock. It is safe to call once.
func AcquireWebLock(name string, exclusive bool) (func(), error) {
	release, acquired, err := acquireWebLock(name, exclusive, false)
	if err != nil {
		return nil, err
	}
	if !acquired {
		panic("blocking WebLock request returned without acquiring")
	}
	return release, nil
}

// AcquireWebLockIfAvailable requests a WebLock without waiting.
// If the lock is unavailable, acquired is false and release is nil.
func AcquireWebLockIfAvailable(name string, exclusive bool) (release func(), acquired bool, err error) {
	return acquireWebLock(name, exclusive, true)
}

func acquireWebLock(name string, exclusive, ifAvailable bool) (func(), bool, error) {
	opID, op := registerWebLockOp()
	nameBytes := []byte(name)
	if len(nameBytes) == 0 {
		completeWebLockOp(opID, 0, false, errors.New("WebLock name is empty"))
	} else {
		tinygoAcquireWebLock(
			opID,
			unsafe.Pointer(&nameBytes[0]),
			uint32(len(nameBytes)),
			boolUint32(exclusive),
			boolUint32(ifAvailable),
		)
	}
	<-op.done
	deleteWebLockOp(opID)
	if op.err != nil {
		return nil, false, op.err
	}
	if !op.acquired {
		return nil, false, nil
	}

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			if tinygoReleaseWebLock(op.releaseID) == 0 {
				panic("weblock release callback unavailable")
			}
		})
	}, true, nil
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

func completeWebLockOp(opID, releaseID uint32, acquired bool, err error) {
	webLockMu.Lock()
	op := webLockOps[opID]
	if op == nil || op.closed {
		webLockMu.Unlock()
		return
	}
	op.releaseID = releaseID
	op.acquired = acquired
	op.err = err
	op.closed = true
	close(op.done)
	webLockMu.Unlock()
}

func boolUint32(v bool) uint32 {
	if v {
		return 1
	}
	return 0
}

// Exported WebLock callbacks must not block: they run from JavaScript back
// into TinyGo only to publish completion. The original Go caller owns waiting.
//
//go:wasmexport BLDR_OPFS_WEB_LOCK_RESOLVE
func tinygoWebLockResolve(opID uint32, releaseID uint32, acquired uint32) {
	completeWebLockOp(opID, releaseID, acquired != 0, nil)
}

//go:wasmexport BLDR_OPFS_WEB_LOCK_REJECT
func tinygoWebLockReject(opID uint32, _ uint32) {
	completeWebLockOp(opID, 0, false, errors.New("WebLock request rejected"))
}
