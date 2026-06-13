package v86_wazero

import (
	"context"
	"math"
)

const pitOscillatorKHz = 1193.1816666

type pitDevice struct {
	host              *HostRuntime
	counterStartTime  [3]float64
	counterStartValue [3]uint16
	counterNextLow    [4]uint8
	counterEnabled    [4]uint8
	counterMode       [4]uint8
	counterReadMode   [4]uint8
	counterLatch      [4]uint8
	counterLatchValue [3]uint16
	counterReload     [3]uint16
	speakerToggle     uint8
}

func (h *HostRuntime) registerPIT() {
	pit := &pitDevice{host: h}
	h.pit = pit

	h.RegisterIORead(0x61, 8, func(context.Context, uint16) uint32 {
		now := h.microtick()
		pit.speakerToggle ^= 1
		refToggle := uint32(pit.speakerToggle)
		counter2Out := uint32(pit.didRollover(2, now))
		if pit.counterEnabled[2] == 0 {
			counter2Out = refToggle
		}
		return (refToggle << 4) | (counter2Out << 5)
	})
	h.RegisterIOWrite(0x61, 8, func(context.Context, uint16, uint32) {})

	for i := range 3 {
		counter := i
		h.RegisterIORead(uint16(0x40+counter), 8, func(context.Context, uint16) uint32 {
			return uint32(pit.counterRead(counter))
		})
		h.RegisterIOWrite(uint16(0x40+counter), 8, func(_ context.Context, _ uint16, value uint32) {
			pit.counterWrite(counter, uint8(value))
		})
	}
	h.RegisterIOWrite(0x43, 8, func(ctx context.Context, _ uint16, value uint32) {
		pit.writeControl(ctx, uint8(value))
	})
}

func (p *pitDevice) timer(ctx context.Context, now float64, noIRQ bool) float64 {
	next := 100.0
	if noIRQ {
		return next
	}
	if p.counterEnabled[0] != 0 && p.didRollover(0, now) != 0 {
		p.counterStartValue[0] = p.counterValue(0, now)
		p.counterStartTime[0] = now
		_ = p.host.callVoid(ctx, "device_lower_irq", 0)
		_ = p.host.callVoid(ctx, "device_raise_irq", 0)
		if p.counterMode[0] == 0 {
			p.counterEnabled[0] = 0
		}
	} else {
		_ = p.host.callVoid(ctx, "device_lower_irq", 0)
	}
	if p.counterEnabled[0] != 0 {
		diff := now - p.counterStartTime[0]
		diffTicks := math.Floor(diff * pitOscillatorKHz)
		missing := float64(p.counterStartValue[0]) - diffTicks
		next = missing / pitOscillatorKHz
	}
	return next
}

func (p *pitDevice) counterRead(i int) uint8 {
	if p.counterLatch[i] != 0 {
		latch := p.counterLatch[i]
		p.counterLatch[i]--
		if latch == 2 {
			return uint8(p.counterLatchValue[i])
		}
		return uint8(p.counterLatchValue[i] >> 8)
	}

	nextLow := p.counterNextLow[i]
	if p.counterMode[i] == 3 {
		p.counterNextLow[i] ^= 1
	}
	value := p.counterValue(i, p.host.microtick())
	if nextLow != 0 {
		return uint8(value)
	}
	return uint8(value >> 8)
}

func (p *pitDevice) counterWrite(i int, value uint8) {
	if p.counterNextLow[i] != 0 {
		p.counterReload[i] = (p.counterReload[i] &^ 0xff) | uint16(value)
	} else {
		p.counterReload[i] = (p.counterReload[i] & 0xff) | uint16(value)<<8
	}
	if p.counterReadMode[i] != 3 || p.counterNextLow[i] == 0 {
		if p.counterReload[i] == 0 {
			p.counterReload[i] = 0xffff
		}
		p.counterStartValue[i] = p.counterReload[i]
		p.counterEnabled[i] = 1
		p.counterStartTime[i] = p.host.microtick()
	}
	if p.counterReadMode[i] == 3 {
		p.counterNextLow[i] ^= 1
	}
}

func (p *pitDevice) writeControl(ctx context.Context, value uint8) {
	mode := (value >> 1) & 7
	i := int((value >> 6) & 3)
	readMode := (value >> 4) & 3
	if i == 3 {
		return
	}
	if readMode == 0 {
		p.counterLatch[i] = 2
		counterValue := p.counterValue(i, p.host.microtick())
		if counterValue != 0 {
			counterValue--
		}
		p.counterLatchValue[i] = counterValue
		return
	}
	if mode >= 6 {
		mode &^= 4
	}
	switch readMode {
	case 1:
		p.counterNextLow[i] = 1
	case 2:
		p.counterNextLow[i] = 0
	default:
		p.counterNextLow[i] = 1
	}
	if i == 0 {
		_ = p.host.callVoid(ctx, "device_lower_irq", 0)
	}
	p.counterMode[i] = mode
	p.counterReadMode[i] = readMode
}

func (p *pitDevice) counterValue(i int, now float64) uint16 {
	if p.counterEnabled[i] == 0 {
		return 0
	}
	diff := now - p.counterStartTime[i]
	diffTicks := math.Floor(diff * pitOscillatorKHz)
	value := int(p.counterStartValue[i]) - int(diffTicks)
	reload := int(p.counterReload[i])
	if reload == 0 {
		reload = 0x10000
	}
	if value >= reload {
		value %= reload
	} else if value < 0 {
		value = value%reload + reload
	}
	return uint16(value)
}

func (p *pitDevice) didRollover(i int, now float64) uint8 {
	diff := now - p.counterStartTime[i]
	if diff < 0 {
		return 1
	}
	diffTicks := math.Floor(diff * pitOscillatorKHz)
	if float64(p.counterStartValue[i]) < diffTicks {
		return 1
	}
	return 0
}
