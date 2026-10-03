package v86_wazero

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestV86WazeroAptOverNet boots the guest with --net networking, brings eth0 up
// with the static lease the usermode stack serves, and asserts apt update fetches
// real index data over the network (DNS resolves via the stack proxy, TCP
// downloads complete) and exits 0 with no writable-root EIO and no fchmod EPERM.
// The guest image has no DHCP client, so the interface is configured
// statically to the stack's gateway/lease (10.0.2.15/24 via 10.0.2.2) and
// resolv.conf is pointed at the stack's DNS proxy (the gateway), which resolves
// via the host resolver.
//
// apt exit 0 is required. The guest v86fs kernel driver now owns new inodes by
// the creating task (inode_init_owner on create/mkdir/symlink), so apt's _apt
// download sandbox can fchmod its own cache temp files instead of getting EPERM
// from setattr_prepare. A recurrence of "fchmod (1: Operation not permitted)" is
// a driver regression and fails the test.
func TestV86WazeroAptOverNet(t *testing.T) {
	// Require the enabled apt integration fixture before booting the guest.
	if !runV86AptTest() {
		t.Skip("set RUN_V86_APT_TEST=true to boot the networked v86 guest and run apt update")
	}

	// Bound the guest runtime lifetime for the apt check.
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
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
		t.Fatalf("instantiate v86 runtime: %v", err)
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
		BIOS:    bios,
		VGABIOS: vgaBIOS,
		// apt's package-list parser needs more than the default 64 MiB guest
		// envelope after kernel reservations; keep this scoped to the apt proof.
		MemorySize:  128 * 1024 * 1024,
		Kernel:      kernel,
		V86FSServer: v86fsServer,
		Networking:  &NetworkConfig{},
	}); err != nil {
		t.Fatalf("init CPU with networking: %v", err)
	}
	instance.SetSerialSink(os.Stderr)

	// Wait for the guest root shell before running apt.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer waitCancel()
	if _, err := waitSerial(waitCtx, instance, ":/#"); err != nil {
		t.Fatalf("shell prompt not reached: %v", err)
	}

	// Configure the guest interface, route, and DNS proxy for apt.
	for _, cmd := range []string{
		"ip addr show dev eth0 | grep -q '10.0.2.15/24' || ip addr add 10.0.2.15/24 dev eth0",
		"ip link set eth0 up",
		"ip route replace default via 10.0.2.2",
		// Point the guest at the stack's DNS proxy (the gateway), which resolves
		// via the host resolver. Older images shipped public DNS servers here, so
		// keep the setup idempotent across rootfs revisions.
		"printf 'nameserver 10.0.2.2\\n' > /etc/resolv.conf",
	} {
		// Run the guest network setup command within its deadline.
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		serial, err := runShellCommand(cctx, instance, cmd)
		ccancel()
		if err != nil {
			t.Fatalf("guest net setup %q failed: %v; serial=%q", cmd, err, serial)
		}
	}

	// Bound the guest apt update command lifetime.
	aptCtx, aptCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer aptCancel()

	// The verifier needs the signed package index fetched and stored over the
	// usermode network; translation indexes only add time and memory pressure.
	serial, err := runShellCommand(aptCtx, instance, "TERM=dumb apt-get -o Acquire::Languages=none -o Dpkg::Progress-Fancy=0 -o APT::Color=0 update; echo APT_RC=$?")
	if err != nil {
		t.Fatalf("run apt update: %v; serial=%q", err, serial)
	}

	// Verify apt reports no guest filesystem errors.
	for _, marker := range []string{"Input/output error", "mkstemp", "partial is missing"} {
		if strings.Contains(serial, marker) {
			t.Fatalf("apt update hit filesystem EIO marker %q; serial=%q", marker, serial)
		}
	}

	// Verify apt reports no DNS or connection failures.
	for _, marker := range []string{"Temporary failure resolving", "Could not resolve", "Cannot initiate the connection"} {
		if strings.Contains(serial, marker) {
			t.Fatalf("apt update hit network failure marker %q; serial=%q", marker, serial)
		}
	}

	// Real fetch proof: apt pulled index data over the usermode stack (DNS +
	// TCP) and finished the download phase.
	if !strings.Contains(serial, "Get:") {
		t.Fatalf("apt update did not download over the network (no Get:); serial=%q", serial)
	}
	if !strings.Contains(serial, "Fetched") {
		t.Fatalf("apt update did not complete a fetch (no Fetched summary); serial=%q", serial)
	}

	// The guest v86fs driver now owns new inodes by the creating task, so apt's
	// _apt sandbox must not get EPERM fchmodding its own cache temp files. A
	// recurrence is a driver regression, checked before the exit code so the
	// failure names the root cause.
	if strings.Contains(serial, "fchmod") && strings.Contains(serial, "Operation not permitted") {
		t.Fatalf("apt update hit the v86fs fchmod EPERM gap that inode_init_owner should fix; serial=%q", serial)
	}

	// Verify apt exits successfully after fetching the package indexes.
	if !strings.Contains(serial, "APT_RC=0") {
		t.Fatalf("apt update did not exit 0; serial=%q", serial)
	}

	// Report the networked guest apt output.
	t.Logf("apt update fetched over the usermode network and exited 0; serial tail=%q", tailString(serial, 4096))
}
