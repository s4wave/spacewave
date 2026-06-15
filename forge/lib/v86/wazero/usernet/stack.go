// Package usernet implements a small usermode L2 network stack for v86 guests.
package usernet

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"
)

// Stack is the usermode network stack bridging guest L2 frames to host sockets.
type Stack struct {
	// cfg is the normalized stack configuration
	cfg Config
	// inbound delivers synthesized Ethernet frames to the guest
	inbound func(frame []byte)
	// le receives optional diagnostic logs
	le *logrus.Entry

	// ctx owns all host socket read loops
	ctx context.Context
	// cancel tears down host socket read loops
	cancel context.CancelFunc

	// mtx guards below fields
	mtx sync.Mutex
	// closed indicates Close has been called
	closed bool
	// guestMAC is learned from guest frames
	guestMAC [6]byte
	// udp maps guest UDP tuples to host sockets
	udp map[string]*udpConn
	// tcp maps guest TCP tuples to host sockets
	tcp map[string]*tcpConn
}

// New constructs a Stack. A nil le is replaced with a usable entry so the
// host-socket error paths can log without a nil dereference (this logrus fork
// does not nil-guard Entry methods); Debug records stay silent at the default
// level.
func New(cfg Config, inbound func(frame []byte), le *logrus.Entry) *Stack {
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg = cfg.withDefaults()
	s := &Stack{
		cfg:      cfg,
		inbound:  inbound,
		le:       le,
		ctx:      ctx,
		cancel:   cancel,
		guestMAC: cfg.GuestMAC,
		udp:      make(map[string]*udpConn),
		tcp:      make(map[string]*tcpConn),
	}
	return s
}

// HandleOutbound processes one guest-transmitted Ethernet frame.
func (s *Stack) HandleOutbound(frame []byte) {
	packet, err := parseEth(frame)
	if err != nil {
		return
	}
	s.learnGuest(packet.src)
	if packet.arp != nil {
		s.handleARP(packet)
		return
	}
	if packet.ipv4 == nil {
		return
	}
	if packet.ipv4.icmp != nil {
		s.handleICMP(packet)
		return
	}
	if packet.ipv4.udp != nil {
		s.handleUDP(packet)
		return
	}
	if packet.ipv4.tcp != nil {
		s.handleTCP(packet)
	}
}

// Close tears down all live TCP/UDP host connections.
func (s *Stack) Close() error {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return ErrStackClosed
	}
	s.closed = true
	s.cancel()
	udp := make([]*udpConn, 0, len(s.udp))
	for _, conn := range s.udp {
		udp = append(udp, conn)
	}
	tcp := make([]*tcpConn, 0, len(s.tcp))
	for _, conn := range s.tcp {
		tcp = append(tcp, conn)
	}
	s.udp = make(map[string]*udpConn)
	s.tcp = make(map[string]*tcpConn)
	s.mtx.Unlock()

	var ret error
	for _, conn := range udp {
		if err := conn.close(); err != nil && ret == nil {
			ret = err
		}
	}
	for _, conn := range tcp {
		if err := conn.closeHost(); err != nil && ret == nil {
			ret = err
		}
	}
	return ret
}

func (s *Stack) learnGuest(mac [6]byte) {
	s.mtx.Lock()
	s.guestMAC = mac
	s.mtx.Unlock()
}

func (s *Stack) currentGuestMAC() [6]byte {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.guestMAC
}

func (s *Stack) handleARP(packet *ethPacket) {
	if packet.arp.oper != 1 || packet.arp.ptype != ethTypeIPv4 {
		return
	}
	if packet.arp.tpa != s.cfg.GatewayIP && packet.arp.tpa != s.cfg.DNSServer {
		return
	}
	s.emit(buildARPReply(packet.src, packet.arp.tpa, packet.arp.spa))
}

func (s *Stack) handleICMP(packet *ethPacket) {
	if packet.ipv4.icmp.typ != 8 || packet.ipv4.dest != s.cfg.GatewayIP {
		return
	}
	icmp := buildICMP(0, packet.ipv4.icmp.code, packet.ipv4.icmp.data)
	ip := buildIPv4(ipProtoICMP, packet.ipv4.dest, packet.ipv4.src, icmp)
	s.emit(buildEth(packet.src, routerMAC, ethTypeIPv4, ip))
}

func (s *Stack) handleUDP(packet *ethPacket) {
	udp := packet.ipv4.udp
	if udp.sport == 68 && udp.dport == 67 {
		s.handleDHCP(packet)
		return
	}
	if udp.dport == 53 && packet.ipv4.dest == s.cfg.DNSServer {
		s.handleDNS(packet)
		return
	}
	s.handleUDPHost(packet)
}

