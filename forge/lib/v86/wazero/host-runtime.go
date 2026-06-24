package v86_wazero

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

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

// IOReadFunc handles a v86 IO-port read for a registered width.
type IOReadFunc func(ctx context.Context, port uint16) uint32

// IOWriteFunc handles a v86 IO-port write for a registered width.
type IOWriteFunc func(ctx context.Context, port uint16, value uint32)

// MmapReadFunc handles a v86 memory-mapped read for a registered range.
type MmapReadFunc func(addr uint32) uint32

// MmapWriteFunc handles a v86 memory-mapped write for a registered range.
type MmapWriteFunc func(addr uint32, value uint32)

// HostRuntime is an instantiated v86 wasm module and its Go-owned host surface.
type HostRuntime struct {
	Runtime wazero.Runtime
	Module  api.Module
	Report  *ImportReport

	started           time.Time
	random            atomic.Uint32
	ioPorts           []ioPort
	ioReads           map[uint16]uint64
	ioWrites          map[uint16]uint64
	ioLastReads       map[uint16]uint32
	ioLastWrites      map[uint16]uint32
	mmapBlocks        map[uint32]mmapBlock
	guestMemoryOffset uint32
	guestMemorySize   uint32
	pci               *pciDevice
	v86fs             *virtioV86FSDevice
	ne2k              *ne2kDevice
	network           *usernet.Stack
	netWake           chan struct{}
	pit               *pitDevice
	cmos              *cmosDevice
	fwValue           []byte
	fwPointer         int
	optionROMs        []optionROM
	serial            *uartDevice
	serialOutput      []byte
	serialSink        io.Writer
	haltEvents        atomic.Uint64
	exceptions        atomic.Uint64
	lastException     atomic.Uint32
	Logs              []string
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
		Runtime:      r,
		Report:       report,
		started:      time.Now(),
		ioPorts:      newIOPorts(),
		ioReads:      make(map[uint16]uint64),
		ioWrites:     make(map[uint16]uint64),
		ioLastReads:  make(map[uint16]uint32),
		ioLastWrites: make(map[uint16]uint32),
		mmapBlocks:   make(map[uint32]mmapBlock),
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
	var netErr error
	if h.network != nil {
		if err := h.network.Close(); err != nil && !errors.Is(err, usernet.ErrStackClosed) {
			netErr = err
		}
	}
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

// RegisterIORead installs a handler for one IO-port read width.
func (h *HostRuntime) RegisterIORead(port uint16, width int, fn IOReadFunc) {
	slot := &h.ioPorts[port]
	switch width {
	case 8:
		slot.read8 = fn
	case 16:
		slot.read16 = fn
	case 32:
		slot.read32 = fn
	default:
		panic("unsupported v86 io read width")
	}
}

// RegisterIOWrite installs a handler for one IO-port write width.
func (h *HostRuntime) RegisterIOWrite(port uint16, width int, fn IOWriteFunc) {
	slot := &h.ioPorts[port]
	switch width {
	case 8:
		slot.write8 = fn
	case 16:
		slot.write16 = fn
	case 32:
		slot.write32 = fn
	default:
		panic("unsupported v86 io write width")
	}
}

// RegisterMmap installs handlers for every v86 MMIO block touched by the range.
func (h *HostRuntime) RegisterMmap(start, size uint32, read8 MmapReadFunc, write8 MmapWriteFunc, read32 MmapReadFunc, write32 MmapWriteFunc) {
	if size == 0 {
		return
	}
	first := start >> mmapBlockBits
	last := (start + size - 1) >> mmapBlockBits
	for block := first; block <= last; block++ {
		h.mmapBlocks[block] = mmapBlock{
			read8:   read8,
			write8:  write8,
			read32:  read32,
			write32: write32,
		}
	}
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
	return api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
		switch name {
		case "microtick":
			stack[0] = api.EncodeF64(float64(time.Since(h.started).Microseconds()) / 1000)
		case "get_rand_int":
			stack[0] = api.EncodeI32(int32(h.random.Add(0x9e3779b9)))
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

func (h *HostRuntime) apicTimer(ctx context.Context, now float64) float64 {
	fn := h.Module.ExportedFunction("apic_timer")
	if fn == nil {
		return 100
	}
	result, err := fn.Call(ctx, api.EncodeF64(now))
	if err != nil {
		return 100
	}
	if len(result) == 0 {
		return 100
	}
	return api.DecodeF64(result[0])
}

func (h *HostRuntime) readIO(ctx context.Context, port uint32, width int) uint32 {
	if h.ioReads != nil {
		h.ioReads[uint16(port)]++
	}
	slot := h.ioPorts[uint16(port)]
	var value uint32
	switch width {
	case 8:
		value = slot.read8(ctx, uint16(port)) & 0xff
	case 16:
		value = slot.read16(ctx, uint16(port)) & 0xffff
	case 32:
		value = slot.read32(ctx, uint16(port))
	default:
		value = 0
	}
	if h.ioLastReads != nil {
		h.ioLastReads[uint16(port)] = value
	}
	return value
}

func (h *HostRuntime) writeIO(ctx context.Context, port, value uint32, width int) {
	if h.ioWrites != nil {
		h.ioWrites[uint16(port)]++
	}
	if h.ioLastWrites != nil {
		h.ioLastWrites[uint16(port)] = value
	}
	slot := h.ioPorts[uint16(port)]
	switch width {
	case 8:
		slot.write8(ctx, uint16(port), value&0xff)
	case 16:
		slot.write16(ctx, uint16(port), value&0xffff)
	case 32:
		slot.write32(ctx, uint16(port), value)
	}
}

func (h *HostRuntime) readMmap(addr uint32, width int) uint32 {
	block := h.mmapBlocks[addr>>mmapBlockBits]
	switch width {
	case 8:
		if block.read8 != nil {
			return block.read8(addr) & 0xff
		}
		return 0xff
	case 32:
		if block.read32 != nil {
			return block.read32(addr)
		}
		return 0xffffffff
	default:
		return 0
	}
}

func (h *HostRuntime) writeMmap(addr, value uint32, width int) {
	block := h.mmapBlocks[addr>>mmapBlockBits]
	switch width {
	case 8:
		if block.write8 != nil {
			block.write8(addr, value&0xff)
		}
	case 16:
		h.writeMmap(addr, value&0xff, 8)
		h.writeMmap(addr+1, value>>8, 8)
	case 32:
		if block.write32 != nil {
			block.write32(addr, value)
		}
	}
}

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

func (h *HostRuntime) microtick() float64 {
	return float64(time.Since(h.started).Microseconds()) / 1000
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

type ioPort struct {
	read8   IOReadFunc
	read16  IOReadFunc
	read32  IOReadFunc
	write8  IOWriteFunc
	write16 IOWriteFunc
	write32 IOWriteFunc
}

func newIOPorts() []ioPort {
	ports := make([]ioPort, 0x10000)
	for i := range ports {
		ports[i] = emptyIOPort()
	}
	return ports
}

func emptyIOPort() ioPort {
	return ioPort{
		read8:   func(context.Context, uint16) uint32 { return 0xff },
		read16:  func(context.Context, uint16) uint32 { return 0xffff },
		read32:  func(context.Context, uint16) uint32 { return 0xffffffff },
		write8:  func(context.Context, uint16, uint32) {},
		write16: func(context.Context, uint16, uint32) {},
		write32: func(context.Context, uint16, uint32) {},
	}
}

func (h *HostRuntime) moveIOPorts(from, to uint32, size uint32) {
	if from == to || size == 0 {
		return
	}
	if uint64(from)+uint64(size) > uint64(len(h.ioPorts)) || uint64(to)+uint64(size) > uint64(len(h.ioPorts)) {
		return
	}
	for i := range size {
		h.ioPorts[to+i] = h.ioPorts[from+i]
		h.ioPorts[from+i] = emptyIOPort()
	}
}

type mmapBlock struct {
	read8   MmapReadFunc
	write8  MmapWriteFunc
	read32  MmapReadFunc
	write32 MmapWriteFunc
}
