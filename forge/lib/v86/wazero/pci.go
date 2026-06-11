package v86_wazero

import (
	"context"
	"encoding/binary"
)

const (
	pciConfigAddress = 0xcf8
	pciConfigData    = 0xcfc
)

type pciDevice struct {
	addr     uint32
	response [4]byte
	spaces   map[uint16][]byte
}

func (h *HostRuntime) registerPCI() {
	pci := &pciDevice{
		spaces: map[uint16][]byte{
			0:      newPCIHostBridgeSpace(),
			1 << 3: newPCIISABridgeSpace(),
		},
	}

	h.RegisterIOWrite(pciConfigAddress, 32, func(_ context.Context, _ uint16, value uint32) {
		pci.addr = value &^ 3
		pci.query()
	})
	h.RegisterIORead(pciConfigAddress, 32, func(context.Context, uint16) uint32 {
		return pci.addr
	})
	h.RegisterIORead(pciConfigData, 32, func(context.Context, uint16) uint32 {
		return binary.LittleEndian.Uint32(pci.response[:])
	})
	h.RegisterIOWrite(pciConfigData, 32, func(context.Context, uint16, uint32) {})

	for offset := range uint16(4) {
		i := offset
		h.RegisterIORead(pciConfigData+i, 8, func(context.Context, uint16) uint32 {
			return uint32(pci.response[i])
		})
		h.RegisterIOWrite(pciConfigData+i, 8, func(context.Context, uint16, uint32) {})
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
}

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
