package v86_wazero

import (
	"context"
	"math"
	"time"
)

const (
	cmosRTCSeconds      = 0x00
	cmosRTCSecondsAlarm = 0x01
	cmosRTCMinutes      = 0x02
	cmosRTCMinutesAlarm = 0x03
	cmosRTCHours        = 0x04
	cmosRTCHoursAlarm   = 0x05
	cmosRTCDayWeek      = 0x06
	cmosRTCDayMonth     = 0x07
	cmosRTCMonth        = 0x08
	cmosRTCYear         = 0x09
	cmosStatusA         = 0x0a
	cmosStatusB         = 0x0b
	cmosStatusC         = 0x0c
	cmosStatusD         = 0x0d
	cmosDiagStatus      = 0x0e
	cmosEquipmentInfo   = 0x14
	cmosMemBaseLow      = 0x15
	cmosMemBaseHigh     = 0x16
	cmosMemOldExtLow    = 0x17
	cmosMemOldExtHigh   = 0x18
	cmosMemExtLow       = 0x30
	cmosMemExtHigh      = 0x31
	cmosCentury         = 0x32
	cmosMemExt2Low      = 0x34
	cmosMemExt2High     = 0x35
	cmosCentury2        = 0x37
	cmosBiosBootflag1   = 0x38
	cmosBiosBootflag2   = 0x3d
	cmosMemHighmemLow   = 0x5b
	cmosMemHighmemMid   = 0x5c
	cmosMemHighmemHigh  = 0x5d
	cmosBiosSMPCount    = 0x5f
	bootOrderCDFirst    = 0x123
)

type cmosDevice struct {
	host                  *HostRuntime
	index                 byte
	data                  [128]byte
	rtcTime               int64
	lastUpdate            int64
	nextPeriodicInterrupt int64
	nextAlarmInterrupt    int64
	periodicInterrupt     bool
	periodicInterruptTime float64
	updateInterrupt       bool
	updateInterruptTime   int64
	statusA               byte
	statusB               byte
	statusC               byte
	diagStatus            byte
	nmiDisabled           byte
}

