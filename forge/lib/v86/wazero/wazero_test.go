package v86_wazero

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

func TestV86WazeroCompileFromRealImage(t *testing.T) {
	if !runV86WazeroTests() {
		t.Skip("set RUN_V86_WAZERO=true to hydrate the real V86Image and compile v86 wasm with wazero")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	report, err := CompileImportReport(ctx, assets.Wasm)
	if err != nil {
		t.Fatalf("compile v86 wasm: %v", err)
	}
	if len(report.Exports) == 0 {
		t.Fatal("compiled v86 wasm exposes no functions")
	}
	t.Logf("v86 image %s assets at %s", assets.ImageKey, assets.Dir)
	t.Logf(
		"v86 wasm imports: functions=%d memories=%d tables=%d exports=%d",
		len(report.Functions),
		len(report.Memories),
		len(report.Tables),
		len(report.Exports),
	)
	for _, imp := range report.Memories {
		t.Logf("v86 wasm imported memory: %s", imp)
	}
	for _, imp := range report.Tables {
		t.Logf("v86 wasm imported table: %s", imp)
	}
	for i, imp := range report.Functions {
		if i >= 20 {
			t.Logf("v86 wasm imports truncated after 20 of %d functions", len(report.Functions))
			break
		}
		t.Logf("v86 wasm import[%d]: %s", i, imp)
	}
}

func TestV86WazeroInstantiateHostRuntime(t *testing.T) {
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to instantiate v86 wasm with the Go wazero host runtime")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	instance, err := InstantiateHostRuntime(ctx, assets.Wasm, HostRuntimeOptions{})
	if err != nil {
		t.Fatalf("instantiate v86 wasm with wazero host runtime: %v", err)
	}
	defer instance.Close(ctx)

	if err := instance.RustInit(ctx); err != nil {
		t.Fatalf("initialize v86 wasm: %v", err)
	}
	t.Logf(
		"instantiated v86 wasm image %s with wazero host runtime; functions=%d memories=%d tables=%d exports=%d",
		assets.ImageKey,
		len(instance.Report.Functions),
		len(instance.Report.Memories),
		len(instance.Report.Tables),
		len(instance.Report.Exports),
	)
}

func TestV86WazeroCPUSetupAndMainLoop(t *testing.T) {
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to initialize v86 CPU memory and run the Go host loop")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
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
		BIOS:    bios,
		VGABIOS: vgaBIOS,
		Kernel:  kernel,
		Cmdline: "console=ttyS0",
	}); err != nil {
		t.Fatalf("initialize v86 CPU: %v", err)
	}
	const ticks = 100000
	var delay float64
	for range ticks {
		delay, err = instance.MainLoop(ctx)
		if err != nil {
			t.Fatalf("run v86 main loop: %v", err)
		}
	}
	serial := instance.SerialOutput()
	if len(serial) == 0 {
		t.Fatalf(
			"expected v86 serial output after bounded boot loop; delay=%f halt_events=%d exceptions=%d last_exception=%d ip=%#x cs=%#x io reads=%s writes=%s last_reads=%s last_writes=%s logs=%q",
			delay,
			instance.HaltEvents(),
			instance.ExceptionCount(),
			instance.LastException(),
			readUint32Le(t, instance, 556),
			readUint16Le(t, instance, 668+2*1),
			topPorts(instance.ioReads),
			topPorts(instance.ioWrites),
			portValues(instance.ioLastReads, 0x61, 0x64, 0x60, 0x42, 0x43),
			portValues(instance.ioLastWrites, 0x61, 0x64, 0x60, 0x42, 0x43),
			tailStrings(instance.Logs, 5),
		)
	}
	t.Logf("ran %d v86 main loop ticks; requested delay=%f halt_events=%d exceptions=%d serial=%q", ticks, delay, instance.HaltEvents(), instance.ExceptionCount(), string(serial))
}

