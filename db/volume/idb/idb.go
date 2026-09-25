//go:build js

// Package volume_idb implements the browser volume engines' storage
// primitives on IndexedDB: a chunked device.Device and a records.Store.
//
// Every call is one IndexedDB transaction. A call issues all of its requests
// in one JavaScript task and returns on the transaction's complete event, so
// the Go code blocks only once per call.
package volume_idb

import (
	"syscall/js"

	"github.com/pkg/errors"
)

// schemaVersion is the database version whose upgrade creates the stores.
const schemaVersion = 1

// openDB opens the database name, creating stores on first open.
func openDB(name string, stores ...string) (js.Value, error) {
	req := js.Global().Get("indexedDB").Call("open", name, schemaVersion)
	upgrade := js.FuncOf(func(this js.Value, args []js.Value) any {
		db := req.Get("result")
		for _, store := range stores {
			db.Call("createObjectStore", store)
		}
		return nil
	})
	defer upgrade.Release()
	req.Set("onupgradeneeded", upgrade)
	if err := wait(req, "success", "error", "blocked"); err != nil {
		return js.Undefined(), errors.Wrapf(err, "open %s", name)
	}
	return req.Get("result"), nil
}

// DeleteDatabase deletes the database name. A missing database is not an
// error.
func DeleteDatabase(name string) error {
	req := js.Global().Get("indexedDB").Call("deleteDatabase", name)
	return errors.Wrapf(wait(req, "success", "error", "blocked"), "delete %s", name)
}

// transaction starts a transaction on stores. A write transaction is strict
// when durable is set and relaxed otherwise.
func transaction(db js.Value, write, durable bool, stores ...string) js.Value {
	names := make([]any, len(stores))
	for i, store := range stores {
		names[i] = store
	}
	if !write {
		return db.Call("transaction", names, "readonly")
	}
	opts := js.Global().Get("Object").New()
	opts.Set("durability", "relaxed")
	if durable {
		opts.Set("durability", "strict")
	}
	return db.Call("transaction", names, "readwrite", opts)
}

// complete waits for a transaction to commit.
func complete(tx js.Value) error {
	return wait(tx, "complete", "error", "abort")
}

// wait blocks until target fires the ok event or a failure event, returning
// the target's error for a failure.
func wait(target js.Value, ok string, fail ...string) error {
	done := make(chan string, 1)
	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case done <- args[0].Get("type").String():
		default:
		}
		return nil
	})
	defer handler.Release()
	events := append([]string{ok}, fail...)
	for _, ev := range events {
		target.Call("addEventListener", ev, handler)
	}
	fired := <-done
	for _, ev := range events {
		target.Call("removeEventListener", ev, handler)
	}
	if fired == ok {
		return nil
	}
	return errors.Errorf("%s: %s", fired, errorText(target.Get("error")))
}

// errorText describes a DOMException or an absent error.
func errorText(err js.Value) string {
	if err.IsNull() || err.IsUndefined() {
		return "no error"
	}
	return err.Get("name").String() + ": " + err.Get("message").String()
}

// toJS copies b into a new Uint8Array.
func toJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// toGo copies a Uint8Array or ArrayBuffer into a new slice.
func toGo(v js.Value) []byte {
	if v.InstanceOf(js.Global().Get("ArrayBuffer")) {
		v = js.Global().Get("Uint8Array").New(v)
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}
