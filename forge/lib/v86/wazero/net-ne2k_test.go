package v86_wazero

import (
	"bytes"
	"context"
	"testing"
)

func TestNE2KResetAndInitialRegisters(t *testing.T) {
	// Create an NE2K device with its initial register state.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)

	// Reset the NE2K device through its reset port.
	_ = host.readIO(ctx, uint32(dev.port+ne2kReset), 8)

	// Verify the NE2K ring bounds and reset registers.
	if dev.pstart != ne2kStartPage {
		t.Fatalf("pstart = %#x, want %#x", dev.pstart, ne2kStartPage)
	}
	if dev.pstop != ne2kStopPage {
		t.Fatalf("pstop = %#x, want %#x", dev.pstop, ne2kStopPage)
	}
	if dev.curpg != ne2kStartRXPage {
		t.Fatalf("curpg = %#x, want %#x", dev.curpg, ne2kStartRXPage)
	}
	if dev.boundary != ne2kStartRXPage {
		t.Fatalf("boundary = %#x, want %#x", dev.boundary, ne2kStartRXPage)
	}
	if dev.isr&enisrReset == 0 {
		t.Fatalf("reset ISR = %#x, missing reset bit", dev.isr)
	}
	if got := host.readIO(ctx, uint32(dev.port+en0Startpg), 8); got != ne2kStartPage {
		t.Fatalf("EN0_STARTPG = %#x, want %#x", got, ne2kStartPage)
	}
	if got := host.readIO(ctx, uint32(dev.port+en0Stoppg), 8); got != ne2kStopPage {
		t.Fatalf("EN0_STOPPG = %#x, want %#x", got, ne2kStopPage)
	}
}

func TestNE2KPage1MACProgramReadback(t *testing.T) {
	// Create an NE2K device and the station address to program.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	want := []byte{0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}

	// Program the NE2K station address through page-one registers.
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), 0x41, 8)
	for i, value := range want {
		host.writeIO(ctx, uint32(dev.port+en0Startpg+uint16(i)), uint32(value), 8)
	}

	// Verify the page-one station address readback.
	for i, wantValue := range want {
		got := host.readIO(ctx, uint32(dev.port+en0Startpg+uint16(i)), 8)
		if got != uint32(wantValue) {
			t.Fatalf("page1 PAR%d = %#x, want %#x", i, got, wantValue)
		}
	}
}

func TestNE2KRemoteDMARoundTrip(t *testing.T) {
	// Create an NE2K device and a byte DMA payload.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	want := []byte{0xde, 0xad, 0xbe, 0xef, 0x42}
	addr := uint16(ne2kStartPage) << 8

	// Write the payload into the NE2K ring through byte DMA.
	programNE2KDMA(ctx, host, dev, addr, len(want))
	for _, value := range want {
		host.writeIO(ctx, uint32(dev.port+ne2kDataPort), uint32(value), 8)
	}

	// Read the payload back through byte DMA.
	programNE2KDMA(ctx, host, dev, addr, len(want))
	got := make([]byte, len(want))
	for i := range got {
		got[i] = byte(host.readIO(ctx, uint32(dev.port+ne2kDataPort), 8))
	}

	// Verify the byte DMA payload survives the round trip.
	if !bytes.Equal(got, want) {
		t.Fatalf("remote DMA round trip = %x, want %x", got, want)
	}
}

func TestNE2KRemoteDMAWordRoundTrip(t *testing.T) {
	// Create an NE2K device for word DMA transfers.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)

	// Two 16-bit words; each data-port access must move two ring bytes through the
	// DMA engine, the word transfer mode the Linux NE2000-PCI driver uses.
	words := []uint16{0xbeef, 0x4221}
	want := []byte{0xef, 0xbe, 0x21, 0x42}
	addr := uint16(ne2kStartPage) << 8

	// Write the words into the NE2K ring through word DMA.
	programNE2KDMA(ctx, host, dev, addr, len(want))
	for _, word := range words {
		host.writeIO(ctx, uint32(dev.port+ne2kDataPort), uint32(word), 16)
	}

	// Verify word DMA stores little-endian bytes in the ring.
	if got := dev.memory[addr : addr+uint16(len(want))]; !bytes.Equal(got, want) {
		t.Fatalf("ring after word DMA write = %x, want %x", got, want)
	}

	// Verify word DMA reads the original words from the ring.
	programNE2KDMA(ctx, host, dev, addr, len(want))
	for i, word := range words {
		got := uint16(host.readIO(ctx, uint32(dev.port+ne2kDataPort), 16))
		if got != word {
			t.Fatalf("word DMA read %d = %#x, want %#x", i, got, word)
		}
	}
}

