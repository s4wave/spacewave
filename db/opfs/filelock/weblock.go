//go:build js && !tinygo

package filelock

import (
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

const acquireWebLockHelper = "BLDR_OPFS_ACQUIRE_WEB_LOCK"

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
	helper := js.Global().Get(acquireWebLockHelper)
	if helper.Type() == js.TypeFunction {
		return acquireWebLockWithHelper(helper, name, exclusive, ifAvailable)
	}

	acquiredCh := make(chan struct{})
	var resolveFunc js.Value
	acquired := true

	mode := "shared"
	if exclusive {
		mode = "exclusive"
	}

	var executorCb js.Func
	lockCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if ifAvailable && (len(args) == 0 || args[0].IsNull() || args[0].IsUndefined()) {
			acquired = false
			close(acquiredCh)
			return nil
		}
		executorCb = js.FuncOf(func(this js.Value, pArgs []js.Value) any {
			resolveFunc = pArgs[0]
			close(acquiredCh)
			return nil
		})
		return jsutil.NewPromise(executorCb)
	})

	opts := jsutil.NewObject()
	opts.Set("mode", mode)
	if ifAvailable {
		opts.Set("ifAvailable", true)
	}

	jsutil.Call(js.Global().Get("navigator").Get("locks"), "request", name, opts, lockCb)
	<-acquiredCh
	if !acquired {
		lockCb.Release()
		return nil, false, nil
	}

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			if resolveFunc.IsUndefined() || resolveFunc.IsNull() || resolveFunc.Type() != js.TypeFunction {
				panic("weblock release callback unavailable")
			}
			defer func() {
				if e := recover(); e != nil {
					panic("weblock release invoke failed")
				}
			}()
			resolveFunc.Invoke()
			executorCb.Release()
			lockCb.Release()
		})
	}, true, nil
}

func acquireWebLockWithHelper(helper js.Value, name string, exclusive, ifAvailable bool) (func(), bool, error) {
	done := make(chan struct{})
	var releaseFunc js.Value
	var acquired bool
	var err error

	mode := "shared"
	if exclusive {
		mode = "exclusive"
	}

	resolveCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 2 && args[1].Type() == js.TypeBoolean {
			acquired = args[1].Bool()
		}
		if acquired {
			releaseFunc = args[0]
		}
		close(done)
		return nil
	})
	rejectCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		err = errors.New("WebLock request rejected")
		close(done)
		return nil
	})

	helper.Invoke(name, mode, ifAvailable, resolveCb, rejectCb)
	<-done
	resolveCb.Release()
	rejectCb.Release()

	if err != nil {
		return nil, false, err
	}
	if !acquired {
		return nil, false, nil
	}

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			if releaseFunc.IsUndefined() || releaseFunc.IsNull() || releaseFunc.Type() != js.TypeFunction {
				panic("weblock release callback unavailable")
			}
			releaseFunc.Invoke()
		})
	}, true, nil
}
