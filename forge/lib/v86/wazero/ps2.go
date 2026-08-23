package v86_wazero

import "context"

// ps2Device models the i8042 keyboard/mouse controller: its command and data
type ps2Device struct {
	host                 *HostRuntime
	commandRegister      uint8
	controllerOutputPort uint8
	readCommandRegister  bool
	readOutputPort       bool
	nextMouseCommand     bool
	queue                []ps2QueuedByte
	lastData             uint8
}

// ps2QueuedByte is one pending controller output byte and its device kind.
type ps2QueuedByte struct {
	value uint8
	aux   bool
}

// registerPS2 wires the controller's data (0x60) and status/command (0x64)
func (h *HostRuntime) registerPS2() {
	ps2 := &ps2Device{host: h, commandRegister: 1 | 4}
	h.RegisterIORead(0x60, 8, func(ctx context.Context, _ uint16) uint32 {
		return uint32(ps2.readData(ctx))
	})
	h.RegisterIORead(0x64, 8, func(context.Context, uint16) uint32 {
		return uint32(ps2.readStatus())
	})
	h.RegisterIOWrite(0x60, 8, func(ctx context.Context, _ uint16, value uint32) {
		ps2.writeData(ctx, uint8(value))
	})
	h.RegisterIOWrite(0x64, 8, func(ctx context.Context, _ uint16, value uint32) {
		ps2.writeCommand(ctx, uint8(value))
	})
}

// readData pops the oldest queued byte and re-raises the interrupt line for
func (p *ps2Device) readData(ctx context.Context) uint8 {
	if len(p.queue) == 0 {
		return p.lastData
	}
	entry := p.queue[0]
	copy(p.queue, p.queue[1:])
	p.queue = p.queue[:len(p.queue)-1]
	p.lastData = entry.value
	irq := uint32(1)
	if entry.aux {
		irq = 12
	}
	_ = p.host.lowerIRQ(ctx, irq)
	p.raiseIRQ(ctx)
	return entry.value
}

// readStatus renders the status register: output-buffer full plus the aux
func (p *ps2Device) readStatus() uint8 {
	status := uint8(0x10)
	if len(p.queue) != 0 {
		status |= 1
		if p.queue[0].aux {
			status |= 0x20
		}
	}
	return status
}

// writeData delivers a byte to the selected device (keyboard or aux mouse).
func (p *ps2Device) writeData(ctx context.Context, value uint8) {
	switch {
	case p.readCommandRegister:
		p.commandRegister = value
		p.readCommandRegister = false
	case p.readOutputPort:
		p.controllerOutputPort = value
		p.readOutputPort = false
	case p.nextMouseCommand:
		p.nextMouseCommand = false
		p.enqueueAux(ctx, 0xfa)
		switch value {
		case 0xf2:
			p.enqueueAux(ctx, 0)
		case 0xff:
			p.enqueueAux(ctx, 0xaa, 0)
		}
	default:
		p.enqueueKeyboard(ctx, 0xfa)
		switch value {
		case 0xf0:
			// A zero payload asks for the current scan-code set.
		case 0xf2:
			p.enqueueKeyboard(ctx, 0xab, 0x83)
		case 0xff:
			p.queue = nil
			p.enqueueKeyboard(ctx, 0xfa, 0xaa, 0)
		}
	}
}

// writeCommand handles a controller command, possibly consuming the next
func (p *ps2Device) writeCommand(ctx context.Context, value uint8) {
	switch value {
	case 0x20:
		p.queue = nil
		p.enqueueKeyboard(ctx, p.commandRegister)
	case 0x60:
		p.readCommandRegister = true
	case 0xa7:
		p.commandRegister |= 0x20
	case 0xa8:
		p.commandRegister &^= 0x20
	case 0xa9, 0xab:
		p.queue = nil
		p.enqueueKeyboard(ctx, 0)
	case 0xaa:
		p.queue = nil
		p.enqueueKeyboard(ctx, 0x55)
	case 0xad:
		p.commandRegister |= 0x10
	case 0xae:
		p.commandRegister &^= 0x10
	case 0xd0:
		p.queue = nil
		p.enqueueKeyboard(ctx, p.controllerOutputPort)
	case 0xd1:
		p.readOutputPort = true
	case 0xd3:
		p.readOutputPort = true
	case 0xd4:
		p.nextMouseCommand = true
	case 0xfe:
		// Reset requests are ignored by the headless host runtime.
	default:
	}
}

// enqueueKeyboard queues a keyboard byte and raises the IRQ1 line.
func (p *ps2Device) enqueueKeyboard(ctx context.Context, values ...uint8) {
	for _, value := range values {
		p.queue = append(p.queue, ps2QueuedByte{value: value})
	}
	p.raiseIRQ(ctx)
}

// enqueueAux queues an auxiliary (mouse) byte and raises the IRQ12 line.
func (p *ps2Device) enqueueAux(ctx context.Context, values ...uint8) {
	for _, value := range values {
		p.queue = append(p.queue, ps2QueuedByte{value: value, aux: true})
	}
	p.raiseIRQ(ctx)
}

// raiseIRQ asserts whichever interrupt line matches the next queued byte.
func (p *ps2Device) raiseIRQ(ctx context.Context) {
	if len(p.queue) == 0 {
		return
	}
	if p.queue[0].aux {
		if p.commandRegister&2 != 0 {
			_ = p.host.lowerIRQ(ctx, 12)
			_ = p.host.raiseIRQ(ctx, 12)
		}
		return
	}
	if p.commandRegister&1 != 0 {
		_ = p.host.lowerIRQ(ctx, 1)
		_ = p.host.raiseIRQ(ctx, 1)
	}
}
