package v86_wazero

import "context"

// registerA20Port wires port 0x92, the fast A20 gate and reset register.
func (h *HostRuntime) registerA20Port() {
	var value uint32
	h.RegisterIORead(0x92, 8, func(context.Context, uint16) uint32 {
		return value
	})
	h.RegisterIOWrite(0x92, 8, func(_ context.Context, _ uint16, next uint32) {
		value = next & 0xff
	})
}