func (h *HostRuntime) registerCMOS() {
	now := time.Now().UnixMilli()
	cmos := &cmosDevice{
		host:                  h,
		rtcTime:               now,
		lastUpdate:            now,
		periodicInterruptTime: 1000.0 / 1024.0,
		statusA:               0x26,
		statusB:               2,
	}
	h.cmos = cmos
	cmos.fill(h.guestMemorySize)
	h.RegisterIOWrite(0x70, 8, func(_ context.Context, _ uint16, value uint32) {
		cmos.index = byte(value & 0x7f)
		cmos.nmiDisabled = byte(value >> 7)
	})
	h.RegisterIORead(0x71, 8, func(ctx context.Context, _ uint16) uint32 {
		return uint32(cmos.read(ctx))
	})
	h.RegisterIOWrite(0x71, 8, func(ctx context.Context, _ uint16, value uint32) {
		cmos.write(ctx, byte(value))
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
		memoryAbove1M = min(memoryAbove1M, uint32(0xffff))
	}
	c.data[cmosMemOldExtLow] = byte(memoryAbove1M)
	c.data[cmosMemOldExtHigh] = byte(memoryAbove1M >> 8)
	c.data[cmosMemExtLow] = byte(memoryAbove1M)
	c.data[cmosMemExtHigh] = byte(memoryAbove1M >> 8)

	memoryAbove16M := uint32(0)
	if memorySize >= 16*1024*1024 {
		memoryAbove16M = (memorySize - 16*1024*1024) >> 16
		memoryAbove16M = min(memoryAbove16M, uint32(0xffff))
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

func (c *cmosDevice) read(ctx context.Context) byte {
	now := time.UnixMilli(c.rtcTime).UTC()
	switch c.index {
	case cmosRTCSeconds:
		return c.encodeTime(now.Second())
	case cmosRTCMinutes:
		return c.encodeTime(now.Minute())
	case cmosRTCHours:
		return c.encodeTime(now.Hour())
	case cmosRTCDayWeek:
		return c.encodeTime(int(now.Weekday()) + 1)
	case cmosRTCDayMonth:
		return c.encodeTime(now.Day())
	case cmosRTCMonth:
		return c.encodeTime(int(now.Month()))
	case cmosRTCYear:
		return c.encodeTime(now.Year() % 100)
	case cmosStatusA:
		tick := c.host.microtick()
		if tick-float64(int64(tick/1000)*1000) >= 999 {
			return c.statusA | 0x80
		}
		return c.statusA
	case cmosStatusB:
		return c.statusB
	case cmosStatusC:
		value := c.statusC
		c.statusC &^= 0xf0
		_ = c.host.lowerIRQ(ctx, 8)
		return value
	case cmosStatusD:
		return 1 << 7
	case cmosDiagStatus:
		return c.diagStatus
	case cmosCentury, cmosCentury2:
		return c.encodeTime(now.Year() / 100)
	default:
		return c.data[c.index]
	}
}

func (c *cmosDevice) write(_ context.Context, value byte) {
	switch c.index {
	case cmosStatusA:
		c.statusA = value & 0x7f
		rate := c.statusA & 0xf
		if rate > 0 {
			denom := 32768 >> uint(rate-1)
			c.periodicInterruptTime = 1000.0 / float64(denom)
		}
	case cmosStatusB:
		c.statusB = value
		if c.statusB&0x80 != 0 {
			c.statusB &^= 0x10
		}
		now := time.Now().UnixMilli()
		if c.statusB&0x40 != 0 {
			c.nextPeriodicInterrupt = now
		}
		if c.statusB&0x20 != 0 {
			c.nextAlarmInterrupt = c.alarmTime(now)
		}
		if c.statusB&0x10 != 0 {
			c.updateInterruptTime = now
		}
	case cmosDiagStatus:
		c.diagStatus = value
	case cmosRTCSecondsAlarm, cmosRTCMinutesAlarm, cmosRTCHoursAlarm:
		c.data[c.index] = value
	default:
		c.data[c.index] = value
	}
	c.updateInterrupt = c.statusB&0x10 != 0 && c.statusA&0xf > 0
	c.periodicInterrupt = c.statusB&0x40 != 0 && c.statusA&0xf > 0
}

func bcdPack(value int) byte {
	return byte((value/10)<<4 | value%10)
}

func bcdUnpack(value byte) int {
	return int(value&0xf) + int(value>>4)*10
}

func (c *cmosDevice) encodeTime(value int) byte {
	if c.statusB&4 != 0 {
		return byte(value)
	}
	return bcdPack(value)
}

func (c *cmosDevice) decodeTime(value byte) int {
	if c.statusB&4 != 0 {
		return int(value)
	}
	return bcdUnpack(value)
}

func (c *cmosDevice) timer(ctx context.Context) float64 {
	now := time.Now().UnixMilli()
	c.rtcTime += now - c.lastUpdate
	c.lastUpdate = now

	if c.periodicInterrupt && c.nextPeriodicInterrupt < now {
		_ = c.host.raiseIRQ(ctx, 8)
		c.statusC |= (1 << 6) | (1 << 7)
		missed := float64(now-c.nextPeriodicInterrupt) / c.periodicInterruptTime
		c.nextPeriodicInterrupt += int64(c.periodicInterruptTime * math.Ceil(missed))
	}
	if c.nextAlarmInterrupt != 0 && c.nextAlarmInterrupt < now {
		_ = c.host.raiseIRQ(ctx, 8)
		c.statusC |= (1 << 5) | (1 << 7)
		c.nextAlarmInterrupt = 0
	}
	if c.updateInterrupt && c.updateInterruptTime < now {
		_ = c.host.raiseIRQ(ctx, 8)
		c.statusC |= (1 << 4) | (1 << 7)
		c.updateInterruptTime = now + 1000
	}

	next := 100.0
	if c.periodicInterrupt && c.nextPeriodicInterrupt != 0 {
		next = min(next, max(0, float64(c.nextPeriodicInterrupt-now)))
	}
	if c.nextAlarmInterrupt != 0 {
		next = min(next, max(0, float64(c.nextAlarmInterrupt-now)))
	}
	if c.updateInterrupt {
		next = min(next, max(0, float64(c.updateInterruptTime-now)))
	}
	return next
}

func (c *cmosDevice) alarmTime(nowMillis int64) int64 {
	now := time.UnixMilli(nowMillis).UTC()
	return time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		c.decodeTime(c.data[cmosRTCHoursAlarm]),
		c.decodeTime(c.data[cmosRTCMinutesAlarm]),
		c.decodeTime(c.data[cmosRTCSecondsAlarm]),
		0,
		time.UTC,
	).UnixMilli()
}