func TestV86WazeroV86FSDeviceProbe(t *testing.T) {
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to boot Linux with the Go v86fs host device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	v86fsServer, releaseRoot, err := OpenV86Root(RootMode{Mode: rootModeReadonly}, assets.RootfsTar)
	if err != nil {
		t.Fatalf("open v86fs root: %v", err)
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
		t.Fatalf("initialize v86 CPU with v86fs: %v", err)
	}

	const ticks = 50000
	for range ticks {
		if _, err := instance.MainLoop(ctx); err != nil {
			t.Fatalf("run v86 main loop: %v", err)
		}
		serial := string(instance.SerialOutput())
		if strings.Contains(serial, "v86fs: probed, 3 virtqueues ready") {
			t.Logf("v86fs device probe reached proof marker after serial=%q", serial)
			return
		}
	}
	t.Fatalf("v86fs device probe did not reach proof marker after %d ticks; serial=%q io reads=%s writes=%s last_reads=%s last_writes=%s logs=%q",
		ticks,
		string(instance.SerialOutput()),
		topPorts(instance.ioReads),
		topPorts(instance.ioWrites),
		portValues(instance.ioLastReads, virtioV86FSCommonPort, virtioV86FSNotifyPort, virtioV86FSISRPort, pciConfigData),
		portValues(instance.ioLastWrites, virtioV86FSCommonPort, virtioV86FSNotifyPort, virtioV86FSISRPort, pciConfigData),
		tailStrings(instance.Logs, 5),
	)
}

func TestV86WazeroV86FSRootShell(t *testing.T) {
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to boot Linux with the Go v86fs root device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	v86fsServer, releaseRoot, err := OpenV86Root(RootMode{Mode: rootModeReadonly}, assets.RootfsTar)
	if err != nil {
		t.Fatalf("open v86fs root: %v", err)
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
		t.Fatalf("initialize v86 CPU with v86fs: %v", err)
	}
	instance.SetSerialSink(os.Stderr)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer waitCancel()
	if _, err := waitSerial(waitCtx, instance, ":/#"); err != nil {
		stats := instance.v86fs.stats()
		t.Fatalf("v86fs root shell prompt not reached: %v; device{driver_ok=%t last_status=%#x irq_line=%d kicks=%v requests=%d replies=%d notifications=%d trace=%v} serial_tail=%q top_writes=%s logs=%q",
			err,
			stats.driverOK,
			stats.lastStatus,
			stats.irqLine,
			stats.kicks,
			stats.requests,
			stats.replies,
			stats.notifications,
			stats.trace,
			tailString(string(instance.SerialOutput()), 8192),
			topPorts(instance.ioWrites),
			tailStrings(instance.Logs, 8),
		)
	}
	serial, err := runShellCommand(ctx, instance, "echo wazero-v86fs")
	if err != nil {
		t.Fatalf("run echo command: %v; serial=%q", err, serial)
	}
	if !strings.Contains(serial, "wazero-v86fs") {
		t.Fatalf("echo command output missing from serial=%q", serial)
	}
	serial, err = runShellCommand(ctx, instance, "echo $?")
	if err != nil {
		t.Fatalf("run exit status command: %v; serial=%q", err, serial)
	}
	if !strings.Contains(serial, "\n0\r\n") && !strings.Contains(serial, "\r\n0\r\n") && !strings.Contains(serial, "\r0\r\n") {
		t.Fatalf("exit status command did not print 0; serial=%q", serial)
	}
	t.Logf("v86fs root shell proof reached prompt and command exit status; serial tail=%q", tailString(string(instance.SerialOutput()), 4096))
}