func TestNE2KTransmitOutboundAndIRQGating(t *testing.T) {
	// Create an NE2K device and an outbound Ethernet frame.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	frame := []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x02, 0x22, 0x15, 0x00, 0x00, 0x01,
		0x08, 0x00, 0x45, 0x00,
	}

	// Capture the NE2K outbound callback for the prepared ring frame.
	copy(dev.memory[ne2kStartPage<<8:], frame)
	var sent []byte
	dev.SetOutbound(func(frame []byte) {
		sent = append([]byte(nil), frame...)
	})

	// Transmit the ring frame with NE2K transmit interrupts enabled.
	host.writeIO(ctx, uint32(dev.port+en0TPSR), ne2kStartPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0TCNTLO), uint32(len(frame)), 8)
	host.writeIO(ctx, uint32(dev.port+en0TCNTHI), uint32(len(frame)>>8), 8)
	host.writeIO(ctx, uint32(dev.port+en0IMR), enisrTX, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), ne2kCRTXP, 8)

	// Verify the outbound frame and transmit interrupt state.
	if !bytes.Equal(sent, frame) {
		t.Fatalf("outbound frame = %x, want %x", sent, frame)
	}
	if dev.isr&enisrTX == 0 {
		t.Fatalf("ISR = %#x, missing TX bit", dev.isr)
	}
	if !dev.irqAsserted {
		t.Fatal("irqAsserted false with TX interrupt enabled")
	}

	// Mask NE2K transmit interrupts and verify the IRQ clears.
	host.writeIO(ctx, uint32(dev.port+en0IMR), 0, 8)
	if dev.irqAsserted {
		t.Fatal("irqAsserted true with TX interrupt masked")
	}
}

func TestNE2KReceiveFrameRingAndIRQGating(t *testing.T) {
	// Create an NE2K device and an inbound Ethernet frame.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	frame := append([]byte{
		0x02, 0x22, 0x15, 0x00, 0x00, 0x01,
		0x02, 0x22, 0x15, 0x00, 0x00, 0x02,
		0x08, 0x00,
	}, bytes.Repeat([]byte{0xab}, 18)...)
	offset := uint16(ne2kStartRXPage) << 8

	// Receive the Ethernet frame with NE2K receive interrupts enabled.
	host.writeIO(ctx, uint32(dev.port+en0IMR), enisrRX, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), 0, 8)
	dev.ReceiveFrame(ctx, frame)

	// Verify the NE2K receive header, frame bytes, and next ring page.
	wantNext := byte(ne2kStartRXPage + 1)
	if got := dev.memory[offset : offset+4]; !bytes.Equal(got, []byte{enrsrRXOK, wantNext, 64, 0}) {
		t.Fatalf("RX header = %x, want %x", got, []byte{enrsrRXOK, wantNext, 64, 0})
	}
	if got := dev.memory[offset+4 : offset+4+uint16(len(frame))]; !bytes.Equal(got, frame) {
		t.Fatalf("RX frame = %x, want %x", got, frame)
	}
	if dev.curpg != wantNext {
		t.Fatalf("curpg = %#x, want %#x", dev.curpg, wantNext)
	}

	// BOUNDARY is the driver's read pointer; the device must not advance it on
	// receive (lib8390 reads the next ring page as EN0_BOUNDARY+1).
	if dev.boundary != ne2kStartRXPage {
		t.Fatalf("boundary = %#x, want unchanged %#x", dev.boundary, ne2kStartRXPage)
	}

	// Verify the NE2K receive interrupt reaches the IRQ line.
	if dev.isr&enisrRX == 0 {
		t.Fatalf("ISR = %#x, missing RX bit", dev.isr)
	}
	if !dev.irqAsserted {
		t.Fatal("irqAsserted false with RX interrupt enabled")
	}

	// Mask NE2K receive interrupts and verify the IRQ clears.
	host.writeIO(ctx, uint32(dev.port+en0IMR), 0, 8)
	if dev.irqAsserted {
		t.Fatal("irqAsserted true with RX interrupt masked")
	}
}

