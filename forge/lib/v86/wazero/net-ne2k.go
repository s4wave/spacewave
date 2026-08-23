package v86_wazero

import (
	"context"
	"encoding/binary"
	"sync"
)

const (
	ne2kPCIPortBase = 0x300
	ne2kPCIPortStep = 0x100
	ne2kBARSize     = 0x20
	ne2kIRQ         = 11

	ne2kMemoryPages = 0x80
	ne2kPageSize    = 0x100
	ne2kStartPage   = 0x40
	ne2kStartRXPage = 0x4c
	ne2kStopPage    = 0x80
	ne2kMinFrameLen = 60

	e8390Cmd = 0x00

	en0Startpg  = 0x01
	en0Stoppg   = 0x02
	en0Boundary = 0x03
	en0TSR      = 0x04
	en0TPSR     = 0x04
	en0TCNTLO   = 0x05
	en0TCNTHI   = 0x06
	en0ISR      = 0x07
	en0RSARLO   = 0x08
	en0RSARHI   = 0x09
	en0RCNTLO   = 0x0a
	en0RCNTHI   = 0x0b
	en0RSR      = 0x0c
	en0RXCR     = 0x0c
	en0TXCR     = 0x0d
	en0DCFG     = 0x0e
	en0IMR      = 0x0f

	ne2kDataPort = 0x10
	ne2kReset    = 0x1f

	ne2kCRStop = 0x01
	ne2kCRTXP  = 0x04
	ne2kCRRDMA = 0x18

	enisrRX    = 0x01
	enisrTX    = 0x02
	enisrRDC   = 0x40
	enisrReset = 0x80

	enrsrRXOK = 0x01

	enrxcrAB  = 0x04
	enrxcrAM  = 0x08
	enrxcrPRO = 0x10
)

// ne2kDevice models an NE2000-compatible Ethernet adapter: the 8390
type ne2kDevice struct {
	host *HostRuntime
	id   int
	port uint16
	bdf  uint16

	isr      byte
	imr      byte
	cr       byte
	dcfg     byte
	rcnt     uint16
	tcnt     uint16
	tpsr     byte
	memory   []byte
	rxcr     byte
	txcr     byte
	tsr      byte
	rsar     uint16
	pstart   byte
	pstop    byte
	curpg    byte
	boundary byte
	mac      [6]byte
	mar      [8]byte

	irqAsserted bool
	outbound    func(frame []byte)

	mtx     sync.Mutex
	inbound [][]byte
}

