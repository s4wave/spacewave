package v86_wazero

import (
	"context"
	"encoding/binary"
)

const (
	fwCfgSignature     = 0x00
	fwCfgID            = 0x01
	fwCfgRamSize       = 0x03
	fwCfgNbCPUs        = 0x05
	fwCfgMaxCPUs       = 0x0f
	fwCfgNuma          = 0x0d
	fwCfgFileDir       = 0x19
	fwCfgCustomStart   = 0x8000
	fwCfgFileStart     = 0xc000
	fwCfgSignatureQEMU = 0x554d4551
)

func (h *HostRuntime) registerFWCfgPorts() {
	h.RegisterIORead(0x511, 8, func(context.Context, uint16) uint32 {
		if h.fwPointer >= len(h.fwValue) {
			return 0
		}
		value := h.fwValue[h.fwPointer]
		h.fwPointer++
		return uint32(value)
	})
	h.RegisterIOWrite(0x510, 16, func(_ context.Context, _ uint16, value uint32) {
		h.fwPointer = 0
		switch {
		case value == fwCfgSignature:
			h.fwValue = le32(fwCfgSignatureQEMU)
		case value == fwCfgID:
			h.fwValue = le32(0)
		case value == fwCfgRamSize:
			h.fwValue = le32(h.guestMemorySize)
		case value == fwCfgNbCPUs || value == fwCfgMaxCPUs:
			h.fwValue = le32(1)
		case value == fwCfgNuma:
			h.fwValue = make([]byte, 16)
		case value == fwCfgFileDir:
			h.fwValue = h.fwCfgFileDir()
		case value >= fwCfgCustomStart && value < fwCfgFileStart:
			h.fwValue = be32(0)
		case value >= fwCfgFileStart && int(value-fwCfgFileStart) < len(h.optionROMs):
			h.fwValue = h.optionROMs[value-fwCfgFileStart].data
		default:
			h.fwValue = le32(0)
		}
	})
}

func (h *HostRuntime) fwCfgFileDir() []byte {
	out := make([]byte, 4+64*len(h.optionROMs))
	binary.BigEndian.PutUint32(out[0:], uint32(len(h.optionROMs)))
	for i, rom := range h.optionROMs {
		ptr := 4 + 64*i
		binary.BigEndian.PutUint32(out[ptr:], uint32(len(rom.data)))
		binary.BigEndian.PutUint16(out[ptr+4:], fwCfgFileStart+uint16(i))
		copy(out[ptr+8:ptr+64], rom.name)
	}
	return out
}

func be32(value uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, value)
	return out
}

func le32(value uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, value)
	return out
}