func TestNE2KQueueInboundDefersDeliveryToDrain(t *testing.T) {
	// Create an NE2K device and a frame to queue twice.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	frame := append([]byte{
		0x02, 0x22, 0x15, 0x00, 0x00, 0x01,
		0x02, 0x22, 0x15, 0x00, 0x00, 0x02,
		0x08, 0x00,
	}, bytes.Repeat([]byte{0xcd}, 18)...)

	// Enable NE2K reception and receive interrupts.
	host.writeIO(ctx, uint32(dev.port+en0IMR), enisrRX, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), 0, 8)

	// Queue two inbound frames for deferred NE2K delivery.
	dev.QueueInbound(frame)
	dev.QueueInbound(frame)

	// Verify queued frames leave the NE2K ring and interrupt state unchanged.
	if dev.curpg != ne2kStartRXPage {
		t.Fatalf("curpg advanced before drain: %#x", dev.curpg)
	}
	if dev.isr&enisrRX != 0 {
		t.Fatalf("RX ISR set before drain: %#x", dev.isr)
	}

	// Drain both queued frames into the NE2K receive ring.
	dev.DrainInbound(ctx)

	// Verify both drained frames advance the ring and assert the receive IRQ.
	if got, want := dev.curpg, byte(ne2kStartRXPage+2); got != want {
		t.Fatalf("curpg after drain = %#x, want %#x (two frames delivered)", got, want)
	}
	if dev.isr&enisrRX == 0 {
		t.Fatalf("RX ISR = %#x, missing RX bit after drain", dev.isr)
	}
	if !dev.irqAsserted {
		t.Fatal("irqAsserted false after draining inbound frames with RX enabled")
	}
}

func TestNE2KReceiveFilterDropsOtherUnicast(t *testing.T) {
	// Create an NE2K device and a frame addressed to another station.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)
	frame := []byte{
		0x02, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x02, 0x22, 0x15, 0x00, 0x00, 0x02,
		0x08, 0x00, 0x45, 0x00,
	}
	offset := uint16(ne2kStartRXPage) << 8

	// Deliver the foreign unicast frame to the NE2K receive filter.
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), 0, 8)
	dev.ReceiveFrame(ctx, frame)

	// Verify the rejected frame leaves the NE2K ring and interrupts unchanged.
	if dev.curpg != ne2kStartRXPage {
		t.Fatalf("curpg = %#x, want unchanged %#x", dev.curpg, ne2kStartRXPage)
	}
	if dev.isr&enisrRX != 0 {
		t.Fatalf("ISR = %#x, expected no RX bit", dev.isr)
	}
	if got := dev.memory[offset : offset+4]; !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("RX header after dropped frame = %x, want zeroes", got)
	}
}

