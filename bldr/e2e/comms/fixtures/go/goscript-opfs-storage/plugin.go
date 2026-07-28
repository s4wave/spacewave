//go:build goscript

package goscript_opfs_storage

import (
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
)

const (
	dirName  = "goscript-opfs-storage-proof"
	fileName = "data.txt"
	payload  = "hello from goscript opfs"
)

func main() {
	go run()
	select {}
}

func run() {
	defer func() {
		if recovered := recover(); recovered != nil {
			postFailure(errors.Errorf("panic: %v", recovered))
		}
	}()

	mode := readMode()
	var err error
	switch mode {
	case "write":
		err = writeProofData()
	case "read":
		err = readProofData()
	default:
		err = errors.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		postFailure(err)
		return
	}
	if err := markReady(); err != nil {
		postFailure(err)
		return
	}
	postMessage(map[string]any{"type": "opfs-done", "mode": mode})
}

func readMode() string {
	encoded := js.Global().Get("BLDR_PLUGIN_START_INFO")
	if encoded.IsUndefined() || encoded.IsNull() {
		return ""
	}
	jsonText := js.Global().Call("atob", encoded.String())
	parsed := js.Global().Get("JSON").Call("parse", jsonText)
	return parsed.Get("instanceKey").String()
}

func writeProofData() error {
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	if err := opfs.DeleteEntry(root, dirName, true); err != nil && !opfs.IsNotFound(err) {
		return err
	}
	dir, err := opfs.GetDirectory(root, dirName, true)
	if err != nil {
		return err
	}
	return opfs.WriteFile(dir, fileName, []byte(payload))
}

func readProofData() error {
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	dir, err := opfs.GetDirectory(root, dirName, false)
	if err != nil {
		return err
	}
	data, err := opfs.ReadFile(dir, fileName)
	if err != nil {
		return err
	}
	if string(data) != payload {
		return errors.Errorf("read %q, want %q", string(data), payload)
	}
	return opfs.DeleteEntry(root, dirName, true)
}

func markReady() error {
	ready := js.Global().Get("BLDR_PLUGIN_MARK_READY")
	if ready.IsUndefined() || ready.IsNull() || ready.Type() != js.TypeFunction {
		return errors.New("BLDR_PLUGIN_MARK_READY is not a function")
	}
	ready.Invoke()
	return nil
}

func postFailure(err error) {
	postMessage(map[string]any{
		"type":          "opfs-failed",
		"failureReason": err.Error(),
	})
}

func postMessage(msg map[string]any) {
	js.Global().Call("postMessage", js.ValueOf(msg))
}
