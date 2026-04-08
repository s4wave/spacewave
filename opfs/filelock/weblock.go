//go:build js

package filelock

import (
	"sync"
	"syscall/js"
)

// AcquireWebLock requests a WebLock via navigator.locks.request.
// The returned function releases the lock. It is safe to call once.
func AcquireWebLock(name string, exclusive bool) (func(), error) {
	acquiredCh := make(chan struct{})
	var resolveFunc js.Value

	mode := "shared"
	if exclusive {
		mode = "exclusive"
	}

	var executorCb js.Func
	lockCb := js.FuncOf(func(this js.Value, args []js.Value) any {
		executorCb = js.FuncOf(func(this js.Value, pArgs []js.Value) any {
			resolveFunc = pArgs[0]
			close(acquiredCh)
			return nil
		})
		return js.Global().Get("Promise").New(executorCb)
	})

	opts := js.Global().Get("Object").New()
	opts.Set("mode", mode)

	js.Global().Get("navigator").Get("locks").Call("request", name, opts, lockCb)
	<-acquiredCh

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			resolveFunc.Invoke()
			executorCb.Release()
			lockCb.Release()
		})
	}, nil
}