// TestNE2KDriverInitReceiveReadback replicates the Linux ne2k-pci driver init and
// receive path against the real page layout (TPSR=0x40, STARTPG=0x46, STOPPG=0x80,
// BNRY=0x7f, CURR=0x46, RXCR=accept-broadcast, IMR=RX) and reads the ring header
// back through remote DMA exactly as ne2k_pci_get_8390_hdr does. It injects a short
// (ARP-reply-sized) frame addressed to the byte-doubled PROM MAC the driver
// programs into PAR0-5, asserting the header the driver reads (status RXOK, next
// page, count) and the body match. This is the device-level proof that the live
// guest's apt-over-net DNS/ARP round trip should not be rejected as rx_errors.
func TestNE2KDriverInitReceiveReadback(t *testing.T) {
	// Create an NE2K device for the Linux driver receive sequence.
	ctx := context.Background()
	host, dev := newNE2KTestDevice(t)

	// Define the Linux driver ring layout and command values.
	const (
		txStartPage = 0x40
		rxStartPage = 0x46
		rxStopPage  = 0x80
		crStart     = 0x22 // E8390_NODMA|E8390_PAGE0|E8390_START
		crPage1Stop = 0x61 // E8390_NODMA|E8390_PAGE1|E8390_STOP
		crRemoteRd  = 0x0a // E8390_RREAD|E8390_START
		rxConfig    = enrxcrAB
	)

	// The driver reads the station address from the byte-doubled PROM and writes
	// that doubled value into PAR0-5; the usermode stack therefore learns and
	// addresses the doubled MAC, so the RX filter must match it.
	driverMAC := [6]byte{0x02, 0x02, 0x22, 0x22, 0x15, 0x15}
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), crPage1Stop, 8)
	for i, value := range driverMAC {
		host.writeIO(ctx, uint32(dev.port+en0Startpg+uint16(i)), uint32(value), 8)
	}
	host.writeIO(ctx, uint32(dev.port+en0ISR), rxStartPage, 8) // EN1_CURPAG

	// Initialize the NE2K page-zero ring and enable reception.
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), 0x21, 8) // page0, stop
	host.writeIO(ctx, uint32(dev.port+en0TPSR), txStartPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0Startpg), rxStartPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0Boundary), rxStopPage-1, 8)
	host.writeIO(ctx, uint32(dev.port+en0Stoppg), rxStopPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0RXCR), rxConfig, 8)
	host.writeIO(ctx, uint32(dev.port+en0IMR), enisrRX, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), crStart, 8)

	// Verify the driver initialization selects the first receive page.
	if dev.curpg != rxStartPage {
		t.Fatalf("after driver init curpg = %#x, want %#x", dev.curpg, rxStartPage)
	}

	// 42-byte ARP-reply-sized frame to the doubled MAC; gets padded to 60 in ring.
	frame := append([]byte{
		0x02, 0x02, 0x22, 0x22, 0x15, 0x15, // dst: guest (doubled)
		0x02, 0x22, 0x15, 0x00, 0x00, 0x02, // src: gateway
		0x08, 0x06, // ethertype ARP
	}, bytes.Repeat([]byte{0xa5}, 28)...)
	dev.ReceiveFrame(ctx, frame)

	// Verify the received ARP frame asserts the NE2K receive IRQ.
	if !dev.irqAsserted {
		t.Fatal("irqAsserted false after RX with RX interrupt enabled")
	}
	if dev.isr&enisrRX == 0 {
		t.Fatalf("ISR = %#x, missing RX bit", dev.isr)
	}

	// Read the 4-byte ring header via remote DMA exactly like ne2k_pci_get_8390_hdr.
	host.writeIO(ctx, uint32(dev.port+en0RSARLO), 0, 8)
	host.writeIO(ctx, uint32(dev.port+en0RSARHI), rxStartPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTLO), 4, 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTHI), 0, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), crRemoteRd, 8)
	hdr := make([]byte, 4)
	for i := range hdr {
		hdr[i] = byte(host.readIO(ctx, uint32(dev.port+ne2kDataPort), 8))
	}

	// Verify the driver reads a padded receive header through DMA.
	wantHdr := []byte{enrsrRXOK, rxStartPage + 1, 64, 0}
	if !bytes.Equal(hdr, wantHdr) {
		t.Fatalf("driver-read RX header = %x, want %x", hdr, wantHdr)
	}

	// Verify the driver accepts the padded packet length.
	pktLen := (int(hdr[2]) | int(hdr[3])<<8) - 4
	if pktLen < ne2kMinFrameLen {
		t.Fatalf("driver pkt_len = %d, would be rejected as rx_error (< %d)", pktLen, ne2kMinFrameLen)
	}

	// Read the body via remote DMA from rxStartPage<<8 + 4, like ne2k_pci_block_input.
	host.writeIO(ctx, uint32(dev.port+en0RSARLO), 4, 8)
	host.writeIO(ctx, uint32(dev.port+en0RSARHI), rxStartPage, 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTLO), uint32(pktLen), 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTHI), 0, 8)
	host.writeIO(ctx, uint32(dev.port+e8390Cmd), crRemoteRd, 8)
	body := make([]byte, pktLen)
	for i := range body {
		body[i] = byte(host.readIO(ctx, uint32(dev.port+ne2kDataPort), 8))
	}

	// Verify the driver reads the original Ethernet frame through DMA.
	if !bytes.Equal(body[:len(frame)], frame) {
		t.Fatalf("driver-read RX body = %x, want prefix %x", body[:len(frame)], frame)
	}
}

func newNE2KTestDevice(t *testing.T) (*HostRuntime, *ne2kDevice) {
	// Create a HostRuntime with I/O tracking and PCI register state.
	t.Helper()
	ctx := context.Background()
	host := &HostRuntime{
		ioPorts:      newIOPorts(),
		ioReads:      make(map[uint16]uint64),
		ioWrites:     make(map[uint16]uint64),
		ioLastReads:  make(map[uint16]uint32),
		ioLastWrites: make(map[uint16]uint32),
	}
	host.pci = &pciDevice{
		host:      host,
		spaces:    make(map[uint16][]byte),
		barSizes:  make(map[uint16]map[int]uint32),
		barIO:     make(map[uint16]map[int]bool),
		barProbes: make(map[uint16]map[int]bool),
	}

	// Register the NE2K device on the test host.
	dev := newNE2KDevice(host, 0, [6]byte{0x02, 0x22, 0x15, 0x00, 0x00, 0x01})
	if err := dev.register(ctx); err != nil {
		t.Fatalf("register NE2K: %v", err)
	}

	return host, dev
}

func programNE2KDMA(ctx context.Context, host *HostRuntime, dev *ne2kDevice, addr uint16, count int) {
	host.writeIO(ctx, uint32(dev.port+en0RSARLO), uint32(byte(addr)), 8)
	host.writeIO(ctx, uint32(dev.port+en0RSARHI), uint32(byte(addr>>8)), 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTLO), uint32(byte(count)), 8)
	host.writeIO(ctx, uint32(dev.port+en0RCNTHI), uint32(byte(count>>8)), 8)
}
