//go:build goscript

package goscript_opfs_storage

import (
	"context"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
)

const (
	// dirName isolates the fixture's saved bytes from other browser tests.
	dirName = "goscript-opfs-storage-proof"
	// fileName contains the raw OPFS persistence proof.
	fileName = "data.txt"
	// payload is the expected raw and volume metadata content.
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

	// Execute one side of the worker restart persistence check.
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

	// Publish readiness only after all persistence checks succeed.
	if err := markReady(); err != nil {
		postFailure(err)
		return
	}
	postMessage(map[string]any{"type": "opfs-done", "mode": mode})
}

// readMode reads the worker's requested persistence phase.
func readMode() string {
	encoded := js.Global().Get("BLDR_PLUGIN_START_INFO")
	if encoded.IsUndefined() || encoded.IsNull() {
		return ""
	}
	jsonText := js.Global().Call("atob", encoded.String())
	parsed := js.Global().Get("JSON").Call("parse", jsonText)
	return parsed.Get("instanceKey").String()
}

// writeProofData seeds legacy bytes and writes through the recovered volume.
func writeProofData() error {
	// Start with a disposable fixture directory and recognizable saved data.
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
	if err := opfs.WriteFile(dir, fileName, []byte(payload)); err != nil {
		return err
	}
	legacy, err := opfs.GetDirectory(dir, "volume", true)
	if err != nil {
		return err
	}
	if err := opfs.WriteFile(legacy, ".spacewave-opfs-format", []byte("spacewave-opfs-volume/1\n")); err != nil {
		return err
	}
	if err := opfs.WriteFile(legacy, fileName, []byte(payload)); err != nil {
		return err
	}

	// A legacy root must automatically mount a new writable volume.
	ctx := context.Background()
	vol, err := volume_opfs.NewOpfs(ctx, nil, &volume_opfs.Config{RootPath: dirName + "/volume"})
	if err != nil {
		return err
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(fileName), []byte(payload)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if _, err := vol.Sync(ctx); err != nil {
		return err
	}
	return opfs.WriteFile(dir, "volume-id", []byte(vol.GetID()))
}

// readProofData verifies remount identity, metadata, and legacy preservation on deletion.
func readProofData() error {
	// Verify the raw file survives the worker restart.
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

	// Reopening the configured path must retain the replacement's identity and data.
	ctx := context.Background()
	vol, err := volume_opfs.NewOpfs(ctx, nil, &volume_opfs.Config{RootPath: dirName + "/volume"})
	if err != nil {
		return err
	}
	defer vol.Close()

	// The replacement keeps its volume identity.
	id, err := opfs.ReadFile(dir, "volume-id")
	if err != nil {
		return err
	}
	if string(id) != vol.GetID() {
		return errors.New("replacement volume identity changed on remount")
	}

	// The replacement keeps the metadata written before the restart.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	data, found, err := tx.Get(ctx, []byte(fileName))
	tx.Discard()
	if err != nil {
		return err
	}
	if !found || string(data) != payload {
		return errors.New("replacement metadata did not survive worker restart")
	}

	// Deleting the active volume must retain all legacy bytes.
	if err := vol.Delete(); err != nil {
		return err
	}
	if _, err := opfs.GetDirectory(dir, "volume.spacewave-opfs-v4", false); !opfs.IsNotFound(err) {
		return errors.Errorf("replacement remains after Delete: %v", err)
	}

	// Recreate the active volume and exercise deletion without a mounted handle.
	reopened, err := volume_opfs.NewOpfs(ctx, nil, &volume_opfs.Config{RootPath: dirName + "/volume"})
	if err != nil {
		return err
	}
	if err := reopened.Close(); err != nil {
		return err
	}

	// DeleteRoot removes the replacement and is safe to repeat.
	if err := volume_opfs.DeleteRoot(dirName + "/volume"); err != nil {
		return err
	}
	if err := volume_opfs.DeleteRoot(dirName + "/volume"); err != nil {
		return errors.Wrap(err, "repeat replacement deletion")
	}
	if _, err := opfs.GetDirectory(dir, "volume.spacewave-opfs-v4", false); !opfs.IsNotFound(err) {
		return errors.Errorf("replacement remains after DeleteRoot: %v", err)
	}

	// Both deletion APIs must leave the incompatible root available for recovery.
	legacy, err := opfs.GetDirectory(dir, "volume", false)
	if err != nil {
		return err
	}
	data, err = opfs.ReadFile(legacy, fileName)
	if err != nil {
		return err
	}
	if string(data) != payload {
		return errors.New("legacy data changed")
	}

	// The legacy root keeps its original format marker.
	marker, err := opfs.ReadFile(legacy, ".spacewave-opfs-format")
	if err != nil {
		return err
	}
	if string(marker) != "spacewave-opfs-volume/1\n" {
		return errors.New("legacy format marker changed")
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
