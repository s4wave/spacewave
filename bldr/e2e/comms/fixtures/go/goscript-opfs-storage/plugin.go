//go:build goscript

package goscript_opfs_storage

import (
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
)

const (
	// dirName isolates the fixture's saved bytes from other browser tests.
	dirName = "goscript-opfs-storage-proof"
	// fileName contains the raw OPFS persistence proof.
	fileName = "data.txt"
	// payload is the raw file content.
	payload = "hello from goscript opfs"
)

// main keeps the GoScript worker alive while the asynchronous probe runs.
func main() {
	go run()
	select {}
}

// run reports completion or a foreign JavaScript exception to the harness.
func run() {
	// Translate foreign JavaScript throws into the fixture's failure message.
	defer func() {
		if recovered := recover(); recovered != nil {
			postFailure(errors.Errorf("panic: %v", recovered))
		}
	}()

	// Execute one side of the worker restart persistence checks.
	mode := readMode()
	var err error
	switch mode {
	case "write":
		err = writeProofData()
		if err == nil {
			err = writeVolumes()
		}
	case "read":
		err = readProofData()
		if err == nil {
			err = readVolumes()
		}
	default:
		err = errors.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		postFailure(err)
		return
	}

	// Publish readiness only after all persistence checks succeed.
	if err := markReady(); err != nil {
		postFailure(err)
		return
	}
	postMessage(map[string]any{"type": "opfs-done", "mode": mode})
}

// readMode reads the worker's requested persistence phase.
func readMode() string {
	// Decode the start info the harness passes the worker.
	encoded := js.Global().Get("BLDR_PLUGIN_START_INFO")
	if encoded.IsUndefined() || encoded.IsNull() {
		return ""
	}
	jsonText := js.Global().Call("atob", encoded.String())
	parsed := js.Global().Get("JSON").Call("parse", jsonText)
	return parsed.Get("instanceKey").String()
}

// writeProofData writes the raw OPFS file the restart checks.
func writeProofData() error {
	// Start with a disposable fixture directory holding recognizable bytes.
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

// readProofData checks that the raw OPFS file survived the worker restart,
// then deletes the fixture directory.
func readProofData() error {
	// Read the file written before the restart.
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

// markReady completes the production worker readiness handshake.
func markReady() error {
	ready := js.Global().Get("BLDR_PLUGIN_MARK_READY")
	if ready.IsUndefined() || ready.IsNull() || ready.Type() != js.TypeFunction {
		return errors.New("BLDR_PLUGIN_MARK_READY is not a function")
	}
	ready.Invoke()
	return nil
}

// postFailure sends the probe error to the browser harness.
func postFailure(err error) {
	postMessage(map[string]any{
		"type":          "opfs-failed",
		"failureReason": err.Error(),
	})
}

// postMessage sends one fixture result through the worker message boundary.
func postMessage(msg map[string]any) {
	js.Global().Call("postMessage", js.ValueOf(msg))
}
