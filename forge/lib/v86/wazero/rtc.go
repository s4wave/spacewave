package v86_wazero

import "time"

const (
	cmosRTCSeconds     = 0x00
	cmosRTCMinutes     = 0x02
	cmosRTCHours       = 0x04
	cmosRTCDayWeek     = 0x06
	cmosRTCDayMonth    = 0x07
	cmosRTCMonth       = 0x08
	cmosRTCYear        = 0x09
	cmosStatusA        = 0x0a
	cmosStatusB        = 0x0b
	cmosStatusC        = 0x0c
	cmosStatusD        = 0x0d
	cmosDiagStatus     = 0x0e
	cmosEquipmentInfo  = 0x14
	cmosMemBaseLow     = 0x15
	cmosMemBaseHigh    = 0x16
	cmosMemOldExtLow   = 0x17
	cmosMemOldExtHigh  = 0x18
	cmosMemExtLow      = 0x30
	cmosMemExtHigh     = 0x31
	cmosCentury        = 0x32
	cmosMemExt2Low     = 0x34
	cmosMemExt2High    = 0x35
	cmosCentury2       = 0x37
	cmosBiosBootflag1  = 0x38
	cmosBiosBootflag2  = 0x3d
	cmosMemHighmemLow  = 0x5b
	cmosMemHighmemMid  = 0x5c
	cmosMemHighmemHigh = 0x5d
	cmosBiosSMPCount   = 0x5f
	bootOrderCDFirst   = 0x123
)

type cmosDevice struct {
	host        *HostRuntime
	index       byte
	data        [128]byte
	statusA     byte
	statusB     byte
	statusC     byte
	diagStatus  byte
	nmiDisabled byte
}

func (h *HostRuntime) registerCMOS() {
	cmos := &cmosDevice{
		host:    h,
		statusA: 0x26,
		statusB: 2,
	}
	cmos.fill(h.guestMemorySize)
	h.RegisterIOWrite(0x70, 8, func(_ uint16, value uint32) {
		cmos.index = byte(value & 0x7f)
		cmos.nmiDisabled = byte(value >> 7)
	})
	h.RegisterIORead(0x71, 8, func(uint16) uint32 {
		return uint32(cmos.read())
	})
	h.RegisterIOWrite(0x71, 8, func(_ uint16, value uint32) {
		cmos.write(byte(value))
	})
}

func (c *cmosDevice) fill(memorySize uint32) {
	bootOrder := bootOrderCDFirst
	c.data[cmosBiosBootflag1] = byte(1 | ((bootOrder >> 4) & 0xf0))
	c.data[cmosBiosBootflag2] = byte(bootOrder & 0xff)
	c.data[cmosMemBaseLow] = 640 & 0xff
	c.data[cmosMemBaseHigh] = 640 >> 8

	memoryAbove1M := uint32(0)
	if memorySize >= 1024*1024 {
		memoryAbove1M = (memorySize - 1024*1024) >> 10
		memoryAbove1M = minU32(memoryAbove1M, 0xffff)
	}
	c.data[cmosMemOldExtLow] = byte(memoryAbove1M)
	c.data[cmosMemOldExtHigh] = byte(memoryAbove1M >> 8)
	c.data[cmosMemExtLow] = byte(memoryAbove1M)
	c.data[cmosMemExtHigh] = byte(memoryAbove1M >> 8)

	memoryAbove16M := uint32(0)
	if memorySize >= 16*1024*1024 {
		memoryAbove16M = (memorySize - 16*1024*1024) >> 16
		memoryAbove16M = minU32(memoryAbove16M, 0xffff)
	}
	c.data[cmosMemExt2Low] = byte(memoryAbove16M)
	c.data[cmosMemExt2High] = byte(memoryAbove16M >> 8)
	c.data[cmosMemHighmemLow] = 0
	c.data[cmosMemHighmemMid] = 0
	c.data[cmosMemHighmemHigh] = 0
	c.data[cmosEquipmentInfo] = 0x2f
	c.data[cmosBiosSMPCount] = 0
	c.data[0x3f] = 1
}

func (c *cmosDevice) read() byte {
	now := time.Now().UTC()
	switch c.index {
	case cmosRTCSeconds:
		return bcdPack(now.Second())
	case cmosRTCMinutes:
		return bcdPack(now.Minute())
	case cmosRTCHours:
		return bcdPack(now.Hour())
	case cmosRTCDayWeek:
		return bcdPack(int(now.Weekday()) + 1)
	case cmosRTCDayMonth:
		return bcdPack(now.Day())
	case cmosRTCMonth:
		return bcdPack(int(now.Month()))
	case cmosRTCYear:
		return bcdPack(now.Year() % 100)
	case cmosStatusA:
		return c.statusA
	case cmosStatusB:
		return c.statusB
	case cmosStatusC:
		value := c.statusC
		c.statusC &^= 0xf0
		return value
	case cmosStatusD:
		return 1 << 7
	case cmosDiagStatus:
		return c.diagStatus
	case cmosCentury, cmosCentury2:
		return bcdPack(now.Year() / 100)
	default:
		return c.data[c.index]
	}
}

func (c *cmosDevice) write(value byte) {
	switch c.index {
	case cmosStatusA:
		c.statusA = value & 0x7f
	case cmosStatusB:
		c.statusB = value
		if c.statusB&0x80 != 0 {
			c.statusB &^= 0x10
		}
	case cmosDiagStatus:
		c.diagStatus = value
	default:
		c.data[c.index] = value
	}
}

func bcdPack(value int) byte {
	return byte((value/10)<<4 | value%10)
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
