//go:build !js

package drivebench

import (
	"io/fs"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// Bundle is the served code payload measured from the built bundle on disk: the
// total JavaScript-plus-WASM module bytes, the WASM subtotal, and the module
// file count. It is a bundled-build metric, so only the bundled bench populates
// it. The page resource timeline cannot supply it because the runtime loads its
// modules inside a worker, off the main-thread timeline. The GoScript build is
// all-JavaScript, so WasmBytes is zero for GoScript and non-zero only for the
// TinyGo/Go-WASM builds.
type Bundle struct {
	// TotalBytes is the JavaScript and WASM module bytes.
	TotalBytes int64 `json:"totalBytes"`
	// WasmBytes is the WASM share of TotalBytes.
	WasmBytes int64 `json:"wasmBytes"`
	// FileCount is the number of module files.
	FileCount int `json:"fileCount"`
}

// MeasureBundleDir walks root and sums the served code-payload modules
// (.mjs/.js plus .wasm), reporting the total bytes, the WASM subtotal, and the
// module file count. It measures the built bundle on disk, the size the runtime
// loads, giving the Phase bundle-size baseline a stable number the page resource
// timeline cannot provide.
func MeasureBundleDir(root string) (*Bundle, error) {
	var bundle Bundle
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		// Propagate bundle traversal errors and skip directories.
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Select JavaScript and WASM modules for bundle measurement.
		ext := filepath.Ext(path)
		if ext != ".mjs" && ext != ".js" && ext != ".wasm" {
			return nil
		}

		// Add the module's file size to the bundle totals.
		info, err := d.Info()
		if err != nil {
			return err
		}
		bundle.TotalBytes += info.Size()
		bundle.FileCount++
		if ext == ".wasm" {
			bundle.WasmBytes += info.Size()
		}
		return nil
	})
	if walkErr != nil {
		return nil, errors.Wrap(walkErr, "walk bundle dir")
	}
	return &bundle, nil
}

func marshalBundleValue(arena *fastjson.Arena, bundle Bundle) *fastjson.Value {
	// Encode the served bundle's byte totals and file count.
	obj := arena.NewObject()
	obj.Set("totalBytes", arena.NewNumberString(strconv.FormatInt(bundle.TotalBytes, 10)))
	obj.Set("wasmBytes", arena.NewNumberString(strconv.FormatInt(bundle.WasmBytes, 10)))
	obj.Set("fileCount", arena.NewNumberInt(bundle.FileCount))
	return obj
}
