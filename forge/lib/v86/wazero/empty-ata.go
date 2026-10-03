package v86_wazero

import "context"

const (
	ataPrimaryCommandBase     = 0x1f0
	ataPrimaryControlBase     = 0x3f6
	ataSecondaryCommandBase   = 0x170
	ataSecondaryControlBase   = 0x376
	ataRegAltStatus           = 0x00
	ataBusMasterBase          = 0xb400
	ataBusMasterChannelStride = 8
)

// registerEmptyATA wires both primary and secondary ATA channels.
func (h *HostRuntime) registerEmptyATA() {
	h.registerEmptyATAChannel(ataPrimaryCommandBase, ataPrimaryControlBase, ataBusMasterBase)
	h.registerEmptyATAChannel(ataSecondaryCommandBase, ataSecondaryControlBase, ataBusMasterBase+ataBusMasterChannelStride)
}

// registerEmptyATAChannel stubs one ATA channel at the given port bases:
// command registers read as zero, the alternate status reads zero, and the
func (h *HostRuntime) registerEmptyATAChannel(commandBase, controlBase, busMasterBase uint16) {
	// Expose empty ATA command and control registers on the channel.
	for offset := range uint16(8) {
		port := commandBase + offset
		h.RegisterIORead(port, 8, func(context.Context, uint16) uint32 {
			return 0
		})
		h.RegisterIOWrite(port, 8, func(context.Context, uint16, uint32) {})
	}
	h.RegisterIORead(controlBase+ataRegAltStatus, 8, func(context.Context, uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(controlBase+ataRegAltStatus, 8, func(context.Context, uint16, uint32) {})

	// Expose empty ATA bus-master registers for byte transfers.
	for offset := range uint16(8) {
		port := busMasterBase + offset
		h.RegisterIORead(port, 8, func(context.Context, uint16) uint32 {
			return 0
		})
		h.RegisterIOWrite(port, 8, func(context.Context, uint16, uint32) {})
	}

	// Expose empty ATA bus-master registers for full-width transfers.
	h.RegisterIORead(busMasterBase, 32, func(context.Context, uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(busMasterBase, 32, func(context.Context, uint16, uint32) {})
	h.RegisterIORead(busMasterBase+4, 32, func(context.Context, uint16) uint32 {
		return 0
	})
	h.RegisterIOWrite(busMasterBase+4, 32, func(context.Context, uint16, uint32) {})
}