func TestV86WazeroHost9PRootShell(t *testing.T) {
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to boot Linux with the Go host9p root device")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}
	if !filesExist(assets.RootfsJSON) {
		t.Fatalf("host9p root proof requires fs.json at %s", assets.RootfsJSON)
	}
	if info, err := os.Stat(assets.RootfsFlatDir); err != nil || !info.IsDir() {
		t.Fatalf("host9p root proof requires flat dir at %s", assets.RootfsFlatDir)
	}
	host9p, err := OpenHost9PFS(filepath.Dir(assets.RootfsJSON))
	if err != nil {
		t.Fatalf("open host9p rootfs: %v", err)
	}

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
		BIOS:     bios,
		VGABIOS:  vgaBIOS,
		Kernel:   kernel,
		Host9PFS: host9p,
		Cmdline:  DefaultHost9PRootCmdline,
	}); err != nil {
		t.Fatalf("initialize v86 CPU with host9p: %v", err)
	}
	instance.SetSerialSink(os.Stderr)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer waitCancel()
	if _, err := waitSerial(waitCtx, instance, ":/#"); err != nil {
		reqCount, lastType, notifies, availIdx, availLastIdx, configured := host9p.stats()
		t.Fatalf("host9p root shell prompt not reached: %v; host9p_requests=%d last_9p_type=%d notifies=%d queue_configured=%t avail_idx=%d avail_last_idx=%d serial_tail=%q io_counts=%s last_reads=%s last_writes=%s logs=%q",
			err,
			reqCount,
			lastType,
			notifies,
			configured,
			availIdx,
			availLastIdx,
			tailString(string(instance.SerialOutput()), 8192),
			portCounts(instance, virtioHost9PCommonPort, virtioHost9PNotifyPort, virtioHost9PISRPort, virtioHost9PConfigPort, 0xc100, 0xc104, 0xc108, 0xc10c, 0xc114, 0xc116, 0xc118, 0xc11c, 0xc120, 0xc128, 0xc130, 0xc140, 0xc150, 0xc000, pciConfigData),
			portValues(instance.ioLastReads, virtioHost9PCommonPort, virtioHost9PNotifyPort, virtioHost9PISRPort, virtioHost9PConfigPort, 0xc100, 0xc104, 0xc108, 0xc10c, 0xc114, 0xc116, 0xc118, 0xc11c, 0xc120, 0xc128, 0xc130, 0xc140, 0xc150, 0xc000, pciConfigData),
			portValues(instance.ioLastWrites, virtioHost9PCommonPort, virtioHost9PNotifyPort, virtioHost9PISRPort, virtioHost9PConfigPort, 0xc100, 0xc104, 0xc108, 0xc10c, 0xc114, 0xc116, 0xc118, 0xc11c, 0xc120, 0xc128, 0xc130, 0xc140, 0xc150, 0xc000, pciConfigData),
			tailStrings(instance.Logs, 5),
		)
	}
	serial, err := runShellCommand(ctx, instance, "echo wazero-host9p")
	if err != nil {
		t.Fatalf("run echo command: %v; serial=%q", err, serial)
	}
	if !strings.Contains(serial, "wazero-host9p") {
		t.Fatalf("echo command output missing from serial=%q", serial)
	}
	serial, err = runShellCommand(ctx, instance, "echo $?")
	if err != nil {
		t.Fatalf("run exit status command: %v; serial=%q", err, serial)
	}
	if !strings.Contains(serial, "\n0\r\n") && !strings.Contains(serial, "\r\n0\r\n") && !strings.Contains(serial, "\r0\r\n") {
		t.Fatalf("exit status command did not print 0; serial=%q", serial)
	}
	t.Logf("host9p root shell proof reached prompt and command exit status; serial tail=%q", tailString(string(instance.SerialOutput()), 4096))
}

func TestLinuxBootROMChecksum(t *testing.T) {
	rom := makeLinuxBootROM(0x8000, 0xe000)
	var checksum byte
	for _, b := range rom {
		checksum += b
	}
	if checksum != 0 {
		t.Fatalf("linux boot option ROM checksum = %#x, want 0", checksum)
	}
}

func TestHostBootOptionsKernelCmdlineDefault(t *testing.T) {
	if got := (HostBootOptions{}).kernelCmdline(); got != DefaultHost9PRootCmdline {
		t.Fatalf("empty boot options cmdline = %q, want %q", got, DefaultHost9PRootCmdline)
	}
	if got := (HostBootOptions{V86FSServer: unixfs_v86fs.NewServer(nil)}).kernelCmdline(); got != DefaultV86FSRootCmdline {
		t.Fatalf("v86fs boot options cmdline = %q, want %q", got, DefaultV86FSRootCmdline)
	}
	if got := (HostBootOptions{
		Cmdline:     "console=ttyS0",
		V86FSServer: unixfs_v86fs.NewServer(nil),
	}).kernelCmdline(); got != "console=ttyS0" {
		t.Fatalf("explicit cmdline = %q, want caller override", got)
	}
}

