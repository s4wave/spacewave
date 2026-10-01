//go:build js && !tinygo

package opfs

import (
	"context"
	"sync/atomic"
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
	// Outcome is the terminal outcome of the request.
	Outcome WebLockOutcome
	// Release asks the browser to release an acquired lock. It returns before
	// the browser frees the lock.
	Release func()
	// Released closes once the browser has freed an acquired lock after
	// Release, so another request for the name can acquire it.
	Released <-chan struct{}
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

// acquireWebLock requests a Web Lock through the runtime helper when one is
// installed, and through navigator.locks otherwise.
func (d BrowserDriver) acquireWebLock(ctx context.Context, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
	// Report an already canceled ctx without a request.
	if err := ctx.Err(); err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, err
	}

	// Prefer the runtime helper, which accounts the request to its runtime.
	helper := js.Global().Get(acquireWebLockHelper)
	if helper.Type() == js.TypeFunction {
		return d.acquireWebLockWithHelper(ctx, helper, name, exclusive, ifAvailable)
	}

	// The lock callback holds the lock until Release resolves its promise.
	acquiredCh := make(chan struct{})
	var resolveFunc js.Value
	acquired := true
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

	// Build the request options, aborting a blocking request with ctx.
	opts := jsutil.NewObject()
	opts.Set("mode", webLockMode(exclusive))
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

	// Request the lock, recording a rejection before acquisition.
	var requestErr error
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

	// Wait for the request to acquire, decline, or reject.
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

	// The request promise settles after the browser frees the lock.
	released := make(chan struct{})
	var releasedCb js.Func
	releasedCb = js.FuncOf(func(js.Value, []js.Value) any {
		close(released)
		releasedCb.Release()
		return nil
	})
	jsutil.Call(promise, "then", releasedCb, releasedCb)

	// Release resolves the lock callback promise once.
	var releaseOnce atomic.Bool
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
			// Release at most once.
			if !releaseOnce.CompareAndSwap(false, true) {
				return
			}

			// Resolve the lock callback promise and free the callbacks.
			if resolveFunc.Type() != js.TypeFunction {
				panic("weblock release callback unavailable")
			}
			resolveFunc.Invoke()
			executorCb.Release()
			lockCb.Release()
		},
		Released: released,
	}, nil
}

// acquireWebLockWithHelper requests a Web Lock through the runtime helper.
func (d BrowserDriver) acquireWebLockWithHelper(ctx context.Context, helper js.Value, name string, exclusive, ifAvailable bool) (*WebLockResult, error) {
	// The helper calls exactly one of resolve or reject.
	done := make(chan struct{})
	var releaseFunc js.Value
	var acquired bool
	var err error
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
	defer resolveCb.Release()
	defer rejectCb.Release()

	// Request the lock and wait for the outcome or cancellation.
	helper.Invoke(name, webLockMode(exclusive), ifAvailable, resolveCb, rejectCb)
	select {
	case <-done:
	case <-ctx.Done():
		return &WebLockResult{Outcome: WebLockOutcomeCanceled}, ctx.Err()
	}
	if err != nil {
		return &WebLockResult{Outcome: WebLockOutcomeRejected}, err
	}
	if !acquired {
		return &WebLockResult{Outcome: WebLockOutcomeUnavailable}, nil
	}

	// The helper release returns a promise that settles once the browser frees
	// the lock.
	released := make(chan struct{})
	var releaseOnce atomic.Bool
	return &WebLockResult{
		Outcome: WebLockOutcomeAcquired,
		Release: func() {
			// Release at most once.
			if !releaseOnce.CompareAndSwap(false, true) {
				return
			}
			if releaseFunc.Type() != js.TypeFunction {
				panic("weblock release callback unavailable")
			}

			// Call the helper release and close released when its freed
			// promise settles.
			var releasedCb js.Func
			releasedCb = js.FuncOf(func(js.Value, []js.Value) any {
				close(released)
				releasedCb.Release()
				return nil
			})
			jsutil.Call(releaseFunc.Invoke(), "then", releasedCb, releasedCb)
		},
		Released: released,
	}, nil
}

// webLockMode returns the Web Locks mode name.
func webLockMode(exclusive bool) string {
	if exclusive {
		return "exclusive"
	}
	return "shared"
}
