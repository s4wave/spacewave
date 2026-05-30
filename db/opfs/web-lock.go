//go:build js && !tinygo

package opfs

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

const acquireWebLockHelper = "BLDR_OPFS_ACQUIRE_WEB_LOCK"

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

func (d BrowserDriver) acquireWebLock(ctx context.Context, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
	if err := ctx.Err(); err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, err
	}

	helper := js.Global().Get(acquireWebLockHelper)
	if helper.Type() == js.TypeFunction {
		return d.acquireWebLockWithHelper(ctx, helper, name, exclusive, ifAvailable)
	}

	acquiredCh := make(chan struct{})
	var resolveFunc js.Value
	acquired := true
	var requestErr error

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
	abortController := js.Global().Get("AbortController")
	if !ifAvailable && !abortController.IsUndefined() && !abortController.IsNull() {
		ctrl := abortController.New()
		opts.Set("signal", ctrl.Get("signal"))
		go func() {
			<-ctx.Done()
			ctrl.Call("abort")
		}()
	}

	promise := jsutil.Call(js.Global().Get("navigator").Get("locks"), "request", name, opts, lockCb)
	catchCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			requestErr = newJSError(args[0])
		} else {
			requestErr = errors.New("WebLock request rejected")
		}
		select {
		case <-acquiredCh:
		default:
			close(acquiredCh)
		}
		return nil
	})
	defer catchCb.Release()
	jsutil.Call(promise, "catch", catchCb)

	<-acquiredCh
	if requestErr != nil {
		lockCb.Release()
		if ctx.Err() != nil {
			return &WebLockResult{Outcome: WebLockOutcomeCanceled}, ctx.Err()
		}
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, requestErr
	}
	if !acquired {
		lockCb.Release()
		return &WebLockResult{Outcome: WebLockOutcomeUnavailable}, nil
	}

	var releaseOnce sync.Once
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
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
		},
	}, nil
}

func (d BrowserDriver) acquireWebLockWithHelper(ctx context.Context, helper js.Value, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
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
	select {
	case <-done:
	case <-ctx.Done():
		resolveCb.Release()
		rejectCb.Release()
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, ctx.Err()
	}
	resolveCb.Release()
	rejectCb.Release()

	if err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, err
	}
	if !acquired {
		return &WebLockResult{Outcome: WebLockOutcomeUnavailable}, nil
	}

	var releaseOnce sync.Once
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
			releaseOnce.Do(func() {
				if releaseFunc.IsUndefined() || releaseFunc.IsNull() || releaseFunc.Type() != js.TypeFunction {
					panic("weblock release callback unavailable")
				}
				releaseFunc.Invoke()
			})
		},
	}, nil
}
