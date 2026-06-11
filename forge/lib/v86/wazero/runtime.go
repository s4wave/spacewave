package v86_wazero

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	defaultInitialMemoryPages = 256
	sharedModuleName          = "v86.shared"
	goenvModuleName           = "v86.goenv"
	envModuleName             = "env"
)

// HostRuntimeOptions configures the Go v86 env runtime.
type HostRuntimeOptions struct {
	InitialMemoryPages uint32
}

// HostRuntime is an instantiated v86 wasm module and its Go-owned host surface.
type HostRuntime struct {
	Runtime wazero.Runtime
	Module  api.Module
	Report  *ImportReport

	started time.Time
	random  atomic.Uint32
	Logs    []string
}

func InstantiateHostRuntime(ctx context.Context, wasmPath string, opts HostRuntimeOptions) (*HostRuntime, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, errors.Wrap(err, "read v86 wasm")
	}
	r := wazero.NewRuntime(ctx)
	closeOnError := true
	defer func() {
		if closeOnError {
			r.Close(ctx)
		}
	}()

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "compile v86 wasm with wazero")
	}
	defer compiled.Close(ctx)

	report, err := importReport(compiled, wasmBytes)
	if err != nil {
		return nil, err
	}
	externs, err := parseWasmExternImports(wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse v86 wasm imports")
	}
	host := &HostRuntime{
		Runtime: r,
		Report:  report,
		started: time.Now(),
	}

	shared, err := buildSharedModule(externs, opts)
	if err != nil {
		return nil, err
	}
	if _, err := r.InstantiateWithConfig(ctx, shared, wazero.NewModuleConfig().WithName(sharedModuleName)); err != nil {
		return nil, errors.Wrap(err, "instantiate shared v86 memory/table module")
	}
	if err := host.instantiateGoEnv(ctx, compiled); err != nil {
		return nil, err
	}
	env, err := buildEnvModule(compiled, externs)
	if err != nil {
		return nil, err
	}
	if _, err := r.InstantiateWithConfig(ctx, env, wazero.NewModuleConfig().WithName(envModuleName)); err != nil {
		return nil, errors.Wrap(err, "instantiate v86 env re-export module")
	}
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("v86"))
	if err != nil {
		return nil, errors.Wrap(err, "instantiate v86 wasm module")
	}
	host.Module = mod
	closeOnError = false
	return host, nil
}

func (h *HostRuntime) Close(ctx context.Context) error {
	if h == nil || h.Runtime == nil {
		return nil
	}
	return h.Runtime.Close(ctx)
}

func (h *HostRuntime) instantiateGoEnv(ctx context.Context, compiled wazero.CompiledModule) error {
	builder := h.Runtime.NewHostModuleBuilder(goenvModuleName)
	for _, fn := range compiled.ImportedFunctions() {
		moduleName, name, _ := fn.Import()
		if moduleName != envModuleName {
			continue
		}
		builder.NewFunctionBuilder().
			WithGoModuleFunction(h.callback(name, fn.ResultTypes()), fn.ParamTypes(), fn.ResultTypes()).
			Export(name)
	}
	if _, err := builder.Instantiate(ctx); err != nil {
		return errors.Wrap(err, "instantiate v86 Go env callbacks")
	}
	return nil
}

func (h *HostRuntime) callback(name string, results []api.ValueType) api.GoModuleFunction {
	return api.GoModuleFunc(func(_ context.Context, mod api.Module, stack []uint64) {
		switch name {
		case "microtick":
			stack[0] = api.EncodeF64(float64(time.Since(h.started).Microseconds()) / 1000)
		case "get_rand_int":
			stack[0] = api.EncodeI32(int32(h.random.Add(0x9e3779b9)))
		case "run_hardware_timers":
			if len(stack) > 1 {
				stack[0] = stack[1]
			} else {
				setZeroResults(stack, results)
			}
		case "log_from_wasm", "console_log_from_wasm":
			h.appendLog(mod, stack)
			setZeroResults(stack, results)
		default:
			setZeroResults(stack, results)
		}
	})
}

