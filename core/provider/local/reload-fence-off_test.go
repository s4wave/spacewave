//go:build !js && !goscript

package provider_local

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

// TestWaitSettledWithReloadFenceOff runs TestWaitSettledAfterUnknownCommit
// again in a child process with sobject.ReloadFenceEnv set to "off", which is
// read once at startup. The switch back must keep every settle answer right:
// the old mount keeps writing after an unknown, and WaitSettled still reports
// applied exactly when the World holds the write.
func TestWaitSettledWithReloadFenceOff(t *testing.T) {
	if sobject.ReloadFenceOff {
		t.Skip("the fence is already off in this process")
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWaitSettledAfterUnknownCommit$", "-test.count=1")
	cmd.Env = append(os.Environ(), sobject.ReloadFenceEnv+"=off")
	out, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(out, []byte("PASS")) {
		t.Fatalf("with the reload fence off: %v\n%s", err, out)
	}
}
