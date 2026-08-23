package v86_wazero

import (
	"context"
	"encoding/binary"
)

const (
	pciConfigAddress = 0xcf8
	pciConfigData    = 0xcfc
)

// pciDevice implements the PCI configuration address/data mechanism: the
type pciDevice struct {
	host      *HostRuntime
	addr      uint32
	response  [4]byte
	spaces    map[uint16][]byte
	barSizes  map[uint16]map[int]uint32
	barIO     map[uint16]map[int]bool
	barProbes map[uint16]map[int]bool
}

// registerPCI wires the 0xcf8/0xcfc config ports and seeds the host bridge
func (h *HostRuntime) registerPCI() {
	pci := &pciDevice{
		host: h,
		spaces: map[uint16][]byte{
			0:      newPCIHostBridgeSpace(),
			1 << 3: newPCIISABridgeSpace(),
		},
		barSizes:  make(map[uint16]map[int]uint32),
		barIO:     make(map[uint16]map[int]bool),
		barProbes: make(map[uint16]map[int]bool),
	}
	h.pci = pci

	h.RegisterIOWrite(pciConfigAddress, 32, func(_ context.Context, _ uint16, value uint32) {
		pci.addr = value &^ 3
		pci.query()
	})
	h.RegisterIORead(pciConfigAddress, 32, func(context.Context, uint16) uint32 {
		return pci.addr
	})
	h.RegisterIORead(pciConfigAddress, 16, func(context.Context, uint16) uint32 {
		return pci.addr & 0xffff
	})
	h.RegisterIORead(pciConfigAddress+2, 16, func(context.Context, uint16) uint32 {
		return pci.addr >> 16
	})
	h.RegisterIORead(pciConfigData, 32, func(context.Context, uint16) uint32 {
		return binary.LittleEndian.Uint32(pci.response[:])
	})
	h.RegisterIORead(pciConfigData, 16, func(context.Context, uint16) uint32 {
		return uint32(binary.LittleEndian.Uint16(pci.response[:2]))
	})
	h.RegisterIORead(pciConfigData+2, 16, func(context.Context, uint16) uint32 {
		return uint32(binary.LittleEndian.Uint16(pci.response[2:]))
	})
	h.RegisterIOWrite(pciConfigData, 32, func(_ context.Context, _ uint16, value uint32) {
		pci.write(0, value, 4)
	})
	h.RegisterIOWrite(pciConfigData, 16, func(_ context.Context, _ uint16, value uint32) {
		pci.write(0, value, 2)
	})
	h.RegisterIOWrite(pciConfigData+2, 16, func(_ context.Context, _ uint16, value uint32) {
		pci.write(2, value, 2)
	})

	for offset := range uint16(4) {
		i := offset
		h.RegisterIORead(pciConfigData+i, 8, func(context.Context, uint16) uint32 {
			return uint32(pci.response[i])
		})
		h.RegisterIOWrite(pciConfigData+i, 8, func(_ context.Context, _ uint16, value uint32) {
			pci.write(uint32(i), value, 1)
		})
		h.RegisterIORead(pciConfigAddress+i, 8, func(context.Context, uint16) uint32 {
			return uint32(byte(pci.addr >> (8 * i)))
		})
		h.RegisterIOWrite(pciConfigAddress+i, 8, func(_ context.Context, _ uint16, value uint32) {
			mask := uint32(0xff) << (8 * i)
			pci.addr = (pci.addr &^ mask) | ((value & 0xff) << (8 * i))
			if i == 0 {
				pci.addr &^= 3
			}
			if i == 3 {
				pci.query()
			}
		})
	}
}

// query resolves the latched config address into the response bytes,
func (p *pciDevice) query() {
	p.response = [4]byte{0xff, 0xff, 0xff, 0xff}
	if p.addr&0x80000000 == 0 {
		return
	}
	bdf := uint16((p.addr >> 8) & 0xffff)
	addr := int(p.addr & 0xff)
	space := p.spaces[bdf]
	if space == nil || addr >= len(space) {
		return
	}
	copy(p.response[:], space[addr:])
	if bar := pciBARIndex(addr); bar >= 0 && p.barProbes[bdf] != nil && p.barProbes[bdf][bar] {
		size := p.barSizes[bdf][bar]
		isIO := p.barIO[bdf] != nil && p.barIO[bdf][bar]
		mask := uint32(0xfffffff0)
		if isIO {
			mask = 0xfffffffc
		}
		value := ^(size - 1) & mask
		if isIO {
			value |= 1
		}
		binary.LittleEndian.PutUint32(p.response[:], value)
	}
}

// write stores config-space bytes, detecting BAR sizing probes and moving
func (p *pciDevice) write(offset, value uint32, width int) {
	if p.addr&0x80000000 == 0 {
		return
	}
	bdf := uint16((p.addr >> 8) & 0xffff)
	addr := int(p.addr&0xff) + int(offset)
	space := p.spaces[bdf]
	if space == nil || addr >= len(space) {
		return
	}
	if width == 4 {
		if bar := pciBARIndex(addr); bar >= 0 {
			if _, ok := p.barProbes[bdf]; !ok {
				p.barProbes[bdf] = make(map[int]bool)
			}
			p.barProbes[bdf][bar] = value == 0xffffffff
			if value == 0xffffffff {
				return
			}
			if p.barIO[bdf] != nil && p.barIO[bdf][bar] {
				oldPort := binary.LittleEndian.Uint32(space[addr:]) &^ 3
				value |= 1
				p.host.moveIOPorts(oldPort, value&^3, p.barSizes[bdf][bar])
			}
		}
		binary.LittleEndian.PutUint32(space[addr:], value)
		p.query()
		return
	}
	if width == 2 {
		if pciBARIndex(addr&^3) >= 0 {
			return
		}
		binary.LittleEndian.PutUint16(space[addr:], uint16(value))
		p.query()
		return
	}
	space[addr] = byte(value)
	p.query()
}

// setBARSize records a device BAR's size and IO/memory kind for sizing
func (p *pciDevice) setBARSize(bdf uint16, bar int, size uint32, isIO bool) {
	if _, ok := p.barSizes[bdf]; !ok {
		p.barSizes[bdf] = make(map[int]uint32)
	}
	if _, ok := p.barIO[bdf]; !ok {
		p.barIO[bdf] = make(map[int]bool)
	}
	p.barSizes[bdf][bar] = size
	p.barIO[bdf][bar] = isIO
}

// pciBARIndex returns the BAR index of a config-space offset, or -1 when
func pciBARIndex(addr int) int {
	if addr < 0x10 || addr >= 0x28 || addr&3 != 0 {
		return -1
	}
	return (addr - 0x10) / 4
}

// newPCIHostBridgeSpace builds the config space of the host bridge at bus 0.
func newPCIHostBridgeSpace() []byte {
	space := make([]byte, 256)
	copy(space, []byte{
		0x86, 0x80, 0x37, 0x12,
		0x00, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x06,
		0x00, 0x00, 0x00, 0x00,
	})
	space[0x59] = 0x10
	return space
}

// newPCIISABridgeSpace builds the config space of the ISA bridge device.
func newPCIISABridgeSpace() []byte {
	space := make([]byte, 256)
	copy(space, []byte{
		0x86, 0x80, 0x00, 0x70,
		0x07, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x01, 0x06,
		0x00, 0x00, 0x80, 0x00,
	})
	return space
}
