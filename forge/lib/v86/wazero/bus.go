package v86_wazero

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"
)

// machineBus is what the emulated devices share: the I/O port table, the
// memory-mapped blocks, the guest's interrupt lines and other v86 exports,
// guest memory, and the clock. Only the goroutine driving the guest uses it.
type machineBus struct {
	// Module is the instantiated v86 guest.
	Module api.Module

	// started is when the runtime started, the zero of microtick.
	started time.Time
	// ioPorts is the handlers of every I/O port.
	ioPorts []ioPort
	// ioReads, ioWrites, ioLastReads and ioLastWrites count and keep the
	// last value of each port's reads and writes, for diagnostics.
	ioReads      map[uint16]uint64
	ioWrites     map[uint16]uint64
	ioLastReads  map[uint16]uint32
	ioLastWrites map[uint16]uint32
	// mmapBlocks is the handlers of each memory-mapped block.
	mmapBlocks map[uint32]mmapBlock
	// guestMemoryOffset and guestMemorySize place guest physical memory in
	// the module's linear memory.
	guestMemoryOffset uint32
	guestMemorySize   uint32
}

// newMachineBus returns a bus with open-bus ports and no mapped blocks,
// whose clock starts now.
func newMachineBus() *machineBus {
	return &machineBus{
		started:      time.Now(),
		ioPorts:      newIOPorts(),
		ioReads:      make(map[uint16]uint64),
		ioWrites:     make(map[uint16]uint64),
		ioLastReads:  make(map[uint16]uint32),
		ioLastWrites: make(map[uint16]uint32),
		mmapBlocks:   make(map[uint32]mmapBlock),
	}
}

// IOReadFunc handles a v86 IO-port read for a registered width.
type IOReadFunc func(ctx context.Context, port uint16) uint32

// IOWriteFunc handles a v86 IO-port write for a registered width.
type IOWriteFunc func(ctx context.Context, port uint16, value uint32)

// MmapReadFunc handles a v86 memory-mapped read for a registered range.
type MmapReadFunc func(addr uint32) uint32

// MmapWriteFunc handles a v86 memory-mapped write for a registered range.
type MmapWriteFunc func(addr uint32, value uint32)

