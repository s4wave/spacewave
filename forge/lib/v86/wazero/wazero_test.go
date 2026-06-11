package v86_wazero

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
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

	rustInit := instance.Module.ExportedFunction("rust_init")
	if rustInit == nil {
		t.Fatal("instantiated v86 wasm does not export rust_init")
	}
	if _, err := rustInit.Call(ctx); err != nil {
		t.Fatalf("call v86 rust_init: %v", err)
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

func runV86WazeroTests() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_WAZERO")), "true") ||
		runV86WazeroBootTests()
}

func runV86WazeroBootTests() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_V86_WAZERO_BOOT")), "true")
}
