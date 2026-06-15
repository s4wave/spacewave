package v86_wazero

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestV86WazeroAptWritableRoot(t *testing.T) {
	if !runV86AptTest() {
		t.Skip("set RUN_V86_APT_TEST=true to boot the writable v86 root and run apt update")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	v86fsServer, releaseRoot, err := OpenV86Root(RootMode{Mode: rootModeRAM}, assets.RootfsTar)
	if err != nil {
		t.Fatalf("open writable v86fs root: %v", err)
	}
	defer releaseRoot()

	instance, err := InstantiateHostRuntime(ctx, assets.Wasm, HostRuntimeOptions{})
	if err != nil {
		t.Fatalf("instantiate v86 wasm with wazero host runtime: %v", err)
	}
	defer instance.Close(ctx)

	bios, err := os.ReadFile(assets.SeaBIOS)
	if err != nil {
		t.Fatalf("read SeaBIOS: %v", err)
	}
	vgaBIOS, err := os.ReadFile(assets.VGABIOS)
	if err != nil {
		t.Fatalf("read VGABIOS: %v", err)
	}
	kernel, err := os.ReadFile(assets.Kernel)
	if err != nil {
		t.Fatalf("read kernel: %v", err)
	}
	if err := instance.InitCPU(ctx, HostBootOptions{
		BIOS:        bios,
		VGABIOS:     vgaBIOS,
		Kernel:      kernel,
		V86FSServer: v86fsServer,
	}); err != nil {
		t.Fatalf("initialize v86 CPU with writable v86fs: %v", err)
	}
	instance.SetSerialSink(os.Stderr)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer waitCancel()
	if _, err := waitSerial(waitCtx, instance, ":/#"); err != nil {
		t.Fatalf("writable v86fs root shell prompt not reached: %v; serial_tail=%q logs=%q",
			err,
			tailString(string(instance.SerialOutput()), 8192),
			tailStrings(instance.Logs, 8),
		)
	}

	aptCtx, aptCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer aptCancel()
	serial, err := runShellCommand(aptCtx, instance, "apt update")
	if err != nil {
		t.Fatalf("run apt update: %v; serial=%q", err, serial)
	}
	for _, marker := range []string{
		"Input/output error",
		"mkstemp",
		"partial is missing",
	} {
		if strings.Contains(serial, marker) {
			t.Fatalf("apt update hit filesystem EIO marker %q; serial=%q", marker, serial)
		}
	}

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

	t.Logf("apt update reached network/list stage without writable-root EIO markers; serial tail=%q", tailString(serial, 4096))
}

func runV86AptTest() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_APT_TEST")), "true")
}
