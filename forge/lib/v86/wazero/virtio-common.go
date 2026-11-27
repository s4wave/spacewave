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
	// virtioBus returns the machine bus that owns the IO ports.
	virtioBus() *machineBus
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
// the device through dev.reset and returns 0. Otherwise handleStatusWrite
// stores the masked status, raises the ISR on failure, and returns the stored
// value.
func (c *virtioCommonConfig) handleStatusWrite(
	ctx context.Context,
	dev virtioCommonConfigDevice,
	value uint32,
) uint32 {
	// Reset the virtio device when its driver clears the status register.
	if value == 0 {
		dev.reset(ctx)
		return 0
	}

	// Apply virtio feature negotiation and signal driver failure.
	if !c.featuresOK {
		value &^= 8
	}
	c.status = value
	if value&virtioStatusFailed != 0 {
		dev.raiseIRQ(ctx, virtioISRQueue)
	}

	return value
}

// readRegister reads the shared common configuration register at offset:
// the feature selectors and windows, the config generation, and queue_select.
func (c *virtioCommonConfig) readRegister(offset uint16) uint32 {
	switch offset {
	case 0:
		return c.deviceFeatureSelect
	case 4:
		return c.deviceFeatures[c.deviceFeatureSelect&3]
	case 8:
		return c.driverFeatureSelect
	case 12:
		return c.driverFeatures[c.driverFeatureSelect&3]
	case 21:
		return c.configGeneration
	case 22:
		return c.queueSelect
	}
	return 0
}

// writeRegister writes the shared common configuration register at offset.
// The device feature window and the config generation are read-only.
func (c *virtioCommonConfig) writeRegister(offset uint16, value uint32) {
	switch offset {
	case 0:
		c.deviceFeatureSelect = value
	case 8:
		c.driverFeatureSelect = value
	case 12:
		c.writeDriverFeatures(value)
	case 22:
		c.queueSelect = value
	}
}

// resetState clears the common configuration registers.
func (c *virtioCommonConfig) resetState() {
	// Restore the virtio feature selectors and advertised feature set.
	c.driverFeatureSelect = 0
	c.deviceFeatureSelect = 0
	c.driverFeatures = c.deviceFeatures
	c.featuresOK = true

	// Clear the virtio driver status and selected queue.
	c.status = 0
	c.queueSelect = 0
}

// registerVirtioCommonPorts wires the virtio common configuration structure
// to the ISA ports at basePort.
func registerVirtioCommonPorts(basePort uint16, dev virtioCommonConfigDevice) {
	// Describe the virtio common registers. The shared configuration and the
	// selected queue each decode their own registers, and the device answers
	// NUM_QUEUES and STATUS writes.
	bus := dev.virtioBus()
	c := dev.commonConfig()
	nop := func(context.Context, uint32) {}
	constant := func(value uint32) func() uint32 {
		return func() uint32 { return value }
	}
	type field struct {
		offset uint16
		width  int
		read   func() uint32
		write  func(context.Context, uint32)
	}
	common := func(offset uint16, width int) field {
		return field{offset, width, func() uint32 { return c.readRegister(offset) }, func(_ context.Context, value uint32) {
			c.writeRegister(offset, value)
		}}
	}
	queue := func(offset uint16, width int) field {
		return field{offset, width, func() uint32 { return dev.selectedQueue().readRegister(offset) }, func(_ context.Context, value uint32) {
			dev.selectedQueue().writeRegister(offset, value)
		}}
	}
	fields := []field{
		common(0, 32),
		common(4, 32),
		common(8, 32),
		common(12, 32),
		{16, 16, constant(0xffff), nop},
		{18, 16, dev.numQueues, nop},
		{20, 8, func() uint32 { return c.status }, dev.applyStatus},
		common(21, 8),
		common(22, 16),
		queue(24, 16),
		{26, 16, constant(0xffff), nop},
		queue(28, 16),
		queue(30, 16),
		queue(32, 32),
		{36, 32, constant(0), nop},
		queue(40, 32),
		{44, 32, constant(0), nop},
		queue(48, 32),
		{52, 32, constant(0), nop},
	}

	// Register the virtio common fields with their supported I/O access widths.
	for _, field := range fields {
		port := basePort + field.offset
		switch field.width {
		case 8:
			bus.RegisterIORead(port, 8, func(context.Context, uint16) uint32 { return field.read() & 0xff })
			bus.RegisterIOWrite(port, 8, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xff) })
		case 16:
			bus.RegisterIORead(port, 16, func(context.Context, uint16) uint32 { return field.read() & 0xffff })
			bus.RegisterIORead(port, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			bus.RegisterIORead(port+1, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			bus.RegisterIOWrite(port, 16, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xffff) })
		case 32:
			bus.RegisterIORead(port, 32, func(context.Context, uint16) uint32 { return field.read() })
			for i := range uint16(4) {
				offset := i
				bus.RegisterIORead(port+offset, 8, func(context.Context, uint16) uint32 {
					return (field.read() >> (offset * 8)) & 0xff
				})
			}
			bus.RegisterIOWrite(port, 32, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value) })
		}
	}
}

// registerVirtioISRPort wires the ISR status port at port. Reading it returns
// the pending ISR flags and lowers the interrupt line.
func registerVirtioISRPort(port uint16, dev virtioCommonConfigDevice) {
	bus := dev.virtioBus()
	c := dev.commonConfig()
	bus.RegisterIORead(port, 8, func(ctx context.Context, _ uint16) uint32 {
		value := c.isrStatus
		dev.lowerIRQ(ctx)
		return value
	})
	bus.RegisterIOWrite(port, 8, func(context.Context, uint16, uint32) {})
}
