//go:build js

package volume_opfs

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

const (
	currentStorageFormatVersion uint32 = 2
	formatMarkerName                   = ".spacewave-opfs-format.json"
	formatMarkerKind                   = "spacewave-opfs-volume"

	driverModeAuto         = "auto"
	driverModeStandardWasm = "standard-wasm"
	driverModeTinyGo       = "tinygo"

	resetPolicyAutomatic = "automatic"
)

// ResetReason identifies why the OPFS Volume Runtime initialized a clean root.
type ResetReason string

const (
	ResetReasonMissing      ResetReason = "missing"
	ResetReasonCurrentV1    ResetReason = "current-v1"
	ResetReasonUnknown      ResetReason = "unknown"
	ResetReasonIncompatible ResetReason = "incompatible"
)

type formatMarker struct {
	Kind    string `json:"kind"`
	Version uint32 `json:"version"`
}

var resetCounts = struct {
	sync.Mutex
	byReason map[ResetReason]uint64
}{
	byReason: make(map[ResetReason]uint64),
}

func runtimeStorageFormatVersion(conf *Config) uint32 {
	version := conf.GetStorageFormatVersion()
	if version == 0 {
		return currentStorageFormatVersion
	}
	return version
}

func runtimeDriverMode(conf *Config) string {
	mode := conf.GetDriverMode()
	if mode == "" {
		return driverModeAuto
	}
	return mode
}

func runtimeResetPolicy(conf *Config) string {
	policy := conf.GetResetPolicy()
	if policy == "" {
		return resetPolicyAutomatic
	}
	return policy
}

func openRuntimeRoot(
	ctx context.Context,
	le *logrus.Entry,
	opfsRoot js.Value,
	conf *Config,
) (js.Value, error) {
	_ = ctx

	rootPath := conf.GetRootPath()
	pathParts, _ := unixfs.SplitPath(rootPath)
	if len(pathParts) == 0 {
		return js.Undefined(), errors.New("root_path must name an OPFS directory")
	}

	version := runtimeStorageFormatVersion(conf)
	volDir, err := opfs.GetDirectoryPath(opfsRoot, pathParts, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return resetRuntimeRoot(le, opfsRoot, rootPath, ResetReasonMissing, 0, nil, version)
		}
		return js.Undefined(), errors.Wrap(err, "open volume directory")
	}

	marker, err := readFormatMarker(volDir)
	if err == nil {
		if marker.Kind == formatMarkerKind && marker.Version == version {
			return volDir, nil
		}
		names, listErr := opfs.ListDirectory(volDir)
		if listErr != nil {
			return js.Undefined(), errors.Wrap(listErr, "list incompatible volume directory")
		}
		return resetRuntimeRoot(le, opfsRoot, rootPath, ResetReasonIncompatible, marker.Version, names, version)
	}
	if !opfs.IsNotFound(err) {
		names, listErr := opfs.ListDirectory(volDir)
		if listErr != nil {
			return js.Undefined(), errors.Wrap(listErr, "list unreadable volume directory")
		}
		return resetRuntimeRoot(le, opfsRoot, rootPath, ResetReasonIncompatible, 0, names, version)
	}

	names, err := opfs.ListDirectory(volDir)
	if err != nil {
		return js.Undefined(), errors.Wrap(err, "list unmarked volume directory")
	}
	reason := ResetReasonUnknown
	if len(names) == 0 {
		reason = ResetReasonMissing
	} else if hasCurrentV1Layout(names) {
		reason = ResetReasonCurrentV1
	}
	return resetRuntimeRoot(le, opfsRoot, rootPath, reason, 0, names, version)
}

func readFormatMarker(volDir js.Value) (*formatMarker, error) {
	data, err := opfs.ReadFile(volDir, formatMarkerName)
	if err != nil {
		return nil, err
	}
	var marker formatMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, errors.Wrap(err, "decode format marker")
	}
	return &marker, nil
}

func writeFormatMarker(volDir js.Value, version uint32) error {
	data, err := json.Marshal(formatMarker{
		Kind:    formatMarkerKind,
		Version: version,
	})
	if err != nil {
		return err
	}
	return opfs.WriteFile(volDir, formatMarkerName, append(data, '\n'))
}

func resetRuntimeRoot(
	le *logrus.Entry,
	opfsRoot js.Value,
	rootPath string,
	reason ResetReason,
	previousVersion uint32,
	previousEntries []string,
	version uint32,
) (js.Value, error) {
	if err := deleteRuntimeRoot(opfsRoot, rootPath); err != nil {
		return js.Undefined(), errors.Wrap(err, "reset volume directory")
	}
	pathParts, _ := unixfs.SplitPath(rootPath)
	volDir, err := opfs.GetDirectoryPath(opfsRoot, pathParts, true)
	if err != nil {
		return js.Undefined(), errors.Wrap(err, "initialize volume directory")
	}
	if err := writeFormatMarker(volDir, version); err != nil {
		return js.Undefined(), errors.Wrap(err, "write format marker")
	}

	recordRuntimeReset(reason)
	logRuntimeReset(le, rootPath, reason, previousVersion, previousEntries, version)
	return volDir, nil
}

func deleteRuntimeRoot(opfsRoot js.Value, rootPath string) error {
	pathParts, _ := unixfs.SplitPath(rootPath)
	if len(pathParts) == 0 {
		return errors.New("root_path must name an OPFS directory")
	}
	parent := opfsRoot
	for _, p := range pathParts[:len(pathParts)-1] {
		next, err := opfs.GetDirectory(parent, p, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				return nil
			}
			return err
		}
		parent = next
	}
	err := opfs.DeleteEntry(parent, pathParts[len(pathParts)-1], true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	return nil
}

func hasCurrentV1Layout(names []string) bool {
	return slices.Contains(names, "blocks") &&
		slices.Contains(names, "meta") &&
		slices.Contains(names, "gc")
}

func recordRuntimeReset(reason ResetReason) {
	resetCounts.Lock()
	defer resetCounts.Unlock()
	resetCounts.byReason[reason]++
}

func logRuntimeReset(
	le *logrus.Entry,
	rootPath string,
	reason ResetReason,
	previousVersion uint32,
	previousEntries []string,
	version uint32,
) {
	if le == nil {
		return
	}
	fields := logrus.Fields{
		"root_path":      rootPath,
		"reason":         string(reason),
		"format_version": version,
	}
	if previousVersion != 0 {
		fields["previous_format_version"] = previousVersion
	}
	if len(previousEntries) != 0 {
		fields["previous_entries"] = previousEntries
	}
	entry := le.WithFields(fields)
	if reason == ResetReasonMissing {
		entry.Info("initialized opfs volume v2 root")
		return
	}
	entry.Warn("reset opfs volume root for v2 format")
}
