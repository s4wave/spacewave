package usernet

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"
)

type testSink struct {
	mtx    sync.Mutex
	frames [][]byte
	ch     chan []byte
}

func newTestSink() *testSink {
	return &testSink{
		ch: make(chan []byte, 32),
	}
}

func (s *testSink) inbound(frame []byte) {
	cp := bytes.Clone(frame)
	s.mtx.Lock()
	s.frames = append(s.frames, cp)
	s.mtx.Unlock()
	s.ch <- cp
}

func (s *testSink) next(t *testing.T) []byte {
	t.Helper()
	select {
	case frame := <-s.ch:
		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for inbound frame")
		return nil
	}
}

func testConfig() Config {
	return Config{
		GuestMAC:  [6]byte{0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		GuestIP:   netip.MustParseAddr("10.0.2.15"),
		GatewayIP: netip.MustParseAddr("10.0.2.2"),
		Netmask:   netip.MustParseAddr("255.255.255.0"),
		DNSServer: netip.MustParseAddr("10.0.2.3"),
	}
}

func TestARPReply(t *testing.T) {
	cfg := testConfig()
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	frame := buildARPRequest(cfg.GuestMAC, netip.MustParseAddr("10.0.2.15"), cfg.GatewayIP)
	stack.HandleOutbound(frame)

	got := sink.next(t)
	want := buildARPReply(cfg.GuestMAC, cfg.GatewayIP, cfg.GuestIP)
	if !bytes.Equal(got, want) {
		t.Fatalf("arp reply = %x, want %x", got, want)
	}
}

func TestICMPEchoReply(t *testing.T) {
	cfg := testConfig()
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	data := []byte{0x12, 0x34, 0x00, 0x01, 'p', 'i', 'n', 'g'}
	icmp := buildICMP(8, 0, data)
	ip := buildIPv4(ipProtoICMP, cfg.GuestIP, cfg.GatewayIP, icmp)
	stack.HandleOutbound(buildEth(routerMAC, cfg.GuestMAC, ethTypeIPv4, ip))

	got := sink.next(t)
	wantICMP := buildICMP(0, 0, data)
	wantIP := buildIPv4(ipProtoICMP, cfg.GatewayIP, cfg.GuestIP, wantICMP)
	want := buildEth(cfg.GuestMAC, routerMAC, ethTypeIPv4, wantIP)
	if !bytes.Equal(got, want) {
		t.Fatalf("icmp reply = %x, want %x", got, want)
	}
}

func TestDHCPDiscoverRequestLease(t *testing.T) {
	cfg := testConfig()
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	stack.HandleOutbound(buildDHCPFrame(cfg, 0x12345678, 1))
	offer := parseDHCPReply(t, sink.next(t))
	if offer.op != 2 {
		t.Fatalf("offer op = %d, want 2", offer.op)
	}
	if offer.yiaddr != ipToLong(cfg.GuestIP) {
		t.Fatalf("offer yiaddr = %#x, want %#x", offer.yiaddr, ipToLong(cfg.GuestIP))
	}
	if got := dhcpOptionByte(offer, 53); got != 2 {
		t.Fatalf("offer message type = %d, want 2", got)
	}
	if got := dhcpOptionAddr(t, offer, 3); got != cfg.GatewayIP {
		t.Fatalf("offer router = %s, want %s", got, cfg.GatewayIP)
	}
	if got := dhcpOptionAddr(t, offer, 6); got != cfg.DNSServer {
		t.Fatalf("offer dns = %s, want %s", got, cfg.DNSServer)
	}

	stack.HandleOutbound(buildDHCPFrame(cfg, 0x12345678, 3))
	ack := parseDHCPReply(t, sink.next(t))
	if got := dhcpOptionByte(ack, 53); got != 5 {
		t.Fatalf("ack message type = %d, want 5", got)
	}
	if got := dhcpOption(ack, 51); !bytes.Equal(got, []byte{8, 0, 0, 0}) {
		t.Fatalf("ack lease option = %x, want 08000000", got)
	}
}

func TestDNSProxy(t *testing.T) {
	cfg := testConfig()
	dnsConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		err := dnsConn.Close()
		if err != nil {
			t.Fatal(err.Error())
		}
	})
	go serveTestDNS(dnsConn)

	dialer := &net.Dialer{}
	cfg.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", dnsConn.LocalAddr().String())
		},
	}
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	query := buildDNSQuery(0x4242, "example.test", 1)
	udp := buildUDP(cfg.GuestIP, cfg.DNSServer, 53000, 53, query)
	ip := buildIPv4(ipProtoUDP, cfg.GuestIP, cfg.DNSServer, udp)
	stack.HandleOutbound(buildEth(routerMAC, cfg.GuestMAC, ethTypeIPv4, ip))

	got := sink.next(t)
	packet, err := parseEth(got)
	if err != nil {
		t.Fatal(err.Error())
	}
	resp, err := parseDNS(packet.ipv4.udp.data)
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.id != 0x4242 {
		t.Fatalf("dns id = %#x, want 0x4242", resp.id)
	}
	if binary.BigEndian.Uint16(packet.ipv4.udp.data[6:8]) == 0 {
		t.Fatal("dns answer count is zero")
	}
	if !bytes.Contains(packet.ipv4.udp.data, []byte{203, 0, 113, 9}) {
		t.Fatalf("dns response missing expected A record: %x", packet.ipv4.udp.data)
	}
}

