package v86_wazero

import (
	"context"

	"github.com/pkg/errors"
)

const (
	uartDLAB                = 0x80
	uartIerMSI              = 0x08
	uartIerTHRI             = 0x02
	uartIerRDI              = 0x01
	uartIirMSI              = 0x00
	uartIirNoInt            = 0x01
	uartIirTHRI             = 0x02
	uartIirRDI              = 0x04
	uartIirCTI              = 0x0c
	uartMcrLoopback         = 0x10
	uartLsrDataReady        = 0x01
	uartLsrTXEmpty          = 0x20
	uartLsrTransmitterEmpty = 0x40
)

type uartDevice struct {
	host            *HostRuntime
	port            uint16
	irq             uint32
	ints            uint32
	baudRate        uint32
	lineControl     uint32
	lsr             uint32
	fifoControl     uint32
	ier             uint32
	iir             uint32
	modemControl    uint32
	modemStatus     uint32
	scratchRegister uint32
	input           []byte
}

func (h *HostRuntime) registerUART(port uint16) {
	irq := uint32(4)
	if port == 0x2f8 || port == 0x2e8 {
		irq = 3
	}
	u := &uartDevice{
		host: h,
		port: port,
		irq:  irq,
		ints: 1 << uartIirTHRI,
		lsr:  uartLsrTransmitterEmpty | uartLsrTXEmpty,
		iir:  uartIirNoInt,
	}
	if port == 0x3f8 {
		h.serial = u
	}
	u.register()
}

func (u *uartDevice) register() {
	u.host.RegisterIOWrite(u.port, 8, func(ctx context.Context, _ uint16, value uint32) {
		u.writeData(ctx, value)
	})
	u.host.RegisterIOWrite(u.port, 16, func(ctx context.Context, _ uint16, value uint32) {
		u.writeData(ctx, value&0xff)
		u.writeData(ctx, value>>8)
	})
	u.host.RegisterIORead(u.port, 8, func(ctx context.Context, _ uint16) uint32 {
		if u.lineControl&uartDLAB != 0 {
			return u.baudRate & 0xff
		}
		if len(u.input) == 0 {
			return 0
		}
		data := u.input[0]
		u.input = u.input[1:]
		if len(u.input) == 0 {
			u.lsr &^= uartLsrDataReady
			u.clearInterrupt(ctx, uartIirCTI)
			u.clearInterrupt(ctx, uartIirRDI)
		}
		return uint32(data)
	})

	u.host.RegisterIOWrite(u.port|1, 8, func(ctx context.Context, _ uint16, value uint32) {
		if u.lineControl&uartDLAB != 0 {
			u.baudRate = (u.baudRate & 0xff) | (value << 8)
			return
		}
		if u.ier&uartIirTHRI == 0 && value&uartIirTHRI != 0 {
			u.throwInterrupt(ctx, uartIirTHRI)
		}
		u.ier = value & 0xf
		u.checkInterrupt(ctx)
	})
	u.host.RegisterIORead(u.port|1, 8, func(context.Context, uint16) uint32 {
		if u.lineControl&uartDLAB != 0 {
			return u.baudRate >> 8
		}
		return u.ier & 0xf
	})

	u.host.RegisterIORead(u.port|2, 8, func(ctx context.Context, _ uint16) uint32 {
		ret := u.iir & 0xf
		if u.iir == uartIirTHRI {
			u.clearInterrupt(ctx, uartIirTHRI)
		}
		if u.fifoControl&1 != 0 {
			ret |= 0xc0
		}
		return ret
	})
	u.host.RegisterIOWrite(u.port|2, 8, func(_ context.Context, _ uint16, value uint32) {
		u.fifoControl = value
	})

	u.host.RegisterIORead(u.port|3, 8, func(context.Context, uint16) uint32 { return u.lineControl })
	u.host.RegisterIOWrite(u.port|3, 8, func(_ context.Context, _ uint16, value uint32) { u.lineControl = value })
	u.host.RegisterIORead(u.port|4, 8, func(context.Context, uint16) uint32 { return u.modemControl })
	u.host.RegisterIOWrite(u.port|4, 8, func(_ context.Context, _ uint16, value uint32) { u.modemControl = value })
	u.host.RegisterIORead(u.port|5, 8, func(context.Context, uint16) uint32 { return u.lsr })
	u.host.RegisterIOWrite(u.port|5, 8, func(context.Context, uint16, uint32) {})
	u.host.RegisterIORead(u.port|6, 8, func(context.Context, uint16) uint32 {
		value := u.modemStatus
		u.modemStatus &= 0xf0
		return value
	})
	u.host.RegisterIOWrite(u.port|6, 8, func(_ context.Context, _ uint16, value uint32) { u.setModemStatus(value) })
	u.host.RegisterIORead(u.port|7, 8, func(context.Context, uint16) uint32 { return u.scratchRegister })
	u.host.RegisterIOWrite(u.port|7, 8, func(_ context.Context, _ uint16, value uint32) { u.scratchRegister = value })
}