func (s *Stack) handleDHCP(packet *ethPacket) {
	req, err := parseDHCP(packet.ipv4.udp.data)
	if err != nil {
		return
	}
	msgType := byte(0)
	for _, option := range req.options {
		if len(option) == 3 && option[0] == 53 {
			msgType = option[2]
		}
	}
	if msgType != 1 && msgType != 3 {
		return
	}

	replyType := byte(2)
	options := [][]byte{{53, 1, replyType}}
	if msgType == 3 {
		replyType = 5
		options[0] = []byte{53, 1, replyType}
		options = append(options, []byte{51, 4, 8, 0, 0, 0})
	}
	options = append(options,
		append([]byte{1, 4}, s.cfg.Netmask.AsSlice()...),
		append([]byte{3, 4}, s.cfg.GatewayIP.AsSlice()...),
		append([]byte{6, 4}, s.cfg.DNSServer.AsSlice()...),
		append([]byte{54, 4}, s.cfg.GatewayIP.AsSlice()...),
		append([]byte{60, 3}, v86ASCII...),
		[]byte{255, 0},
	)

	resp := &dhcpPacket{
		op:      2,
		htype:   1,
		hlen:    6,
		xid:     req.xid,
		yiaddr:  ipToLong(s.cfg.GuestIP),
		siaddr:  ipToLong(s.cfg.GatewayIP),
		giaddr:  ipToLong(s.cfg.GatewayIP),
		chaddr:  req.chaddr,
		options: options,
	}
	udp := buildUDP(s.cfg.GatewayIP, s.cfg.GuestIP, 67, 68, buildDHCP(resp))
	ip := buildIPv4(ipProtoUDP, s.cfg.GatewayIP, s.cfg.GuestIP, udp)
	s.emit(buildEth(packet.src, routerMAC, ethTypeIPv4, ip))
}

func (s *Stack) handleDNS(packet *ethPacket) {
	req, err := parseDNS(packet.ipv4.udp.data)
	if err != nil {
		return
	}
	answers := make([][]byte, 0)
	rcode := byte(0)
	for _, q := range req.questions {
		if q.qclass != 1 || (q.qtype != 1 && q.qtype != 28) {
			continue
		}
		addrs, err := s.cfg.Resolver.LookupIPAddr(s.ctx, q.name)
		if err != nil {
			rcode = 3
			continue
		}
		for _, addr := range addrs {
			if q.qtype == 1 {
				if v4 := addr.IP.To4(); v4 != nil {
					answers = append(answers, buildDNSAnswer(q, v4, 600))
				}
				continue
			}
			if q.qtype == 28 && addr.IP.To4() == nil {
				if v6 := addr.IP.To16(); v6 != nil {
					answers = append(answers, buildDNSAnswer(q, v6, 600))
				}
			}
		}
	}
	data := buildDNSResponse(req, answers, rcode)
	udp := buildUDP(s.cfg.DNSServer, packet.ipv4.src, 53, packet.ipv4.udp.sport, data)
	ip := buildIPv4(ipProtoUDP, s.cfg.DNSServer, packet.ipv4.src, udp)
	s.emit(buildEth(packet.src, routerMAC, ethTypeIPv4, ip))
}

func (s *Stack) handleUDPHost(packet *ethPacket) {
	key := udpTuple(packet.ipv4.src, packet.ipv4.udp.sport, packet.ipv4.dest, packet.ipv4.udp.dport)
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	conn := s.udp[key]
	if conn != nil {
		s.mtx.Unlock()
		conn.write(packet.ipv4.udp.data)
		return
	}
	s.mtx.Unlock()

	addr := net.JoinHostPort(packet.ipv4.dest.String(), strconv.Itoa(int(packet.ipv4.udp.dport)))
	host, err := s.cfg.Dialer.DialContext(s.ctx, "udp", addr)
	if err != nil {
		return
	}
	conn = newUDPConn(s, key, host, packet)
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		err = conn.close()
		if err != nil {
			s.le.WithError(err).Debug("close udp after stack close")
		}
		return
	}
	if existing := s.udp[key]; existing != nil {
		s.mtx.Unlock()
		err = conn.close()
		if err != nil {
			s.le.WithError(err).Debug("close duplicate udp")
		}
		existing.write(packet.ipv4.udp.data)
		return
	}
	s.udp[key] = conn
	s.mtx.Unlock()
	conn.start()
	conn.write(packet.ipv4.udp.data)
}

