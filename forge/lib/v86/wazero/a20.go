package v86_wazero

func (h *HostRuntime) registerA20Port() {
	var value uint32
	h.RegisterIORead(0x92, 8, func(uint16) uint32 {
		return value
	})
	h.RegisterIOWrite(0x92, 8, func(_ uint16, next uint32) {
		value = next & 0xff
	})
}
