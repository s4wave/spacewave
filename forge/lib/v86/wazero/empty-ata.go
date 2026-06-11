package v86_wazero

const (
	ataPrimaryCommandBase     = 0x1f0
	ataPrimaryControlBase     = 0x3f6
	ataSecondaryCommandBase   = 0x170
	ataSecondaryControlBase   = 0x376
	ataRegStatus              = 0x07
	ataRegAltStatus           = 0x00
	ataBusMasterBase          = 0xb400
	ataBusMasterChannelStride = 8
)

func (h *HostRuntime) registerEmptyATA() {
	h.registerEmptyATAChannel(ataPrimaryCommandBase, ataPrimaryControlBase, ataBusMasterBase)
	h.registerEmptyATAChannel(ataSecondaryCommandBase, ataSecondaryControlBase, ataBusMasterBase+ataBusMasterChannelStride)
}

func (h *HostRuntime) registerEmptyATAChannel(commandBase, controlBase, busMasterBase uint16) {
	for offset := uint16(0); offset < 8; offset++ {
		port := commandBase + offset
		value := uint32(0)
		if offset == ataRegStatus {
			value = 0
		}
		h.RegisterIORead(port, 8, func(uint16) uint32 {
			return value
		})
		h.RegisterIOWrite(port, 8, func(uint16, uint32) {})
	}
	h.RegisterIORead(controlBase+ataRegAltStatus, 8, func(uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(controlBase+ataRegAltStatus, 8, func(uint16, uint32) {})

	for offset := uint16(0); offset < 8; offset++ {
		port := busMasterBase + offset
		h.RegisterIORead(port, 8, func(uint16) uint32 {
			return 0
		})
		h.RegisterIOWrite(port, 8, func(uint16, uint32) {})
	}
	h.RegisterIORead(busMasterBase, 32, func(uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(busMasterBase, 32, func(uint16, uint32) {})
	h.RegisterIORead(busMasterBase+4, 32, func(uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(busMasterBase+4, 32, func(uint16, uint32) {})
}