func (s *Stack) handleTCP(packet *ethPacket) {
	key := tcpTuple(packet.ipv4.src, packet.ipv4.tcp.sport, packet.ipv4.dest, packet.ipv4.tcp.dport)
	if tcpFlag(packet.ipv4.tcp, tcpFlagSYN) && !tcpFlag(packet.ipv4.tcp, tcpFlagACK) {
		s.openTCP(key, packet)
		return
	}
	s.mtx.Lock()
	conn := s.tcp[key]
	s.mtx.Unlock()
	if conn == nil {
		s.sendTCPReset(packet)
		return
	}
	conn.process(packet)
}

func (s *Stack) openTCP(key string, packet *ethPacket) {
	addr := net.JoinHostPort(packet.ipv4.dest.String(), strconv.Itoa(int(packet.ipv4.tcp.dport)))
	host, err := s.cfg.Dialer.DialContext(s.ctx, "tcp", addr)
	if err != nil {
		s.sendTCPReset(packet)
		return
	}
	conn := newTCPConn(s, key, host, packet)
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		err = conn.closeHost()
		if err != nil {
			s.le.WithError(err).Debug("close tcp after stack close")
		}
		return
	}
	if old := s.tcp[key]; old != nil {
		delete(s.tcp, key)
		err = old.closeHost()
		if err != nil {
			s.le.WithError(err).Debug("close replaced tcp")
		}
	}
	s.tcp[key] = conn
	s.mtx.Unlock()
	conn.accept(packet)
	conn.start()
}

func (s *Stack) sendTCPReset(packet *ethPacket) {
	bop := packet.ipv4.tcp.ack
	if tcpFlag(packet.ipv4.tcp, tcpFlagFIN) || tcpFlag(packet.ipv4.tcp, tcpFlagSYN) {
		bop++
	}
	ack := packet.ipv4.tcp.seq
	if tcpFlag(packet.ipv4.tcp, tcpFlagSYN) {
		ack++
	}
	reply := &tcpPacket{
		sport:   packet.ipv4.tcp.dport,
		dport:   packet.ipv4.tcp.sport,
		seq:     bop,
		ack:     ack,
		flags:   tcpFlagRST,
		winsize: packet.ipv4.tcp.winsize,
	}
	if tcpFlag(packet.ipv4.tcp, tcpFlagSYN) {
		reply.flags |= tcpFlagACK
	}
	tcp := buildTCP(packet.ipv4.dest, packet.ipv4.src, reply, nil, 0)
	ip := buildIPv4(ipProtoTCP, packet.ipv4.dest, packet.ipv4.src, tcp)
	s.emit(buildEth(packet.src, routerMAC, ethTypeIPv4, ip))
}

func (s *Stack) sendTCP(conn *tcpConn, tcp *tcpPacket, data []byte, mss uint16) {
	msg := buildTCP(conn.psrc, conn.pdest, tcp, data, mss)
	ip := buildIPv4(ipProtoTCP, conn.psrc, conn.pdest, msg)
	s.emit(buildEth(conn.hdest, conn.hsrc, ethTypeIPv4, ip))
}

func (s *Stack) sendUDP(conn *udpConn, data []byte) {
	udp := buildUDP(conn.psrc, conn.pdest, conn.sport, conn.dport, data)
	ip := buildIPv4(ipProtoUDP, conn.psrc, conn.pdest, udp)
	s.emit(buildEth(conn.hdest, conn.hsrc, ethTypeIPv4, ip))
}

func (s *Stack) releaseUDP(key string) {
	s.mtx.Lock()
	delete(s.udp, key)
	s.mtx.Unlock()
}

func (s *Stack) releaseTCP(key string, conn *tcpConn) {
	s.mtx.Lock()
	if s.tcp[key] == conn {
		delete(s.tcp, key)
	}
	s.mtx.Unlock()
}

func (s *Stack) emit(frame []byte) {
	if s.inbound == nil {
		return
	}
	s.inbound(frame)
}

func ipToLong(addr netip.Addr) uint32 {
	return binary.BigEndian.Uint32(addr.AsSlice())
}

func udpTuple(src netip.Addr, sport uint16, dest netip.Addr, dport uint16) string {
	return connTuple(src, sport, dest, dport)
}

func tcpTuple(src netip.Addr, sport uint16, dest netip.Addr, dport uint16) string {
	return connTuple(src, sport, dest, dport)
}

func connTuple(src netip.Addr, sport uint16, dest netip.Addr, dport uint16) string {
	return src.String() + ":" + strconv.Itoa(int(sport)) + ":" + dest.String() + ":" + strconv.Itoa(int(dport))
}