func (h *HostRuntime) appendLog(mod api.Module, stack []uint64) {
	if len(stack) < 2 || mod == nil || mod.Memory() == nil {
		return
	}
	offset := api.DecodeU32(stack[0])
	byteCount := api.DecodeU32(stack[1])
	data, ok := mod.Memory().Read(offset, byteCount)
	if !ok {
		return
	}
	h.Logs = append(h.Logs, string(data))
}

func setZeroResults(stack []uint64, results []api.ValueType) {
	for i, typ := range results {
		switch typ {
		case api.ValueTypeF32:
			stack[i] = api.EncodeF32(0)
		case api.ValueTypeF64:
			stack[i] = api.EncodeF64(0)
		default:
			stack[i] = 0
		}
	}
}

func buildSharedModule(imports *wasmExternImports, opts HostRuntimeOptions) ([]byte, error) {
	if len(imports.Memories) > 1 {
		return nil, errors.Errorf("v86 host runtime supports one imported memory, got %d", len(imports.Memories))
	}
	if len(imports.Tables) > 1 {
		return nil, errors.Errorf("v86 host runtime supports one imported table, got %d", len(imports.Tables))
	}
	memoryMin := uint32(0)
	for _, memory := range imports.Memories {
		memoryMin = maxU32(memoryMin, memory.Limits.Min)
	}
	if len(imports.Memories) != 0 {
		memoryMin = maxU32(memoryMin, defaultInitialMemoryPages)
	}
	if opts.InitialMemoryPages != 0 {
		memoryMin = maxU32(memoryMin, opts.InitialMemoryPages)
	}
	tableMin := uint32(0)
	for _, table := range imports.Tables {
		tableMin = maxU32(tableMin, table.Limits.Min)
	}

	var sections [][]byte
	if len(imports.Tables) != 0 {
		tableSection := appendU32(nil, 1)
		tableSection = append(tableSection, 0x70)
		tableSection = appendLimits(tableSection, wasmLimits{Min: tableMin})
		sections = append(sections, wasmSection(4, tableSection))
	}
	if len(imports.Memories) != 0 {
		memorySection := appendU32(nil, 1)
		memorySection = appendLimits(memorySection, wasmLimits{Min: memoryMin})
		sections = append(sections, wasmSection(5, memorySection))
	}
	exportSection := appendU32(nil, uint32(len(imports.Memories)+len(imports.Tables)))
	if len(imports.Memories) != 0 {
		exportSection = appendName(exportSection, imports.Memories[0].Name)
		exportSection = append(exportSection, 0x02)
		exportSection = appendU32(exportSection, 0)
	}
	if len(imports.Tables) != 0 {
		exportSection = appendName(exportSection, imports.Tables[0].Name)
		exportSection = append(exportSection, 0x01)
		exportSection = appendU32(exportSection, 0)
	}
	sections = append(sections, wasmSection(7, exportSection))
	return wasmModule(sections...), nil
}

