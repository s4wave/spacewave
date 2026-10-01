//go:build js

package volume_opfs

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
	"github.com/sirupsen/logrus"
)

// StorageFormatVersion is the engine format the runtime frames each root with.
const StorageFormatVersion uint32 = engine.FormatVersion

var (
	// formatMarker is fixed framing, not a second mutable storage descriptor.
	formatMarker = "spacewave-opfs-volume/" + strconv.Itoa(engine.FormatVersion) + "\n"
	// recoverySuffix reserves a sibling root while preserving incompatible data.
	recoverySuffix = ".spacewave-opfs-v" + strconv.Itoa(engine.FormatVersion)
)

const (
	// formatMarkerName distinguishes initialized roots from preexisting data.
	formatMarkerName = ".spacewave-opfs-format"
	// driverModeAuto selects the current runtime's supported browser driver.
	driverModeAuto = "auto"
	// driverModeStandardWasm selects standard Go's browser driver ABI.
	driverModeStandardWasm = "standard-wasm"
	// driverModeTinyGo selects TinyGo's browser driver ABI.
	driverModeTinyGo = "tinygo"
)

// runtimeDriverMode resolves the driver selection for the current runtime.
func runtimeDriverMode(conf *Config) string {
	if mode := conf.GetDriverMode(); mode != "" {
		return mode
	}
	return driverModeAuto
}

// openRuntimeRoot opens the active directory and returns its physical path.
// Incompatible roots remain intact beside a stable current-format replacement.
func openRuntimeRoot(ctx context.Context, le *logrus.Entry, root js.Value, conf *Config) (js.Value, string, error) {
	// Serialize root selection and initialization across mounting workers.
	parts, _ := unixfs.SplitPath(conf.GetRootPath())
	if len(parts) == 0 {
		return js.Undefined(), "", errors.New("root_path must name an OPFS directory")
	}
	prefix := conf.GetLockPrefix()
	if prefix == "" {
		prefix = conf.GetRootPath()
	}
	lock, err := opfs.DefaultDriver.AcquireWebLock(ctx, prefix+"/engine/publish", true)
	if err != nil {
		return js.Undefined(), "", err
	}
	if lock.Outcome != opfs.WebLockOutcomeAcquired {
		return js.Undefined(), "", errors.New("OPFS format lock was not acquired")
	}
	defer lock.Release()

	// Prefer an existing replacement, including an interrupted initialization.
	rootPath := conf.GetRootPath()
	recoveryParts := slices.Clone(parts)
	recoveryParts[len(recoveryParts)-1] += recoverySuffix
	dir, err := opfs.GetDirectoryPath(root, recoveryParts, false)
	if err == nil {
		return dir, strings.Join(recoveryParts, "/"), initializeRuntimeRoot(dir)
	}
	if !opfs.IsNotFound(err) {
		return js.Undefined(), "", err
	}

	// Initialize the configured root only when it contains no saved data.
	dir, err = opfs.GetDirectoryPath(root, parts, true)
	if err != nil {
		return js.Undefined(), "", err
	}
	err = initializeRuntimeRoot(dir)
	if !errors.Is(err, errIncompatibleFormat) {
		return dir, rootPath, err
	}

	// Preserve the legacy directory and create one deterministic replacement.
	dir, err = opfs.GetDirectoryPath(root, recoveryParts, true)
	if err != nil {
		return js.Undefined(), "", err
	}
	rootPath = strings.Join(recoveryParts, "/")
	if err := initializeRuntimeRoot(dir); err != nil {
		return js.Undefined(), "", err
	}
	if le != nil {
		le.WithField("legacy_root_path", conf.GetRootPath()).WithField("root_path", rootPath).
			Warn("opened replacement OPFS volume; incompatible data preserved in legacy root")
	}
	return dir, rootPath, nil
}

// checkRuntimeRoot reports current framing or an empty, initializable directory.
func checkRuntimeRoot(dir js.Value) (bool, error) {
	// Read compatible framing without enumerating stored entries.
	marker, err := opfs.ReadFile(dir, formatMarkerName)
	emptyMarker := err == nil && len(marker) == 0
	if err == nil {
		if bytes.Equal(marker, []byte(formatMarker)) {
			return true, nil
		}
		if len(marker) != 0 {
			return false, errIncompatibleFormat
		}
	}
	if err != nil && !opfs.IsNotFound(err) {
		return false, err
	}

	// Listing is confined to first initialization, never ordinary compatible open.
	names, err := opfs.ListDirectory(dir)
	if err != nil {
		return false, err
	}
	// An interrupted first marker write may leave its newly created empty entry.
	if len(names) != 0 && !(len(names) == 1 && names[0] == formatMarkerName && emptyMarker) {
		return false, errIncompatibleFormat
	}
	return false, nil
}

// initializeRuntimeRoot writes current framing only into an empty directory.
func initializeRuntimeRoot(dir js.Value) error {
	if initialized, err := checkRuntimeRoot(dir); initialized || err != nil {
		return err
	}
	return opfs.WriteFile(dir, formatMarkerName, []byte(formatMarker))
}

// errIncompatibleFormat stops retries when the reserved replacement is incompatible.
var errIncompatibleFormat = volume.Permanent(errors.New("incompatible OPFS volume format: use a new empty volume or explicitly export and import the existing data"))

// DeleteRoot deletes an unmounted active volume, preserving a replaced legacy root.
// Callers must release all mounted users before deleting the volume.
func DeleteRoot(rootPath string) error {
	// Resolve the same reserved replacement used when mounting the volume.
	parts, _ := unixfs.SplitPath(rootPath)
	if len(parts) == 0 {
		return errors.New("root_path must name an OPFS directory")
	}
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	parts[len(parts)-1] += recoverySuffix
	_, err = opfs.GetDirectoryPath(root, parts, false)
	if err == nil {
		return deleteRuntimeRoot(root, strings.Join(parts, "/"))
	}
	if !opfs.IsNotFound(err) {
		return err
	}

	// An absent replacement must not make a repeated deletion erase legacy data.
	parts, _ = unixfs.SplitPath(rootPath)
	dir, err := opfs.GetDirectoryPath(root, parts, false)
	if opfs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = checkRuntimeRoot(dir)
	if errors.Is(err, errIncompatibleFormat) {
		return nil
	}
	if err != nil {
		return err
	}
	return deleteRuntimeRoot(root, rootPath)
}

// deleteRuntimeRoot performs the Volume interface's explicit destructive operation.
func deleteRuntimeRoot(root js.Value, rootPath string) error {
	parts, _ := unixfs.SplitPath(rootPath)
	if len(parts) == 0 {
		return errors.New("root_path must name an OPFS directory")
	}
	parent := root
	for _, part := range parts[:len(parts)-1] {
		next, err := opfs.GetDirectory(parent, part, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				return nil
			}
			return err
		}
		parent = next
	}
	err := opfs.DeleteEntry(parent, parts[len(parts)-1], true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	return nil
}
