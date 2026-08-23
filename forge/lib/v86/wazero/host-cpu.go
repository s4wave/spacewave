package v86_wazero

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"
)

const (
	defaultMemorySize        = 64 * 1024 * 1024
	defaultMinimumMemorySize = 1024 * 1024
	guestMemorySizeOffset    = 812
	fpuStackEmptyOffset      = 816
	fpuControlWordOffset     = 1036
	mmapBlockSize            = 1 << mmapBlockBits
)

// InitCPU initializes the v86 CPU memory and optional BIOS images.
func (h *HostRuntime) InitCPU(ctx context.Context, opts HostBootOptions) error {
	if err := h.RustInit(ctx); err != nil {
		return err
	}
	if err := h.createMemory(ctx, opts); err != nil {
		return err
	}
	if !opts.EnableJIT {
		if err := h.callVoid(ctx, "set_jit_config", 0, 1); err != nil {
			return err
		}
	}
	if err := h.callVoid(ctx, "set_tsc", 0, 0); err != nil {
		return err
	}
	if err := h.callVoid(ctx, "reset_cpu"); err != nil {
		return err
	}
	if len(opts.BIOS) != 0 {
		if err := h.loadBIOS(ctx, opts.BIOS, opts.VGABIOS); err != nil {
			return err
		}
	}
	if len(opts.Kernel) != 0 {
		if err := h.loadLinuxKernel(ctx, opts.Kernel, opts.Initrd, opts.kernelCmdline()); err != nil {
			return err
		}
	}
	h.registerFWCfgPorts()
	h.registerA20Port()
	h.registerCMOS()
	h.registerPCI()
	if opts.Networking != nil {
		h.registerNetworking(ctx, *opts.Networking)
	}
	if opts.Host9PFS != nil {
		h.registerHost9P(opts.Host9PFS)
	}
	if opts.V86FSServer != nil {
		h.registerV86FS(ctx, opts.V86FSServer)
	}
	h.registerPIT()
	h.registerPS2()
	h.registerEmptyATA()
	h.registerUART(0x3f8)
	return nil
}

// MainLoop executes one v86 CPU loop and returns the requested delay.
func (h *HostRuntime) MainLoop(ctx context.Context) (float64, error) {
	fn := h.Module.ExportedFunction("main_loop")
	if fn == nil {
		return 0, errors.New("v86 wasm does not export main_loop")
	}
	result, err := fn.Call(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "call v86 main_loop")
	}
	if len(result) == 0 {
		return 0, errors.New("v86 main_loop returned no result")
	}
	return api.DecodeF64(result[0]), nil
}

// RunMainLoop executes a bounded number of v86 CPU loop ticks.
func (h *HostRuntime) RunMainLoop(ctx context.Context, ticks int) error {
	for range ticks {
		h.drainNetwork(ctx)
		if _, err := h.MainLoop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// createMemory allocates guest memory pages and exports them to the module.
func (h *HostRuntime) createMemory(ctx context.Context, opts HostBootOptions) error {
	size := opts.MemorySize
	if size == 0 {
		size = defaultMemorySize
	}
	minimumSize := opts.MinimumMemorySize
	if minimumSize == 0 {
		minimumSize = defaultMinimumMemorySize
	}
	if len(opts.Initrd) != 0 {
		minimumSize = max(minimumSize, initrdAddress+uint32(len(opts.Initrd)))
	}
	if size < minimumSize {
		size = minimumSize
	}
	size = alignGuestMemorySize(size)

	memory := h.Module.Memory()
	if memory == nil {
		return errors.New("v86 wasm has no memory")
	}
	if !memory.WriteUint32Le(guestMemorySizeOffset, size) {
		return errors.New("write v86 memory_size")
	}
	if !memory.WriteByte(fpuStackEmptyOffset, 0xff) {
		return errors.New("write v86 fpu_stack_empty")
	}
	if !memory.WriteUint16Le(fpuControlWordOffset, 0x37f) {
		return errors.New("write v86 fpu_control_word")
	}

	results, err := h.call(ctx, "allocate_memory", uint64(size))
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return errors.New("v86 allocate_memory returned no result")
	}
	h.guestMemoryOffset = api.DecodeU32(results[0])
	h.guestMemorySize = size
	return nil
}

// loadBIOS copies the BIOS images into guest memory at their fixed
func (h *HostRuntime) loadBIOS(ctx context.Context, bios []byte, vgaBIOS []byte) error {
	if len(bios) > 0x100000 {
		return errors.Errorf("BIOS image too large for v86 low memory window: %d bytes", len(bios))
	}
	if err := h.writeGuestBlob(ctx, uint32(0x100000-len(bios)), bios); err != nil {
		return errors.Wrap(err, "write BIOS")
	}
	if len(vgaBIOS) != 0 {
		if err := h.writeGuestBlob(ctx, 0xc0000, vgaBIOS); err != nil {
			return errors.Wrap(err, "write VGA BIOS")
		}
	}
	return nil
}

// writeGuestBlob copies bytes into guest memory at the given linear
func (h *HostRuntime) writeGuestBlob(ctx context.Context, guestOffset uint32, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if h.guestMemorySize == 0 {
		return errors.New("v86 guest memory is not initialized")
	}
	if guestOffset > h.guestMemorySize || uint64(guestOffset)+uint64(len(data)) > uint64(h.guestMemorySize) {
		return errors.Errorf("guest write range [%#x,%#x) exceeds memory size %#x", guestOffset, uint64(guestOffset)+uint64(len(data)), h.guestMemorySize)
	}
	if err := h.callVoid(ctx, "jit_dirty_cache", uint64(guestOffset), uint64(guestOffset)+uint64(len(data))); err != nil {
		return err
	}
	if !h.Module.Memory().Write(h.guestMemoryOffset+guestOffset, data) {
		return errors.Errorf("write guest memory at %#x", guestOffset)
	}
	return nil
}

// callVoid invokes an exported no-result function by name with uint64 args.
func (h *HostRuntime) callVoid(ctx context.Context, name string, args ...uint64) error {
	results, err := h.call(ctx, name, args...)
	if err != nil {
		return err
	}
	if len(results) != 0 {
		return errors.Errorf("v86 %s returned %d results, want none", name, len(results))
	}
	return nil
}

// call invokes an exported function by name and returns its first result.
func (h *HostRuntime) call(ctx context.Context, name string, args ...uint64) ([]uint64, error) {
	fn := h.Module.ExportedFunction(name)
	if fn == nil {
		return nil, errors.Errorf("v86 wasm does not export %s", name)
	}
	results, err := fn.Call(ctx, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "call v86 %s", name)
	}
	return results, nil
}

// alignGuestMemorySize rounds a memory size up to the guest page multiple
func alignGuestMemorySize(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	return (size + mmapBlockSize - 1) &^ (mmapBlockSize - 1)
}
