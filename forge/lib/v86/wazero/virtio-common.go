package v86_wazero

import (
	"context"
)

// virtioCommonConfig holds the virtio common configuration register state
// shared by the PCI virtio devices.
type virtioCommonConfig struct {
	deviceFeatureSelect uint32
	driverFeatureSelect uint32
	deviceFeatures      [4]uint32
	driverFeatures      [4]uint32
	featuresOK          bool
	status              uint32
	configGeneration    uint32
	queueSelect         uint32
	isrStatus           uint32
}

// virtioCommonConfigDevice supplies the per-device seams that the shared
// common-configuration port wiring needs.
type virtioCommonConfigDevice interface {
	// commonConfig exposes the shared register state.
	commonConfig() *virtioCommonConfig
	// virtioHost returns the host runtime that owns the IO ports.
	virtioHost() *HostRuntime
	// numQueues reports the value wired into the NUM_QUEUES field.
	numQueues() uint32
	// selectedQueue returns the queue addressed by queueSelect.
	selectedQueue() *virtioQueue
	// applyStatus applies a STATUS register write after shared handling.
	applyStatus(ctx context.Context, value uint32)
	// reset clears all device state in response to a zero STATUS write.
	reset(ctx context.Context)
	// raiseIRQ asserts the device interrupt line.
	raiseIRQ(ctx context.Context, typ uint32)
	// lowerIRQ deasserts the device interrupt line and clears ISR state.
	lowerIRQ(ctx context.Context)
}

// commonConfig exposes the shared register state.
func (c *virtioCommonConfig) commonConfig() *virtioCommonConfig {
	return c
}

// featureNegotiated reports whether the driver negotiated the feature bit.
func (c *virtioCommonConfig) featureNegotiated(bit uint32) bool {
	idx := bit >> 5
	if idx >= uint32(len(c.driverFeatures)) {
		return false
	}
	return c.driverFeatures[idx]&(1<<(bit&31)) != 0
}

// writeDriverFeatures latches a driver feature write against the advertised
// device features.
func (c *virtioCommonConfig) writeDriverFeatures(value uint32) {
	idx := c.driverFeatureSelect & 3
	supported := c.deviceFeatures[idx]
	c.driverFeatures[idx] = value & supported
	c.featuresOK = c.featuresOK && value&^supported == 0
}

// handleStatusWrite applies the shared STATUS semantics. A zero value resets
// the device through dev.reset and returns 0; otherwise it stores the masked
// status, raises the ISR on failure, and returns the stored value.
func (c *virtioCommonConfig) handleStatusWrite(
	ctx context.Context,
	dev virtioCommonConfigDevice,
	value uint32,
) uint32 {
	if value == 0 {
		dev.reset(ctx)
		return 0
	}
	if !c.featuresOK {
		value &^= 8
	}
	c.status = value
	if value&virtioStatusFailed != 0 {
		dev.raiseIRQ(ctx, virtioISRQueue)
	}
	return value
}

// resetState clears the common configuration registers.
func (c *virtioCommonConfig) resetState() {
	c.driverFeatureSelect = 0
	c.deviceFeatureSelect = 0
	c.driverFeatures = c.deviceFeatures
	c.featuresOK = true
	c.status = 0
	c.queueSelect = 0
}

// registerVirtioCommonPorts wires the virtio common configuration structure
// to the ISA ports at basePort.
func registerVirtioCommonPorts(basePort uint16, dev virtioCommonConfigDevice) {
	host := dev.virtioHost()
	c := dev.commonConfig()
	nop := func(context.Context, uint32) {}
	fields := []struct {
		offset uint16
		width  int
		read   func() uint32
		write  func(context.Context, uint32)
	}{
		{0, 32, func() uint32 { return c.deviceFeatureSelect }, func(_ context.Context, value uint32) { c.deviceFeatureSelect = value }},
		{4, 32, func() uint32 { return c.deviceFeatures[c.deviceFeatureSelect&3] }, nop},
		{8, 32, func() uint32 { return c.driverFeatureSelect }, func(_ context.Context, value uint32) { c.driverFeatureSelect = value }},
		{12, 32, func() uint32 { return c.driverFeatures[c.driverFeatureSelect&3] }, func(_ context.Context, value uint32) {
			c.writeDriverFeatures(value)
		}},
		{16, 16, func() uint32 { return 0xffff }, nop},
		{18, 16, dev.numQueues, nop},
		{20, 8, func() uint32 { return c.status }, dev.applyStatus},
		{21, 8, func() uint32 { return c.configGeneration }, nop},
		{22, 16, func() uint32 { return c.queueSelect }, func(_ context.Context, value uint32) { c.queueSelect = value }},
		{24, 16, func() uint32 { return dev.selectedQueue().size }, func(_ context.Context, value uint32) { dev.selectedQueue().setSize(value) }},
		{26, 16, func() uint32 { return 0xffff }, nop},
		{28, 16, func() uint32 {
			if dev.selectedQueue().enabled {
				return 1
			}
			return 0
		}, func(_ context.Context, value uint32) {
			if value == 1 && dev.selectedQueue().canEnable() {
				dev.selectedQueue().enabled = true
			}
		}},
		{30, 16, func() uint32 { return dev.selectedQueue().notifyOffset }, nop},
		{32, 32, func() uint32 { return dev.selectedQueue().descAddr }, func(_ context.Context, value uint32) { dev.selectedQueue().descAddr = value }},
		{36, 32, func() uint32 { return 0 }, nop},
		{40, 32, func() uint32 { return dev.selectedQueue().availAddr }, func(_ context.Context, value uint32) { dev.selectedQueue().availAddr = value }},
		{44, 32, func() uint32 { return 0 }, nop},
		{48, 32, func() uint32 { return dev.selectedQueue().usedAddr }, func(_ context.Context, value uint32) { dev.selectedQueue().usedAddr = value }},
		{52, 32, func() uint32 { return 0 }, nop},
	}
	for _, field := range fields {
		port := basePort + field.offset
		switch field.width {
		case 8:
			host.RegisterIORead(port, 8, func(context.Context, uint16) uint32 { return field.read() & 0xff })
			host.RegisterIOWrite(port, 8, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xff) })
		case 16:
			host.RegisterIORead(port, 16, func(context.Context, uint16) uint32 { return field.read() & 0xffff })
			host.RegisterIORead(port, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			host.RegisterIORead(port+1, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			host.RegisterIOWrite(port, 16, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xffff) })
		case 32:
			host.RegisterIORead(port, 32, func(context.Context, uint16) uint32 { return field.read() })
			for i := range uint16(4) {
				offset := i
				host.RegisterIORead(port+offset, 8, func(context.Context, uint16) uint32 {
					return (field.read() >> (offset * 8)) & 0xff
				})
			}
			host.RegisterIOWrite(port, 32, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value) })
		}
	}
}

// registerVirtioISRPort wires the ISR status port at port. Reading it returns
// the pending ISR flags and lowers the interrupt line.
func registerVirtioISRPort(port uint16, dev virtioCommonConfigDevice) {
	host := dev.virtioHost()
	c := dev.commonConfig()
	host.RegisterIORead(port, 8, func(ctx context.Context, _ uint16) uint32 {
		value := c.isrStatus
		dev.lowerIRQ(ctx)
		return value
	})
	host.RegisterIOWrite(port, 8, func(context.Context, uint16, uint32) {})
}