// newNE2KDevice builds a device at its per-id IO base with a reset state.
func newNE2KDevice(host *HostRuntime, id int, mac [6]byte) *ne2kDevice {
	d := &ne2kDevice{
		host:   host,
		id:     id,
		port:   ne2kPort(id),
		bdf:    ne2kPCIID(id),
		memory: make([]byte, ne2kMemoryPages*ne2kPageSize),
		mac:    mac,
		mar:    [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}
	d.resetState()
	return d
}

// registerNE2K wires the device's PCI BAR and IO ports on the host runtime.
func (h *HostRuntime) registerNE2K(ctx context.Context, id int, mac [6]byte) *ne2kDevice {
	dev := newNE2KDevice(h, id, mac)
	_ = dev.register(ctx)
	return dev
}

// register maps the device into PCI config space and wires byte-wide port
func (d *ne2kDevice) register(ctx context.Context) error {
	if d.host == nil || d.host.pci == nil {
		return nil
	}
	d.host.pci.spaces[d.bdf] = newNE2KPCISpace(d.port)
	d.host.pci.setBARSize(d.bdf, 0, ne2kBARSize, true)
	for i := range uint16(ne2kBARSize) {
		offset := i
		port := d.port + offset
		d.host.RegisterIORead(port, 8, func(ctx context.Context, _ uint16) uint32 {
			return uint32(d.readPort(ctx, offset))
		})
		d.host.RegisterIOWrite(port, 8, func(ctx context.Context, _ uint16, value uint32) {
			d.writePort(ctx, offset, byte(value))
		})
	}

	// Word and long accesses target the remote-DMA data port only. Each moves two
	// or four bytes through the DMA engine, matching the NE2000-PCI word/long
	// transfer modes the Linux 8390 driver uses for packet data. The other 8390
	// registers are byte-wide; widening them would also let a word read of offset
	// 0x1e touch the reset port at 0x1f.
	dataPort := d.port + ne2kDataPort
	d.host.RegisterIORead(dataPort, 16, func(ctx context.Context, _ uint16) uint32 {
		return uint32(d.readData(ctx)) | uint32(d.readData(ctx))<<8
	})
	d.host.RegisterIOWrite(dataPort, 16, func(ctx context.Context, _ uint16, value uint32) {
		d.writeData(ctx, byte(value))
		d.writeData(ctx, byte(value>>8))
	})
	d.host.RegisterIORead(dataPort, 32, func(ctx context.Context, _ uint16) uint32 {
		return uint32(d.readData(ctx)) |
			uint32(d.readData(ctx))<<8 |
			uint32(d.readData(ctx))<<16 |
			uint32(d.readData(ctx))<<24
	})
	d.host.RegisterIOWrite(dataPort, 32, func(ctx context.Context, _ uint16, value uint32) {
		d.writeData(ctx, byte(value))
		d.writeData(ctx, byte(value>>8))
		d.writeData(ctx, byte(value>>16))
		d.writeData(ctx, byte(value>>24))
	})

	d.updateIRQ(ctx)
	return nil
}

// SetOutbound installs the NE2K-to-stack boundary for guest transmits: the
// device calls sink with a copied Ethernet frame on the CPU goroutine when the
// guest transmits; pass nil to detach it. The reverse direction is QueueInbound
// (off-thread) plus DrainInbound (CPU goroutine).
func (d *ne2kDevice) SetOutbound(sink func(frame []byte)) {
	d.outbound = sink
}

// QueueInbound enqueues a guest-inbound Ethernet frame from any goroutine. The
// frame is copied and later delivered into the receive ring by DrainInbound on
// the CPU goroutine, so the network stack never touches device or wasm state
// concurrently with guest port I/O.
func (d *ne2kDevice) QueueInbound(frame []byte) {
	if d == nil {
		return
	}
	cp := append([]byte(nil), frame...)
	d.mtx.Lock()
	d.inbound = append(d.inbound, cp)
	d.mtx.Unlock()
}

// DrainInbound delivers queued inbound frames into the receive ring. It must run
// on the CPU goroutine, between main-loop ticks, so receive-ring writes and IRQ
// delivery stay single-threaded with guest port I/O.
func (d *ne2kDevice) DrainInbound(ctx context.Context) {
	if d == nil {
		return
	}
	d.mtx.Lock()
	frames := d.inbound
	d.inbound = nil
	d.mtx.Unlock()
	for _, frame := range frames {
		d.ReceiveFrame(ctx, frame)
	}
}

// ReceiveFrame is the stack-to-NE2K boundary. It applies the receive filter and
// places accepted Ethernet frames into the NE2000 receive ring. It runs on the
// CPU goroutine; off-thread callers use QueueInbound plus DrainInbound.
func (d *ne2kDevice) ReceiveFrame(ctx context.Context, frame []byte) {
	if d == nil || d.cr&ne2kCRStop != 0 || len(frame) < 6 {
		return
	}
	if !d.acceptsFrame(frame) {
		return
	}

	packetLen := max(len(frame), ne2kMinFrameLen)
	totalLen := packetLen + 4
	needed := byte(1 + (totalLen >> 8))
	if !d.rxAvailable(needed) {
		return
	}

	offset := uint16(d.curpg) << 8
	next := d.curpg + needed
	if next >= d.pstop {
		next += d.pstart - d.pstop
	}

	d.writeRing(offset, []byte{enrsrRXOK, next, byte(totalLen), byte(totalLen >> 8)})
	d.writeRing(offset+4, frame)
	if len(frame) < ne2kMinFrameLen {
		d.writeRingZeros(offset+4+uint16(len(frame)), ne2kMinFrameLen-len(frame))
	}
	// Advance only CURR. BOUNDARY is the driver's read pointer: lib8390 ei_receive
	// reads the next ring page as EN0_BOUNDARY+1, so the device must never write it
	// or the driver reads headers from the wrong (empty) page and drops every frame
	// as an rx_error. The driver advances BOUNDARY itself as it consumes frames.
	d.curpg = next
	d.interrupt(ctx, enisrRX)
}

// readPort services one register read, dispatching by register page.
func (d *ne2kDevice) readPort(ctx context.Context, offset uint16) byte {
	if offset == ne2kDataPort {
		return d.readData(ctx)
	}
	if offset == ne2kReset {
		d.reset(ctx)
		return 0
	}
	if offset == e8390Cmd {
		return d.cr
	}
	switch d.page() {
	case 0:
		return d.readPage0(offset)
	case 1:
		return d.readPage1(offset)
	case 2:
		return d.readPage2(offset)
	default:
		return 0
	}
}

// writePort services one register write, dispatching by register page.
func (d *ne2kDevice) writePort(ctx context.Context, offset uint16, value byte) {
	if offset == ne2kDataPort {
		d.writeData(ctx, value)
		return
	}
	if offset == ne2kReset {
		return
	}
	if offset == e8390Cmd {
		d.writeCommand(ctx, value)
		return
	}
	switch d.page() {
	case 0:
		d.writePage0(ctx, offset, value)
	case 1:
		d.writePage1(ctx, offset, value)
	}
}

// writeCommand handles the shared command-register write: remote-DMA
func (d *ne2kDevice) writeCommand(ctx context.Context, value byte) {
	d.cr = value
	if value&ne2kCRStop != 0 {
		return
	}
	if value&ne2kCRRDMA != 0 && d.rcnt == 0 {
		d.interrupt(ctx, enisrRDC)
	}
	if value&ne2kCRTXP == 0 {
		return
	}
	d.transmit(ctx)
	d.cr &^= ne2kCRTXP
}

// readPage0 reads a page-0 register (DMA and receive status registers).
func (d *ne2kDevice) readPage0(offset uint16) byte {
	switch offset {
	case en0Startpg:
		return d.pstart
	case en0Stoppg:
		return d.pstop
	case en0Boundary:
		return d.boundary
	case en0TSR:
		return d.tsr
	case en0ISR:
		return d.isr
	case en0RSARLO:
		return byte(d.rsar)
	case en0RSARHI:
		return byte(d.rsar >> 8)
	case en0RCNTLO:
		return byte(d.rcnt)
	case en0RCNTHI:
		return byte(d.rcnt >> 8)
	case en0RSR:
		return enrsrRXOK | 1<<3
	default:
		return 0
	}
}

// writePage0 writes a page-0 register (DMA pointers and transmit control).
func (d *ne2kDevice) writePage0(ctx context.Context, offset uint16, value byte) {
	switch offset {
	case en0Startpg:
		d.pstart = value
	case en0Stoppg:
		if int(value) > len(d.memory)>>8 {
			value = byte(len(d.memory) >> 8)
		}
		d.pstop = value
	case en0Boundary:
		d.boundary = value
	case en0TPSR:
		d.tpsr = value
	case en0TCNTLO:
		d.tcnt = d.tcnt&0xff00 | uint16(value)
	case en0TCNTHI:
		d.tcnt = d.tcnt&0x00ff | uint16(value)<<8
	case en0ISR:
		d.isr &^= value
		d.updateIRQ(ctx)
	case en0RSARLO:
		d.rsar = d.rsar&0xff00 | uint16(value)
	case en0RSARHI:
		d.rsar = d.rsar&0x00ff | uint16(value)<<8
	case en0RCNTLO:
		d.rcnt = d.rcnt&0xff00 | uint16(value)
	case en0RCNTHI:
		d.rcnt = d.rcnt&0x00ff | uint16(value)<<8
	case en0RXCR:
		d.rxcr = value
	case en0TXCR:
		d.txcr = value
	case en0DCFG:
		d.dcfg = value
	case en0IMR:
		d.imr = value
		d.updateIRQ(ctx)
	}
}

// readPage1 reads a page-1 register (current receive page and MAC).
func (d *ne2kDevice) readPage1(offset uint16) byte {
	switch {
	case offset >= en0Startpg && offset < en0Startpg+6:
		return d.mac[offset-en0Startpg]
	case offset == en0ISR:
		return d.curpg
	case offset >= en0RSARLO && offset <= en0IMR:
		return d.mar[offset-en0RSARLO]
	default:
		return 0
	}
}

// writePage1 writes a page-1 register (current receive page and MAC).
func (d *ne2kDevice) writePage1(_ context.Context, offset uint16, value byte) {
	switch {
	case offset >= en0Startpg && offset < en0Startpg+6:
		d.mac[offset-en0Startpg] = value
	case offset == en0ISR:
		d.curpg = value
	case offset >= en0RSARLO && offset <= en0IMR:
		d.mar[offset-en0RSARLO] = value
	}
}

// readPage2 reads a page-2 mirror of the ring configuration registers.
func (d *ne2kDevice) readPage2(offset uint16) byte {
	switch offset {
	case en0Startpg:
		return d.pstart
	case en0Stoppg:
		return d.pstop
	default:
		return 0
	}
}

// readData consumes one byte through the remote-DMA read pointer.
func (d *ne2kDevice) readData(ctx context.Context) byte {
	var value byte
	if int(d.rsar) < len(d.memory) {
		value = d.memory[d.rsar]
	}
	d.advanceDMA(ctx)
	return value
}

// writeData stores one byte through the remote-DMA write pointer.
func (d *ne2kDevice) writeData(ctx context.Context, value byte) {
	if int(d.rsar) < len(d.memory) {
		d.memory[d.rsar] = value
	}
	d.advanceDMA(ctx)
}

// advanceDMA walks the DMA address around the receive ring and raises the
func (d *ne2kDevice) advanceDMA(ctx context.Context) {
	d.rsar++
	if d.rsar >= uint16(d.pstop)<<8 {
		d.rsar += uint16(d.pstart-d.pstop) << 8
	}
	if d.rcnt != 0 {
		d.rcnt--
		if d.rcnt == 0 {
			d.interrupt(ctx, enisrRDC)
		}
	}
}

// transmit copies the transmit-buffer frame and hands it to the outbound
func (d *ne2kDevice) transmit(ctx context.Context) {
	start := uint16(d.tpsr) << 8
	end := min(uint32(start)+uint32(d.tcnt), uint32(len(d.memory)))
	frame := append([]byte(nil), d.memory[start:end]...)
	if d.outbound != nil {
		d.outbound(frame)
	}
	d.interrupt(ctx, enisrTX)
}

// acceptsFrame applies the receive filter: promiscuous, broadcast, and
func (d *ne2kDevice) acceptsFrame(frame []byte) bool {
	if d.rxcr&enrxcrPRO != 0 {
		return true
	}
	if d.rxcr&enrxcrAB != 0 && isBroadcastFrame(frame) {
		return true
	}
	if d.rxcr&enrxcrAM != 0 && frame[0]&1 != 0 {
		return false
	}
	for i, value := range d.mac {
		if frame[i] != value {
			return false
		}
	}
	return true
}

// rxAvailable reports whether needed ring pages are free between BOUNDARY
func (d *ne2kDevice) rxAvailable(needed byte) bool {
	if d.boundary == 0 {
		return true
	}
	if d.boundary == d.curpg {
		return true
	}
	var available byte
	if d.boundary > d.curpg {
		available = d.boundary - d.curpg
	} else {
		available = d.pstop - d.curpg + d.boundary - d.pstart
	}
	return available >= needed
}

// writeRing copies bytes into ring memory, wrapping pages.
func (d *ne2kDevice) writeRing(offset uint16, data []byte) {
	for _, value := range data {
		d.writeRingByte(offset, value)
		offset++
	}
}

// writeRingZeros zeroes count ring bytes starting at offset.
func (d *ne2kDevice) writeRingZeros(offset uint16, count int) {
	for range count {
		d.writeRingByte(offset, 0)
		offset++
	}
}

// writeRingByte stores one byte into ring memory with page wrap-around.
func (d *ne2kDevice) writeRingByte(offset uint16, value byte) {
	start := uint16(d.pstart) << 8
	stop := uint16(d.pstop) << 8
	if offset >= stop {
		offset = start + (offset - stop)
	}
	if int(offset) < len(d.memory) {
		d.memory[offset] = value
	}
}

// interrupt marks ISR bits pending for the given event mask.
func (d *ne2kDevice) interrupt(ctx context.Context, mask byte) {
	d.isr |= mask
	d.updateIRQ(ctx)
}

// updateIRQ reevaluates the interrupt line against ISR & IMR.
func (d *ne2kDevice) updateIRQ(ctx context.Context) {
	asserted := d.imr&d.isr != 0
	d.irqAsserted = asserted
	if d.host == nil {
		return
	}
	if asserted {
		_ = d.host.raiseIRQ(ctx, d.assignedIRQ())
		return
	}
	_ = d.host.lowerIRQ(ctx, d.assignedIRQ())
}

// assignedIRQ returns the ISA interrupt line the adapter signals.
func (d *ne2kDevice) assignedIRQ() uint32 {
	if d.host != nil && d.host.pci != nil {
		space := d.host.pci.spaces[d.bdf]
		if len(space) > 0x3c {
			if line := space[0x3c]; line != 0 && line != 0xff {
				return uint32(line)
			}
		}
	}
	return ne2kIRQ
}

// reset performs a software reset and latches the reset ISR bit.
func (d *ne2kDevice) reset(ctx context.Context) {
	d.resetState()
	d.isr = enisrReset
	d.updateIRQ(ctx)
}

// resetState restores power-on register defaults and clears ring memory.
func (d *ne2kDevice) resetState() {
	d.isr = 0
	d.imr = 0
	d.cr = ne2kCRStop
	d.dcfg = 0
	d.rcnt = 0
	d.tcnt = 0
	d.tpsr = 0
	d.rxcr = 0
	d.txcr = 0
	d.tsr = 1
	d.rsar = 0
	d.pstart = ne2kStartPage
	d.pstop = ne2kStopPage
	d.curpg = ne2kStartRXPage
	d.boundary = ne2kStartRXPage
	d.irqAsserted = false
	clear(d.memory)
	d.writePROM()
}

// writePROM seeds the PROM area with the MAC in word-duplicated form.
func (d *ne2kDevice) writePROM() {
	for i, value := range d.mac {
		d.memory[i<<1] = value
		d.memory[i<<1|1] = value
	}
	d.memory[14<<1] = 0x57
	d.memory[14<<1|1] = 0x57
	d.memory[15<<1] = 0x57
	d.memory[15<<1|1] = 0x57
}

// page returns the currently selected register page from the command
func (d *ne2kDevice) page() byte {
	return d.cr >> 6 & 3
}

// ne2kPort returns the IO base for adapter index id.
func ne2kPort(id int) uint16 {
	return uint16(ne2kPCIPortBase + ne2kPCIPortStep*id)
}

// ne2kPCIID returns the PCI device number for adapter index id.
func ne2kPCIID(id int) uint16 {
	if id == 0 {
		return 0x05 << 3
	}
	return uint16(0x07+id) << 3
}

// newNE2KPCISpace builds the adapter's config space exposing its IO BAR.
func newNE2KPCISpace(port uint16) []byte {
	space := make([]byte, 256)
	copy(space, []byte{
		0xec, 0x10, 0x29, 0x80,
		0x03, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x00,
	})
	binary.LittleEndian.PutUint32(space[0x10:], uint32(port)|1)
	binary.LittleEndian.PutUint16(space[0x2c:], 0x1af4)
	binary.LittleEndian.PutUint16(space[0x2e:], 0x1100)
	binary.LittleEndian.PutUint32(space[0x30:], 0xfeb80000)
	space[0x3c] = ne2kIRQ
	space[0x3d] = 1
	return space
}

// isBroadcastFrame reports whether the destination MAC is all ones.
func isBroadcastFrame(frame []byte) bool {
	for _, value := range frame[:6] {
		if value != 0xff {
			return false
		}
	}
	return true
}