func TestUDPEchoThroughHost(t *testing.T) {
	cfg := testConfig()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		err := pc.Close()
		if err != nil {
			t.Fatal(err.Error())
		}
	})
	go serveUDPEcho(pc)

	destIP, destPort := packetAddr(t, pc.LocalAddr())
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	payload := []byte("udp echo")
	udp := buildUDP(cfg.GuestIP, destIP, 40000, destPort, payload)
	ip := buildIPv4(ipProtoUDP, cfg.GuestIP, destIP, udp)
	stack.HandleOutbound(buildEth(routerMAC, cfg.GuestMAC, ethTypeIPv4, ip))

	got := sink.next(t)
	packet, err := parseEth(got)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(packet.ipv4.udp.data, payload) {
		t.Fatalf("udp payload = %q, want %q", packet.ipv4.udp.data, payload)
	}
}

func TestTCPConnectEchoCloseThroughHost(t *testing.T) {
	cfg := testConfig()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		err := ln.Close()
		if err != nil {
			t.Fatal(err.Error())
		}
	})
	go serveTCPEcho(ln)

	destIP, destPort := packetAddr(t, ln.Addr())
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)
	t.Cleanup(func() {
		err := stack.Close()
		if err != nil && err != ErrStackClosed {
			t.Fatal(err.Error())
		}
	})

	guestPort := uint16(41000)
	guestSeq := uint32(1000)
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq, 0, tcpFlagSYN, nil))
	synAck := parseTCPInbound(t, sink.next(t))
	if synAck.flags&(tcpFlagSYN|tcpFlagACK) != tcpFlagSYN|tcpFlagACK {
		t.Fatalf("syn ack flags = %#x", synAck.flags)
	}
	if synAck.ack != guestSeq+1 {
		t.Fatalf("syn ack ack = %d, want %d", synAck.ack, guestSeq+1)
	}

	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq+1, synAck.seq+1, tcpFlagACK, nil))
	payload := []byte("tcp echo")
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq+1, synAck.seq+1, tcpFlagACK|tcpFlagPSH, payload))
	ack := parseTCPInbound(t, sink.next(t))
	if ack.flags&tcpFlagACK == 0 || len(ack.data) != 0 {
		t.Fatalf("data ack flags/data = %#x/%x", ack.flags, ack.data)
	}
	echo := parseTCPInbound(t, sink.next(t))
	if !bytes.Equal(echo.data, payload) {
		t.Fatalf("tcp echo payload = %q, want %q", echo.data, payload)
	}

	guestSeq += 1 + uint32(len(payload))
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq, echo.seq+uint32(len(echo.data)), tcpFlagACK, nil))
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq, echo.seq+uint32(len(echo.data)), tcpFlagACK|tcpFlagFIN, nil))
	finAck := parseTCPInbound(t, sink.next(t))
	if finAck.flags&tcpFlagACK == 0 {
		t.Fatalf("fin ack flags = %#x", finAck.flags)
	}
	hostFin := parseTCPInbound(t, sink.next(t))
	if hostFin.flags&tcpFlagFIN == 0 {
		t.Fatalf("host fin flags = %#x", hostFin.flags)
	}
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, guestPort, destPort, guestSeq+1, hostFin.seq+1, tcpFlagACK, nil))
}

// TestNilLoggerCloseErrorNoPanic proves a nil le passed to New does not panic on
// the host-socket error paths. This logrus fork does not nil-guard Entry
// methods, so le.WithError(...).Debug(...) on a nil entry would dereference nil;
// New must substitute a usable entry.
func TestNilLoggerCloseErrorNoPanic(t *testing.T) {
	cfg := testConfig()
	cfg.Dialer = dialerFunc(func(_ context.Context, _ string, _ string) (net.Conn, error) {
		return &errCloseConn{done: make(chan struct{})}, nil
	})
	sink := newTestSink()
	stack := New(cfg, sink.inbound, nil)

	destIP := netip.MustParseAddr("203.0.113.50")
	stack.HandleOutbound(buildTCPFrame(cfg, destIP, 41100, 80, 2000, 0, tcpFlagSYN, nil))
	parseTCPInbound(t, sink.next(t))

	// closeHost on the registered conn returns an error, exercising the
	// le.WithError(...).Debug(...) path that would panic with a nil entry.
	if err := stack.Close(); err == nil {
		t.Fatal("expected close error from erroring host conn")
	}
}

type dialerFunc func(ctx context.Context, network string, address string) (net.Conn, error)

func (f dialerFunc) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

type errCloseConn struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

func (c *errCloseConn) Read(b []byte) (int, error) {
	<-c.done
	return 0, io.ErrClosedPipe
}

func (c *errCloseConn) Write(b []byte) (int, error) { return len(b), nil }

func (c *errCloseConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return io.ErrClosedPipe
}