// RegisterIORead installs a handler for one IO-port read width.
func (b *machineBus) RegisterIORead(port uint16, width int, fn IOReadFunc) {
	slot := &b.ioPorts[port]
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
func (b *machineBus) RegisterIOWrite(port uint16, width int, fn IOWriteFunc) {
	slot := &b.ioPorts[port]
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
func (b *machineBus) RegisterMmap(start, size uint32, read8 MmapReadFunc, write8 MmapWriteFunc, read32 MmapReadFunc, write32 MmapWriteFunc) {
	if size == 0 {
		return
	}
	first := start >> mmapBlockBits
	last := (start + size - 1) >> mmapBlockBits
	for block := first; block <= last; block++ {
		b.mmapBlocks[block] = mmapBlock{
			read8:   read8,
			write8:  write8,
			read32:  read32,
			write32: write32,
		}
	}
}

// readIO services a guest IO port read through its registered handler.
func (b *machineBus) readIO(ctx context.Context, port uint32, width int) uint32 {
	// Count the guest port read in the host I/O diagnostics.
	b.ioReads[uint16(port)]++ //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.

	// Dispatch the guest read at its requested hardware port width.
	slot := b.ioPorts[uint16(port)] //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.
	var value uint32
	switch width {
	case 8:
		value = slot.read8(ctx, uint16(port)) & 0xff //nolint:gosec // the registered port is an x86 16-bit IO address.
	case 16:
		value = slot.read16(ctx, uint16(port)) & 0xffff //nolint:gosec // the registered port is an x86 16-bit IO address.
	case 32:
		value = slot.read32(ctx, uint16(port)) //nolint:gosec // the registered port is an x86 16-bit IO address.
	default:
		value = 0
	}

	// Retain the last guest port value for host I/O diagnostics.
	b.ioLastReads[uint16(port)] = value //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.

	return value
}

// writeIO services a guest IO port write through its registered handler.
func (b *machineBus) writeIO(ctx context.Context, port, value uint32, width int) {
	b.ioWrites[uint16(port)]++           //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.
	b.ioLastWrites[uint16(port)] = value //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.
	slot := b.ioPorts[uint16(port)]      //nolint:gosec // x86 IO port addresses are the low 16 bits of the guest port value.
	switch width {
	case 8:
		slot.write8(ctx, uint16(port), value&0xff) //nolint:gosec // the registered port is an x86 16-bit IO address.
	case 16:
		slot.write16(ctx, uint16(port), value&0xffff) //nolint:gosec // the registered port is an x86 16-bit IO address.
	case 32:
		slot.write32(ctx, uint16(port), value) //nolint:gosec // the registered port is an x86 16-bit IO address.
	}
}

// readMmap services a memory-mapped device read from its registered block.
func (b *machineBus) readMmap(addr uint32, width int) uint32 {
	block := b.mmapBlocks[addr>>mmapBlockBits]
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

// writeMmap services a memory-mapped device write to its registered block.
func (b *machineBus) writeMmap(addr, value uint32, width int) {
	block := b.mmapBlocks[addr>>mmapBlockBits]
	switch width {
	case 8:
		if block.write8 != nil {
			block.write8(addr, value&0xff)
		}
	case 16:
		b.writeMmap(addr, value&0xff, 8)
		b.writeMmap(addr+1, value>>8, 8)
	case 32:
		if block.write32 != nil {
			block.write32(addr, value)
		}
	}
}

// microtick returns milliseconds elapsed since the runtime started.
func (b *machineBus) microtick() float64 {
	return float64(time.Since(b.started).Microseconds()) / 1000
}

// moveIOPorts relocates size ports from one base to another after a BAR write.
func (b *machineBus) moveIOPorts(from, to uint32, size uint32) {
	if from == to || size == 0 {
		return
	}
	if uint64(from)+uint64(size) > uint64(len(b.ioPorts)) || uint64(to)+uint64(size) > uint64(len(b.ioPorts)) {
		return
	}
	for i := range size {
		b.ioPorts[to+i] = b.ioPorts[from+i]
		b.ioPorts[from+i] = emptyIOPort()
	}
}

// ioPort holds the registered handlers for one IO port address.
type ioPort struct {
	read8   IOReadFunc
	read16  IOReadFunc
	read32  IOReadFunc
	write8  IOWriteFunc
	write16 IOWriteFunc
	write32 IOWriteFunc
}

// newIOPorts builds the full 64K IO port table with empty handlers.
func newIOPorts() []ioPort {
	ports := make([]ioPort, 0x10000)
	for i := range ports {
		ports[i] = emptyIOPort()
	}
	return ports
}

// emptyIOPort returns open-bus handlers: reads return all ones and writes do
// nothing.
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

// mmapBlock holds the registered handlers for one memory-mapped block.
type mmapBlock struct {
	read8   MmapReadFunc
	write8  MmapWriteFunc
	read32  MmapReadFunc
	write32 MmapWriteFunc
}

// raiseIRQ asserts a guest ISA interrupt line.
func (b *machineBus) raiseIRQ(ctx context.Context, irq uint32) error {
	if b.Module == nil {
		return nil
	}
	return b.callVoid(ctx, "device_raise_irq", uint64(irq))
}

// lowerIRQ deasserts a guest ISA interrupt line.
func (b *machineBus) lowerIRQ(ctx context.Context, irq uint32) error {
	if b.Module == nil {
		return nil
	}
	return b.callVoid(ctx, "device_lower_irq", uint64(irq))
}

// callVoid invokes an exported no-result function by name with uint64 args.
func (b *machineBus) callVoid(ctx context.Context, name string, args ...uint64) error {
	results, err := b.call(ctx, name, args...)
	if err != nil {
		return err
	}
	if len(results) != 0 {
		return errors.Errorf("v86 %s returned %d results, want none", name, len(results))
	}
	return nil
}

// call invokes an exported function by name and returns its first result.
func (b *machineBus) call(ctx context.Context, name string, args ...uint64) ([]uint64, error) {
	// Resolve the requested v86 wasm export.
	fn := b.Module.ExportedFunction(name)
	if fn == nil {
		return nil, errors.Errorf("v86 wasm does not export %s", name)
	}

	// Invoke the v86 export with its arguments and preserve error context.
	results, err := fn.Call(ctx, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "call v86 %s", name)
	}

	return results, nil
}

// guestRead copies bytes out of guest linear memory, reporting ok=false
// when the read is out of bounds.
func (b *machineBus) guestRead(addr, size uint32) ([]byte, bool) {
	if uint64(addr)+uint64(size) > uint64(b.guestMemorySize) {
		return nil, false
	}
	data, ok := b.Module.Memory().Read(b.guestMemoryOffset+addr, size)
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

// guestWrite copies bytes into guest linear memory, reporting success.
func (b *machineBus) guestWrite(addr uint32, data []byte) bool {
	if uint64(addr)+uint64(len(data)) > uint64(b.guestMemorySize) {
		return false
	}
	return b.Module.Memory().Write(b.guestMemoryOffset+addr, data)
}

// guestReadUint16 reads one little-endian uint16 from guest memory.
func (b *machineBus) guestReadUint16(addr uint32) uint16 {
	data, ok := b.guestRead(addr, 2)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint16(data)
}

// guestReadUint32 reads one little-endian uint32 from guest memory.
func (b *machineBus) guestReadUint32(addr uint32) uint32 {
	data, ok := b.guestRead(addr, 4)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

// guestWriteUint16 writes one little-endian uint16 to guest memory.
func (b *machineBus) guestWriteUint16(addr uint32, value uint16) {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	b.guestWrite(addr, data[:])
}

// guestWriteUint32 writes one little-endian uint32 to guest memory.
func (b *machineBus) guestWriteUint32(addr, value uint32) {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	b.guestWrite(addr, data[:])
}
