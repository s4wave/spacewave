package v86_wazero

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestV86WazeroAptWritableRoot(t *testing.T) {
	// Require the enabled apt integration fixture before booting the guest.
	if !runV86AptTest() {
		t.Skip("set RUN_V86_APT_TEST=true to boot the writable v86 root and run apt update")
	}

	// Bound the guest runtime lifetime for the apt check.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Resolve the V86 image and boot assets.
	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}

	// Open a writable RAM root for the guest package lists.
	v86fsServer, releaseRoot, err := OpenV86Root(RootMode{Mode: rootModeRAM}, assets.RootfsTar)
	if err != nil {
		t.Fatalf("open writable v86fs root: %v", err)
	}
	defer releaseRoot()

	// Instantiate the V86 guest with the Go host runtime.
	instance, err := InstantiateHostRuntime(ctx, assets.Wasm, HostRuntimeOptions{})
	if err != nil {
		t.Fatalf("instantiate v86 wasm with wazero host runtime: %v", err)
	}
	defer instance.Close(ctx)

	// Load SeaBIOS for the V86 CPU.
	bios, err := os.ReadFile(assets.SeaBIOS)
	if err != nil {
		t.Fatalf("read SeaBIOS: %v", err)
	}

	// Load the VGA BIOS for the V86 CPU.
	vgaBIOS, err := os.ReadFile(assets.VGABIOS)
	if err != nil {
		t.Fatalf("read VGABIOS: %v", err)
	}

	// Load the Linux kernel for the V86 guest.
	kernel, err := os.ReadFile(assets.Kernel)
	if err != nil {
		t.Fatalf("read kernel: %v", err)
	}

	// Boot the V86 CPU with the writable filesystem root.
	if err := instance.InitCPU(ctx, HostBootOptions{
		BIOS:        bios,
		VGABIOS:     vgaBIOS,
		Kernel:      kernel,
		V86FSServer: v86fsServer,
	}); err != nil {
		t.Fatalf("initialize v86 CPU with writable v86fs: %v", err)
	}
	instance.SetSerialSink(os.Stderr)

	// Wait for the guest root shell before running apt.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer waitCancel()
	if _, err := waitSerial(waitCtx, instance, ":/#"); err != nil {
		t.Fatalf("writable v86fs root shell prompt not reached: %v; serial_tail=%q logs=%q",
			err,
			tailString(string(instance.SerialOutput()), 8192),
			tailStrings(instance.Logs, 8),
		)
	}

	// Bound the guest apt update command lifetime.
	aptCtx, aptCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer aptCancel()

	// Run apt update in the writable guest root.
	serial, err := runShellCommand(aptCtx, instance, "apt update")
	if err != nil {
		t.Fatalf("run apt update: %v; serial=%q", err, serial)
	}

	// Verify apt reports no writable-root filesystem failures.
	for _, marker := range []string{
		"Input/output error",
		"mkstemp",
		"partial is missing",
	} {
		if strings.Contains(serial, marker) {
			t.Fatalf("apt update hit filesystem EIO marker %q; serial=%q", marker, serial)
		}
	}

	// Verify apt reaches the network or package-list stage.
	reachedNetwork := false
	for _, marker := range []string{
		"Temporary failure resolving",
		"Could not resolve",
		"Err:",
		"Hit:",
		"Get:",
		"Reading package lists",
	} {
		if strings.Contains(serial, marker) {
			reachedNetwork = true
			break
		}
	}
	if !reachedNetwork {
		t.Fatalf("apt update did not reach an apt network/list step; serial=%q", serial)
	}

	// Report the guest apt output after the writable-root check.
	t.Logf("apt update reached network/list stage without writable-root EIO markers; serial tail=%q", tailString(serial, 4096))
}

func runV86AptTest() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_APT_TEST")), "true")
}