func (u *uartDevice) writeData(ctx context.Context, value uint32) {
	if u.lineControl&uartDLAB != 0 {
		u.baudRate = (u.baudRate &^ 0xff) | (value & 0xff)
		return
	}
	u.throwInterrupt(ctx, uartIirTHRI)
	if u.modemControl&uartMcrLoopback != 0 {
		u.receive(ctx, byte(value))
		return
	}
	u.host.serialOutput = append(u.host.serialOutput, byte(value))
}

func (u *uartDevice) receive(ctx context.Context, value byte) {
	u.input = append(u.input, value)
	u.lsr |= uartLsrDataReady
	if u.fifoControl&1 != 0 {
		u.throwInterrupt(ctx, uartIirCTI)
		return
	}
	u.throwInterrupt(ctx, uartIirRDI)
}

func (u *uartDevice) checkInterrupt(ctx context.Context) {
	switch {
	case u.ints&(1<<uartIirCTI) != 0 && u.ier&uartIerRDI != 0:
		u.iir = uartIirCTI
		_ = u.host.raiseIRQ(ctx, u.irq)
	case u.ints&(1<<uartIirRDI) != 0 && u.ier&uartIerRDI != 0:
		u.iir = uartIirRDI
		_ = u.host.raiseIRQ(ctx, u.irq)
	case u.ints&(1<<uartIirTHRI) != 0 && u.ier&uartIerTHRI != 0:
		u.iir = uartIirTHRI
		_ = u.host.raiseIRQ(ctx, u.irq)
	case u.ints&(1<<uartIirMSI) != 0 && u.ier&uartIerMSI != 0:
		u.iir = uartIirMSI
		_ = u.host.raiseIRQ(ctx, u.irq)
	default:
		u.iir = uartIirNoInt
		_ = u.host.lowerIRQ(ctx, u.irq)
	}
}

func (u *uartDevice) throwInterrupt(ctx context.Context, line uint32) {
	u.ints |= 1 << line
	u.checkInterrupt(ctx)
}

func (u *uartDevice) clearInterrupt(ctx context.Context, line uint32) {
	u.ints &^= 1 << line
	u.checkInterrupt(ctx)
}

func (u *uartDevice) setModemStatus(status uint32) {
	delta := (u.modemStatus ^ status) >> 4
	delta |= u.modemStatus & 0x0f
	u.modemStatus = status | delta
}

func (h *HostRuntime) raiseIRQ(ctx context.Context, irq uint32) error {
	if h.Module == nil {
		return nil
	}
	return h.callVoid(ctx, "device_raise_irq", uint64(irq))
}

func (h *HostRuntime) lowerIRQ(ctx context.Context, irq uint32) error {
	if h.Module == nil {
		return nil
	}
	return h.callVoid(ctx, "device_lower_irq", uint64(irq))
}

// WriteSerialInput queues bytes received by the COM1 UART.
func (h *HostRuntime) WriteSerialInput(ctx context.Context, data []byte) error {
	if h.serial == nil {
		return errors.New("v86 COM1 UART is not initialized")
	}
	for _, value := range data {
		h.serial.receive(ctx, value)
	}
	return nil
}

// SerialOutput returns bytes transmitted by the COM1 UART.
func (h *HostRuntime) SerialOutput() []byte {
	return append([]byte(nil), h.serialOutput...)
}
