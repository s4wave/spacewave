package v86_wazero

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/forge/lib/v86/wazero/usernet"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	defaultInitialMemoryPages = 256
	mmapBlockBits             = 17
	sharedModuleName          = "v86.shared"
	goenvModuleName           = "v86.goenv"
	envModuleName             = "env"
)

// HostRuntime is an instantiated v86 wasm module and its Go-owned host surface.
type HostRuntime struct {
	// machineBus is the bus the devices share. It holds the v86 Module.
	*machineBus

	// Runtime owns the wazero modules instantiated for the guest.
	Runtime wazero.Runtime
	// Report describes the guest's wasm imports and exports.
	Report *ImportReport

	random        atomic.Uint32
	pci           *pciDevice
	v86fs         *virtioV86FSDevice
	ne2k          *ne2kDevice
	network       *usernet.Stack
	netWake       chan struct{}
	pit           *pitDevice
	cmos          *cmosDevice
	fwValue       []byte
	fwPointer     int
	optionROMs    []optionROM
	serial        *uartDevice
	serialLog     serialLog
	haltEvents    atomic.Uint64
	exceptions    atomic.Uint64
	lastException atomic.Uint32

	// Logs holds messages written by the guest through its wasm log callbacks.
	Logs []string

	// host9p is the registered 9P device, when present.
	host9p *virtioHost9PDevice
}

// InstantiateHostRuntime compiles and instantiates the v86 wasm image
// wasmBytes with the Go host runtime that serves its devices.
func InstantiateHostRuntime(ctx context.Context, wasmBytes []byte, opts HostRuntimeOptions) (*HostRuntime, error) {
	// Create a wazero runtime that closes if host instantiation fails.
	r := wazero.NewRuntime(ctx)
	closeOnError := true
	defer func() {
		if closeOnError {
			r.Close(ctx)
		}
	}()

	// Compile the v86 wasm image and retain it through module instantiation.
	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "compile v86 wasm with wazero")
	}
	defer compiled.Close(ctx)

	// Inspect v86 imports and construct the host callback and device state.
	report, err := importReport(compiled, wasmBytes)
	if err != nil {
		return nil, err
	}
	externs, err := parseWasmExternImports(wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse v86 wasm imports")
	}
	host := &HostRuntime{
		machineBus: newMachineBus(),
		Runtime:    r,
		Report:     report,
	}

	// Instantiate the shared v86 memory and table before installing host callbacks.
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

	// Instantiate the env re-exports that connect v86 imports to the Go callbacks.
	env, err := buildEnvModule(compiled, externs)
	if err != nil {
		return nil, err
	}
	if _, err := r.InstantiateWithConfig(ctx, env, wazero.NewModuleConfig().WithName(envModuleName)); err != nil {
		return nil, errors.Wrap(err, "instantiate v86 env re-export module")
	}

	// Instantiate the v86 guest and transfer the runtime lifetime to its host.
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("v86"))
	if err != nil {
		return nil, errors.Wrap(err, "instantiate v86 wasm module")
	}
	host.Module = mod
	closeOnError = false

	return host, nil
}

// Close releases the wazero runtime and every module it instantiated.
func (h *HostRuntime) Close(ctx context.Context) error {
	// Treat an absent host runtime as already closed.
	if h == nil || h.Runtime == nil {
		return nil
	}

	// Close the host network stack while retaining its meaningful failure.
	var netErr error
	if h.network != nil {
		if err := h.network.Close(); err != nil && !errors.Is(err, usernet.ErrStackClosed) {
			netErr = err
		}
	}

	// Release every wazero module before returning the network close result.
	if err := h.Runtime.Close(ctx); err != nil {
		return err
	}

	return netErr
}

// RustInit calls v86's wasm initialization export after imports are wired.
func (h *HostRuntime) RustInit(ctx context.Context) error {
	fn := h.Module.ExportedFunction("rust_init")
	if fn == nil {
		return errors.New("v86 wasm does not export rust_init")
	}
	_, err := fn.Call(ctx)
	return errors.Wrap(err, "call v86 rust_init")
}

// HaltEvents returns the number of CPU halt events observed by the host.
func (h *HostRuntime) HaltEvents() uint64 {
	return h.haltEvents.Load()
}

// ExceptionCount returns the number of CPU exception callbacks observed.
func (h *HostRuntime) ExceptionCount() uint64 {
	return h.exceptions.Load()
}

// LastException returns the most recent CPU exception vector observed.
func (h *HostRuntime) LastException() uint32 {
	return h.lastException.Load()
}

// instantiateGoEnv registers Go implementations for every function the v86
// module imports from env.
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

