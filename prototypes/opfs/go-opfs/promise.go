//go:build js && wasm

package opfs

import (
	"github.com/hack-pad/safejs"
)

// awaitPromise blocks until a JS Promise resolves or rejects.
func awaitPromise(promise safejs.Value) (safejs.Value, error) {
	resCh := make(chan safejs.Value, 1)
	errCh := make(chan error, 1)

	onResolve, err := safejs.FuncOf(func(_ safejs.Value, args []safejs.Value) any {
		if len(args) > 0 {
			resCh <- args[0]
		} else {
			resCh <- safejs.Undefined()
		}
		return nil
	})
	if err != nil {
		return safejs.Undefined(), err
	}
	defer onResolve.Release()

	onReject, err := safejs.FuncOf(func(_ safejs.Value, args []safejs.Value) any {
		if len(args) > 0 {
			msg, msgErr := args[0].Call("toString")
			if msgErr != nil {
				errCh <- msgErr
			} else {
				str, strErr := msg.String()
				if strErr != nil {
					errCh <- strErr
				} else {
					errCh <- &DOMError{Message: str}
				}
			}
		} else {
			errCh <- &DOMError{Message: "promise rejected"}
		}
		return nil
	})
	if err != nil {
		return safejs.Undefined(), err
	}
	defer onReject.Release()

	_, err = promise.Call("then", onResolve, onReject)
	if err != nil {
		return safejs.Undefined(), err
	}

	select {
	case val := <-resCh:
		return val, nil
	case err := <-errCh:
		return safejs.Undefined(), err
	}
}
