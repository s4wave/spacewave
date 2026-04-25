//go:build !js

package spacewave_launcher_controller

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const rawUpdateRelayTargetEnv = "SPACEWAVE_RAW_UPDATE_RELAY_TARGET"

const rawUpdateRelayCleanupEnv = "SPACEWAVE_RAW_UPDATE_RELAY_CLEANUP"

const rawUpdateRelayParentEnv = "SPACEWAVE_RAW_UPDATE_RELAY_PARENT_PID"

func init() {
	runRawUpdateRelayFromEnv()
}

func runRawUpdateRelayFromEnv() {
	if cleanupPath := os.Getenv(rawUpdateRelayCleanupEnv); cleanupPath != "" {
		_ = os.Remove(cleanupPath)
		_ = os.Unsetenv(rawUpdateRelayCleanupEnv)
	}

	targetPath := os.Getenv(rawUpdateRelayTargetEnv)
	if targetPath == "" {
		return
	}
	if err := runRawUpdateRelay(targetPath); err != nil {
		_, _ = os.Stderr.WriteString("spacewave raw update relay: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(0)
}

// applyRawBinaryUpdate starts the staged entrypoint as a tmp relay.
func applyRawBinaryUpdate(execPath, stagedPath string) error {
	tmpPath, err := stageRawUpdateRelay(execPath, stagedPath)
	if err != nil {
		return err
	}
	return startRawUpdateRelay(tmpPath, execPath)
}

func stageRawUpdateRelay(execPath, stagedPath string) (string, error) {
	tmpPath := execPath + ".tmp"
	if err := copyFileMode(stagedPath, tmpPath, 0o755); err != nil {
		return "", errors.Wrap(err, "stage raw update relay")
	}
	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return "", errors.Wrap(err, "remove staged raw update")
	}
	return tmpPath, nil
}

func runRawUpdateRelay(targetPath string) error {
	if err := waitRawUpdateRelayParent(); err != nil {
		return err
	}
	selfPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "resolve relay executable")
	}
	if err := copyFileMode(selfPath, targetPath, 0o755); err != nil {
		return errors.Wrap(err, "copy relay to target")
	}
	return startRawUpdateTarget(targetPath, selfPath)
}

func copyFileMode(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return errors.Wrap(err, "open source")
	}
	defer src.Close()

	tmpPath := dstPath + ".copying"
	defer os.Remove(tmpPath)

	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return errors.Wrap(err, "create destination")
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return errors.Wrap(err, "copy file")
	}
	if err := dst.Close(); err != nil {
		return errors.Wrap(err, "close destination")
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return errors.Wrap(err, "chmod destination")
	}
	return replaceFile(tmpPath, dstPath)
}

func rawUpdateArgs(exePath string) []string {
	if len(os.Args) == 0 {
		return []string{exePath}
	}
	args := append([]string{exePath}, os.Args[1:]...)
	return args
}

func rawUpdateEnv(set map[string]string, remove ...string) []string {
	out := make([]string, 0, len(os.Environ())+len(set))
	for _, kv := range os.Environ() {
		if rawUpdateEnvMatches(kv, set, remove) {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range set {
		out = append(out, k+"="+v)
	}
	return out
}

func rawUpdateEnvMatches(kv string, set map[string]string, remove []string) bool {
	for k := range set {
		if strings.HasPrefix(kv, k+"=") {
			return true
		}
	}
	for _, k := range remove {
		if strings.HasPrefix(kv, k+"=") {
			return true
		}
	}
	return false
}

func rawUpdateRelayEnv(targetPath string) []string {
	return rawUpdateEnv(
		map[string]string{
			rawUpdateRelayTargetEnv: targetPath,
			rawUpdateRelayParentEnv: strconv.Itoa(os.Getpid()),
		},
		rawUpdateRelayCleanupEnv,
	)
}

func rawUpdateTargetEnv(cleanupPath string) []string {
	return rawUpdateEnv(
		map[string]string{
			rawUpdateRelayCleanupEnv: cleanupPath,
		},
		rawUpdateRelayTargetEnv,
		rawUpdateRelayParentEnv,
	)
}