// callback dispatches one env-module import to its host-side behavior.
func (h *HostRuntime) callback(name string, results []api.ValueType) api.GoModuleFunction {
	return api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
		switch name {
		case "microtick":
			stack[0] = api.EncodeF64(h.microtick())
		case "get_rand_int":
			stack[0] = api.EncodeI32(int32(h.random.Add(0x9e3779b9))) //nolint:gosec // wasm i32 uses the PRNG's low 32-bit bit pattern.
		case "cpu_exception_hook":
			h.exceptions.Add(1)
			if len(stack) != 0 {
				h.lastException.Store(api.DecodeU32(stack[0]))
			}
			setZeroResults(stack, results)
		case "run_hardware_timers":
			if len(stack) > 1 {
				acpiEnabled := api.DecodeU32(stack[0]) != 0
				now := api.DecodeF64(stack[1])
				next := 100.0
				if h.pit != nil {
					next = min(next, h.pit.timer(ctx, now, false))
				}
				if h.cmos != nil {
					next = min(next, h.cmos.timer(ctx))
				}
				if acpiEnabled {
					next = min(next, h.apicTimer(ctx, now))
				}
				stack[0] = api.EncodeF64(next)
			} else {
				setZeroResults(stack, results)
			}
		case "cpu_event_halt":
			h.haltEvents.Add(1)
			setZeroResults(stack, results)
		case "io_port_read8":
			stack[0] = api.EncodeU32(h.readIO(ctx, api.DecodeU32(stack[0]), 8))
		case "io_port_read16":
			stack[0] = api.EncodeU32(h.readIO(ctx, api.DecodeU32(stack[0]), 16))
		case "io_port_read32":
			stack[0] = api.EncodeU32(h.readIO(ctx, api.DecodeU32(stack[0]), 32))
		case "io_port_write8":
			h.writeIO(ctx, api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 8)
			setZeroResults(stack, results)
		case "io_port_write16":
			h.writeIO(ctx, api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 16)
			setZeroResults(stack, results)
		case "io_port_write32":
			h.writeIO(ctx, api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 32)
			setZeroResults(stack, results)
		case "mmap_read8":
			stack[0] = api.EncodeU32(h.readMmap(api.DecodeU32(stack[0]), 8))
		case "mmap_read32":
			stack[0] = api.EncodeU32(h.readMmap(api.DecodeU32(stack[0]), 32))
		case "mmap_write8":
			h.writeMmap(api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 8)
			setZeroResults(stack, results)
		case "mmap_write16":
			h.writeMmap(api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 16)
			setZeroResults(stack, results)
		case "mmap_write32":
			h.writeMmap(api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 32)
			setZeroResults(stack, results)
		case "mmap_write64":
			h.writeMmap(api.DecodeU32(stack[0]), api.DecodeU32(stack[1]), 32)
			h.writeMmap(api.DecodeU32(stack[0])+4, api.DecodeU32(stack[2]), 32)
			setZeroResults(stack, results)
		case "mmap_write128":
			addr := api.DecodeU32(stack[0])
			h.writeMmap(addr, api.DecodeU32(stack[1]), 32)
			h.writeMmap(addr+4, api.DecodeU32(stack[2]), 32)
			h.writeMmap(addr+8, api.DecodeU32(stack[3]), 32)
			h.writeMmap(addr+12, api.DecodeU32(stack[4]), 32)
			setZeroResults(stack, results)
		case "jit_clear_func", "jit_clear_all_funcs":
			setZeroResults(stack, results)
		case "codegen_finalize":
			h.codegenFinalize(ctx, stack)
			setZeroResults(stack, results)
		case "log_from_wasm", "console_log_from_wasm":
			h.appendLog(mod, stack)
			setZeroResults(stack, results)
		default:
			setZeroResults(stack, results)
		}
	})
}

// apicTimer advances the guest APIC timer and returns the milliseconds until
// its next tick, or 100 when the export is missing, fails or returns nothing.
func (h *HostRuntime) apicTimer(ctx context.Context, now float64) float64 {
	// Locate the optional guest APIC timer export.
	fn := h.Module.ExportedFunction("apic_timer")
	if fn == nil {
		return 100
	}

	// Advance the guest APIC timer and require a returned deadline.
	result, err := fn.Call(ctx, api.EncodeF64(now))
	if err != nil {
		return 100
	}
	if len(result) == 0 {
		return 100
	}

	return api.DecodeF64(result[0])
}

// codegenFinalize notifies the guest that asynchronous code generation has
// finished.
func (h *HostRuntime) codegenFinalize(ctx context.Context, stack []uint64) {
	if h.Module == nil || len(stack) < 3 {
		return
	}
	fn := h.Module.ExportedFunction("codegen_finalize_finished")
	if fn == nil {
		return
	}
	_, _ = fn.Call(ctx, stack[0], stack[1], stack[2])
}

// appendLog appends a debug-console string written by the guest to Logs.
func (h *HostRuntime) appendLog(mod api.Module, stack []uint64) {
	// Require guest log arguments and readable wasm memory.
	if len(stack) < 2 || mod == nil || mod.Memory() == nil {
		return
	}

	// Read the guest log string from its wasm memory range.
	offset := api.DecodeU32(stack[0])
	byteCount := api.DecodeU32(stack[1])
	data, ok := mod.Memory().Read(offset, byteCount)
	if !ok {
		return
	}

	// Retain the guest console message in the host log.
	h.Logs = append(h.Logs, string(data))
}

// setZeroResults zeroes the return slots of an env callback invocation.
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