func buildARPRequest(src [6]byte, srcIP netip.Addr, targetIP netip.Addr) []byte {
	payload := make([]byte, 28)
	binary.BigEndian.PutUint16(payload[0:2], 1)
	binary.BigEndian.PutUint16(payload[2:4], ethTypeIPv4)
	payload[4] = 6
	payload[5] = 4
	binary.BigEndian.PutUint16(payload[6:8], 1)
	copy(payload[8:14], src[:])
	copy(payload[14:18], srcIP.AsSlice())
	copy(payload[24:28], targetIP.AsSlice())
	return buildEth(broadcastMAC, src, ethTypeARP, payload)
}

func buildDHCPFrame(cfg Config, xid uint32, msgType byte) []byte {
	req := &dhcpPacket{
		op:    1,
		htype: 1,
		hlen:  6,
		xid:   xid,
		options: [][]byte{
			{53, 1, msgType},
			{255, 0},
		},
	}
	copy(req.chaddr[:], cfg.GuestMAC[:])
	udp := buildUDP(netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("255.255.255.255"), 68, 67, buildDHCP(req))
	ip := buildIPv4(ipProtoUDP, netip.MustParseAddr("0.0.0.0"), netip.MustParseAddr("255.255.255.255"), udp)
	return buildEth(broadcastMAC, cfg.GuestMAC, ethTypeIPv4, ip)
}

func parseDHCPReply(t *testing.T, frame []byte) *dhcpPacket {
	t.Helper()
	packet, err := parseEth(frame)
	if err != nil {
		t.Fatal(err.Error())
	}
	dhcp, err := parseDHCP(packet.ipv4.udp.data)
	if err != nil {
		t.Fatal(err.Error())
	}
	return dhcp
}

func dhcpOption(p *dhcpPacket, code byte) []byte {
	for _, option := range p.options {
		if len(option) >= 2 && option[0] == code {
			return option[2:]
		}
	}
	return nil
}

func dhcpOptionByte(p *dhcpPacket, code byte) byte {
	option := dhcpOption(p, code)
	if len(option) == 0 {
		return 0
	}
	return option[0]
}

func dhcpOptionAddr(t *testing.T, p *dhcpPacket, code byte) netip.Addr {
	t.Helper()
	option := dhcpOption(p, code)
	if len(option) != 4 {
		t.Fatalf("dhcp option %d length = %d, want 4", code, len(option))
	}
	return addr4(option)
}

func buildDNSQuery(id uint16, name string, qtype uint16) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], id)
	binary.BigEndian.PutUint16(msg[2:4], 0x0100)
	binary.BigEndian.PutUint16(msg[4:6], 1)
	for _, part := range bytes.Split([]byte(name), []byte{'.'}) {
		msg = append(msg, byte(len(part)))
		msg = append(msg, part...)
	}
	msg = append(msg, 0, 0, 0, 0, 0)
	binary.BigEndian.PutUint16(msg[len(msg)-4:len(msg)-2], qtype)
	binary.BigEndian.PutUint16(msg[len(msg)-2:], 1)
	return msg
}

func serveTestDNS(pc net.PacketConn) {
	buf := make([]byte, 512)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		req, err := parseDNS(buf[:n])
		if err != nil {
			return
		}
		answers := make([][]byte, 0)
		for _, q := range req.questions {
			if q.qtype == 1 {
				answers = append(answers, buildDNSAnswer(q, []byte{203, 0, 113, 9}, 60))
			}
		}
		_, err = pc.WriteTo(buildDNSResponse(req, answers, 0), addr)
		if err != nil {
			return
		}
	}
}

func serveUDPEcho(pc net.PacketConn) {
	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, err = pc.WriteTo(buf[:n], addr)
		if err != nil {
			return
		}
	}
}

func serveTCPEcho(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer func() {
				err := conn.Close()
				if err != nil {
					return
				}
			}()
			_, copyErr := io.Copy(conn, conn)
			if copyErr != nil {
				return
			}
		}(conn)
	}
}

func packetAddr(t *testing.T, addr net.Addr) (netip.Addr, uint16) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatal(err.Error())
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		t.Fatal(err.Error())
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ip, uint16(portInt)
}

func buildTCPFrame(cfg Config, destIP netip.Addr, sport uint16, dport uint16, seq uint32, ack uint32, flags byte, data []byte) []byte {
	tcp := &tcpPacket{
		sport:   sport,
		dport:   dport,
		seq:     seq,
		ack:     ack,
		flags:   flags,
		winsize: 64240,
	}
	msg := buildTCP(cfg.GuestIP, destIP, tcp, data, 0)
	ip := buildIPv4(ipProtoTCP, cfg.GuestIP, destIP, msg)
	return buildEth(routerMAC, cfg.GuestMAC, ethTypeIPv4, ip)
}

func parseTCPInbound(t *testing.T, frame []byte) *tcpPacket {
	t.Helper()
	packet, err := parseEth(frame)
	if err != nil {
		t.Fatal(err.Error())
	}
	if packet.ipv4 == nil || packet.ipv4.tcp == nil {
		t.Fatalf("inbound frame is not TCP: %x", frame)
	}
	return packet.ipv4.tcp
}
