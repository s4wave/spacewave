//go:build !js

package plugin_host_process

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// startupFailure preserves a native plugin's registration protocol failure after
// process exit. Older plugins report this failure on stderr without a typed RPC.
func startupFailure(pluginID, manifestRoot, executable string, lines []string, exitErr error) error {
	// Only classify a protobuf failure from the initial registration operation.
	for _, line := range lines {
		if !strings.Contains(line, "complete initial capability registration:") || !strings.Contains(line, "proto:") {
			continue
		}

		// Compute identities only on this terminal path; normal startup does no extra I/O.
		hostPath, err := os.Executable()
		hostBuild := "unavailable"
		if err == nil {
			hostBuild = executableBuild(hostPath)
		}
		return plugin_host.NewStartupProtocolError(hostBuild,
			pluginID+" manifest="+manifestRoot+" "+executableBuild(executable), errors.New(line))
	}
	return exitErr
}

// executableBuild identifies exact native bytes and their Go toolchain when available.
func executableBuild(path string) string {
	// A digest identifies the build even when the executable omits Go build metadata.
	f, err := os.Open(path)
	if err != nil {
		return path + " (identity unavailable: " + err.Error() + ")"
	}
	defer f.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return path + " (identity unavailable: " + err.Error() + ")"
	}
	identity := "sha256:" + hex.EncodeToString(digest.Sum(nil))

	// Report the compiler alongside the content identity for native protocol diagnosis.
	if info, err := buildinfo.ReadFile(path); err == nil {
		identity += " " + info.GoVersion
	}
	return identity
}