func buildEnvModule(compiled wazero.CompiledModule, imports *wasmExternImports) ([]byte, error) {
	type typeEntry struct {
		params  []api.ValueType
		results []api.ValueType
	}
	type functionImport struct {
		name      string
		typeIndex uint32
	}
	typeMap := map[string]uint32{}
	var types []typeEntry
	var funcs []functionImport
	for _, fn := range compiled.ImportedFunctions() {
		moduleName, name, _ := fn.Import()
		if moduleName != envModuleName {
			continue
		}
		key := signatureKey(fn.ParamTypes(), fn.ResultTypes())
		typeIndex, ok := typeMap[key]
		if !ok {
			typeIndex = uint32(len(types))
			typeMap[key] = typeIndex
			types = append(types, typeEntry{params: fn.ParamTypes(), results: fn.ResultTypes()})
		}
		funcs = append(funcs, functionImport{name: name, typeIndex: typeIndex})
	}

	typeSection := appendU32(nil, uint32(len(types)))
	for _, typ := range types {
		typeSection = append(typeSection, 0x60)
		typeSection = appendValueTypes(typeSection, typ.params)
		typeSection = appendValueTypes(typeSection, typ.results)
	}

	importCount := uint32(len(funcs) + len(imports.Memories) + len(imports.Tables))
	importSection := appendU32(nil, importCount)
	if len(imports.Memories) != 0 {
		importSection = appendName(importSection, sharedModuleName)
		importSection = appendName(importSection, imports.Memories[0].Name)
		importSection = append(importSection, 0x02)
		importSection = appendLimits(importSection, imports.Memories[0].Limits)
	}
	if len(imports.Tables) != 0 {
		importSection = appendName(importSection, sharedModuleName)
		importSection = appendName(importSection, imports.Tables[0].Name)
		importSection = append(importSection, 0x01, 0x70)
		importSection = appendLimits(importSection, imports.Tables[0].Limits)
	}
	for _, fn := range funcs {
		importSection = appendName(importSection, goenvModuleName)
		importSection = appendName(importSection, fn.name)
		importSection = append(importSection, 0x00)
		importSection = appendU32(importSection, fn.typeIndex)
	}

	exportCount := uint32(len(funcs) + len(imports.Memories) + len(imports.Tables))
	exportSection := appendU32(nil, exportCount)
	if len(imports.Memories) != 0 {
		exportSection = appendName(exportSection, imports.Memories[0].Name)
		exportSection = append(exportSection, 0x02)
		exportSection = appendU32(exportSection, 0)
	}
	if len(imports.Tables) != 0 {
		exportSection = appendName(exportSection, imports.Tables[0].Name)
		exportSection = append(exportSection, 0x01)
		exportSection = appendU32(exportSection, 0)
	}
	for i, fn := range funcs {
		exportSection = appendName(exportSection, fn.name)
		exportSection = append(exportSection, 0x00)
		exportSection = appendU32(exportSection, uint32(i))
	}

	return wasmModule(
		wasmSection(1, typeSection),
		wasmSection(2, importSection),
		wasmSection(7, exportSection),
	), nil
}

func signatureKey(params, results []api.ValueType) string {
	var b strings.Builder
	for _, typ := range params {
		b.WriteByte(byte(typ))
	}
	b.WriteByte(':')
	for _, typ := range results {
		b.WriteByte(byte(typ))
	}
	return b.String()
}

func appendValueTypes(out []byte, types []api.ValueType) []byte {
	out = appendU32(out, uint32(len(types)))
	for _, typ := range types {
		out = append(out, wasmValueType(typ))
	}
	return out
}

func wasmValueType(typ api.ValueType) byte {
	switch typ {
	case api.ValueTypeI32:
		return 0x7f
	case api.ValueTypeI64:
		return 0x7e
	case api.ValueTypeF32:
		return 0x7d
	case api.ValueTypeF64:
		return 0x7c
	default:
		panic("unsupported wasm value type")
	}
}

func wasmModule(sections ...[]byte) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, section := range sections {
		out = append(out, section...)
	}
	return out
}

func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = appendU32(out, uint32(len(payload)))
	out = append(out, payload...)
	return out
}

func appendName(out []byte, name string) []byte {
	out = appendU32(out, uint32(len(name)))
	return append(out, name...)
}

func appendLimits(out []byte, limits wasmLimits) []byte {
	if limits.Max == nil {
		out = appendU32(out, 0)
		return appendU32(out, limits.Min)
	}
	out = appendU32(out, 1)
	out = appendU32(out, limits.Min)
	return appendU32(out, *limits.Max)
}

func appendU32(out []byte, value uint32) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if value == 0 {
			return out
		}
	}
}

func maxU32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
