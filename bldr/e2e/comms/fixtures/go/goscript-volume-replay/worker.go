//go:build goscript

package goscript_volume_replay

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
)

// workerClient calls a dedicated worker with one message per request. Each
// request is {id, op, ...args}; the worker answers {id, error?, ...result}.
type workerClient struct {
	// worker is the dedicated worker.
	worker js.Value
	// onMessage delivers each response to its caller.
	onMessage js.Func
	// mtx guards the fields below.
	mtx sync.Mutex
	// next is the next request id.
	next int
	// pending maps request ids to their response channels.
	pending map[int]chan js.Value
}

// startWorker starts the module worker at path, relative to this worker's
// origin.
func startWorker(path string) *workerClient {
	url := js.Global().Get("URL").New(path, js.Global().Get("location").Get("href"))
	opts := js.Global().Get("Object").New()
	opts.Set("type", "module")
	w := &workerClient{
		worker:  js.Global().Get("Worker").New(url, opts),
		pending: make(map[int]chan js.Value),
	}
	w.onMessage = js.FuncOf(func(this js.Value, args []js.Value) any {
		resp := args[0].Get("data")
		id := resp.Get("id").Int()
		w.mtx.Lock()
		ch := w.pending[id]
		delete(w.pending, id)
		w.mtx.Unlock()
		if ch != nil {
			ch <- resp
		}
		return nil
	})
	w.worker.Call("addEventListener", "message", w.onMessage)
	return w
}

// call sends the request op with the fields set by args and waits for its
// response.
func (w *workerClient) call(ctx context.Context, op string, args func(req js.Value)) (js.Value, error) {
	ch := make(chan js.Value, 1)
	w.mtx.Lock()
	id := w.next
	w.next++
	w.pending[id] = ch
	w.mtx.Unlock()

	req := js.Global().Get("Object").New()
	req.Set("id", id)
	req.Set("op", op)
	if args != nil {
		args(req)
	}
	w.worker.Call("postMessage", req)

	select {
	case resp := <-ch:
		if msg := resp.Get("error"); !msg.IsUndefined() {
			return js.Undefined(), errors.Errorf("%s: %s", op, msg.String())
		}
		return resp, nil
	case <-ctx.Done():
		w.mtx.Lock()
		delete(w.pending, id)
		w.mtx.Unlock()
		return js.Undefined(), ctx.Err()
	}
}

// terminate stops the worker.
func (w *workerClient) terminate() {
	w.worker.Call("terminate")
	w.onMessage.Release()
}

// toJS copies b into a new Uint8Array.
func toJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// toGo copies a Uint8Array into a new slice.
func toGo(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}