func TestUARTSerialInputQueue(t *testing.T) {
	ctx := context.Background()
	host := &HostRuntime{
		ioPorts:      newIOPorts(),
		ioReads:      make(map[uint16]uint64),
		ioWrites:     make(map[uint16]uint64),
		ioLastReads:  make(map[uint16]uint32),
		ioLastWrites: make(map[uint16]uint32),
	}
	host.registerUART(0x3f8)

	if err := host.WriteSerialInput(ctx, []byte("hi")); err != nil {
		t.Fatalf("write serial input: %v", err)
	}
	if got := host.readIO(ctx, 0x3fd, 8); got&uartLsrDataReady == 0 {
		t.Fatalf("COM1 line status %#x missing data-ready bit", got)
	}
	if got := host.readIO(ctx, 0x3f8, 8); got != 'h' {
		t.Fatalf("first COM1 byte = %#x, want 'h'", got)
	}
	if got := host.readIO(ctx, 0x3fd, 8); got&uartLsrDataReady == 0 {
		t.Fatalf("COM1 line status %#x dropped data-ready before queue drained", got)
	}
	if got := host.readIO(ctx, 0x3f8, 8); got != 'i' {
		t.Fatalf("second COM1 byte = %#x, want 'i'", got)
	}
	if got := host.readIO(ctx, 0x3fd, 8); got&uartLsrDataReady != 0 {
		t.Fatalf("COM1 line status %#x kept data-ready after queue drained", got)
	}
}

func readUint32Le(t *testing.T, h *HostRuntime, offset uint32) uint32 {
	t.Helper()
	value, ok := h.Module.Memory().ReadUint32Le(offset)
	if !ok {
		t.Fatalf("read wasm uint32 at %#x", offset)
	}
	return value
}

func readUint16Le(t *testing.T, h *HostRuntime, offset uint32) uint16 {
	t.Helper()
	value, ok := h.Module.Memory().ReadUint16Le(offset)
	if !ok {
		t.Fatalf("read wasm uint16 at %#x", offset)
	}
	return value
}

func tailStrings(values []string, count int) []string {
	if len(values) <= count {
		return values
	}
	return values[len(values)-count:]
}

func topPorts(counts map[uint16]uint64) string {
	type entry struct {
		port  uint16
		count uint64
	}
	var entries []entry
	for port, count := range counts {
		entries = append(entries, entry{port: port, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].port < entries[j].port
		}
		return entries[i].count > entries[j].count
	})
	if len(entries) > 12 {
		entries = entries[:12]
	}
	var parts []string
	for _, entry := range entries {
		parts = append(parts, fmt.Sprintf("%#x:%d", entry.port, entry.count))
	}
	return strings.Join(parts, ",")
}

func portValues(values map[uint16]uint32, ports ...uint16) string {
	var parts []string
	for _, port := range ports {
		parts = append(parts, fmt.Sprintf("%#x:%#x", port, values[port]))
	}
	return strings.Join(parts, ",")
}

func portCounts(h *HostRuntime, ports ...uint16) string {
	var parts []string
	for _, port := range ports {
		parts = append(parts, fmt.Sprintf("%#x:r%d/w%d", port, h.ioReads[port], h.ioWrites[port]))
	}
	return strings.Join(parts, ",")
}

func waitSerial(ctx context.Context, h *HostRuntime, marker string) (string, error) {
	return waitSerialFrom(ctx, h, marker, 0)
}

func runShellCommand(ctx context.Context, h *HostRuntime, command string) (string, error) {
	before := len(h.SerialOutput())
	if err := h.WriteSerialInput(ctx, []byte(command+"\n")); err != nil {
		return string(h.SerialOutput()), err
	}
	serial, err := waitSerialFrom(ctx, h, ":/#", before)
	if before < len(serial) {
		return serial[before:], err
	}
	return serial, err
}

func waitSerialFrom(ctx context.Context, h *HostRuntime, marker string, start int) (string, error) {
	type result struct {
		serial string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		for {
			if _, err := h.MainLoop(ctx); err != nil {
				done <- result{serial: string(h.SerialOutput()), err: err}
				return
			}
			serial := string(h.SerialOutput())
			if start < len(serial) && strings.Contains(serial[start:], marker) {
				done <- result{serial: serial}
				return
			}
			if err := ctx.Err(); err != nil {
				done <- result{serial: serial, err: err}
				return
			}
		}
	}()
	select {
	case res := <-done:
		return res.serial, res.err
	case <-ctx.Done():
		return string(h.SerialOutput()), ctx.Err()
	}
}

func tailString(value string, count int) string {
	if len(value) <= count {
		return value
	}
	return value[len(value)-count:]
}

func runV86WazeroTests() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_WAZERO")), "true") ||
		runV86WazeroBootTests()
}

func runV86WazeroBootTests() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_WAZERO_BOOT")), "true")
}
