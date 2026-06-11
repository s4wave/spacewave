package v86_wazero

type ps2Device struct {
	commandRegister      uint8
	controllerOutputPort uint8
	readCommandRegister  bool
	readOutputPort       bool
	nextMouseCommand     bool
	queue                []uint8
	lastData             uint8
}

func (h *HostRuntime) registerPS2() {
	ps2 := &ps2Device{commandRegister: 1 | 4}
	h.RegisterIORead(0x60, 8, func(uint16) uint32 {
		return uint32(ps2.readData())
	})
	h.RegisterIORead(0x64, 8, func(uint16) uint32 {
		return uint32(ps2.readStatus())
	})
	h.RegisterIOWrite(0x60, 8, func(_ uint16, value uint32) {
		ps2.writeData(uint8(value))
	})
	h.RegisterIOWrite(0x64, 8, func(_ uint16, value uint32) {
		ps2.writeCommand(uint8(value))
	})
}

func (p *ps2Device) readData() uint8 {
	if len(p.queue) == 0 {
		return p.lastData
	}
	value := p.queue[0]
	copy(p.queue, p.queue[1:])
	p.queue = p.queue[:len(p.queue)-1]
	p.lastData = value
	return value
}

func (p *ps2Device) readStatus() uint8 {
	status := uint8(0x10)
	if len(p.queue) != 0 {
		status |= 1
	}
	return status
}

func (p *ps2Device) writeData(value uint8) {
	switch {
	case p.readCommandRegister:
		p.commandRegister = value
		p.readCommandRegister = false
	case p.readOutputPort:
		p.controllerOutputPort = value
		p.readOutputPort = false
	case p.nextMouseCommand:
		p.nextMouseCommand = false
		p.enqueue(0xfa)
		switch value {
		case 0xf2:
			p.enqueue(0)
		case 0xff:
			p.enqueue(0xaa, 0)
		}
	default:
		p.enqueue(0xfa)
		switch value {
		case 0xf0:
			// A zero payload asks for the current scan-code set.
		case 0xf2:
			p.enqueue(0xab, 0x83)
		case 0xff:
			p.queue = []uint8{0xfa, 0xaa, 0}
		}
	}
}

func (p *ps2Device) writeCommand(value uint8) {
	switch value {
	case 0x20:
		p.queue = []uint8{p.commandRegister}
	case 0x60:
		p.readCommandRegister = true
	case 0xa7:
		p.commandRegister |= 0x20
	case 0xa8:
		p.commandRegister &^= 0x20
	case 0xa9, 0xab:
		p.queue = []uint8{0}
	case 0xaa:
		p.queue = []uint8{0x55}
	case 0xad:
		p.commandRegister |= 0x10
	case 0xae:
		p.commandRegister &^= 0x10
	case 0xd0:
		p.queue = []uint8{p.controllerOutputPort}
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

func (p *ps2Device) enqueue(values ...uint8) {
	p.queue = append(p.queue, values...)
}
