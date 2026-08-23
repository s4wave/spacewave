package usernet

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strings"
)

const (
	ethTypeIPv4 = 0x0800
	ethTypeARP  = 0x0806

	ipProtoICMP = 1
	ipProtoTCP  = 6
	ipProtoUDP  = 17

	ethHeaderSize  = 14
	ipv4HeaderSize = 20
	icmpHeaderSize = 4
	udpHeaderSize  = 8
	tcpHeaderSize  = 20
	dhcpCookie     = 0x63825363
	defaultMTU     = 1500
)

var (
	broadcastMAC = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	routerMAC    = [6]byte{0x52, 0x54, 0x00, 0x01, 0x02, 0x03}
	v86ASCII     = []byte{118, 56, 54}
)

// ethPacket is a parsed Ethernet frame with its decoded payload layered on
type ethPacket struct {
	dest      [6]byte
	src       [6]byte
	ethType   uint16
	arp       *arpPacket
	ipv4      *ipv4Packet
	payload   []byte
	origFrame []byte
}

// arpPacket is a parsed ARP request or reply.
type arpPacket struct {
	htype uint16
	ptype uint16
	hlen  byte
	plen  byte
	oper  uint16
	sha   [6]byte
	spa   netip.Addr
	tha   [6]byte
	tpa   netip.Addr
}

// ipv4Packet is a parsed IPv4 datagram with its decoded payload layered on
type ipv4Packet struct {
	tos     byte
	id      uint16
	ttl     byte
	proto   byte
	src     netip.Addr
	dest    netip.Addr
	payload []byte
	icmp    *icmpPacket
	udp     *udpPacket
	tcp     *tcpPacket
}

// icmpPacket is a parsed ICMP message: type, code, and raw body.
type icmpPacket struct {
	typ  byte
	code byte
	data []byte
}

// udpPacket is a parsed UDP datagram.
type udpPacket struct {
	sport uint16
	dport uint16
	data  []byte
}

// tcpPacket is a parsed TCP segment header plus its payload bytes.
type tcpPacket struct {
	sport   uint16
	dport   uint16
	seq     uint32
	ack     uint32
	flags   byte
	winsize uint16
	urgent  uint16
	data    []byte
}

// dhcpPacket is a parsed DHCP discover/request/reply message.
type dhcpPacket struct {
	op      byte
	htype   byte
	hlen    byte
	hops    byte
	xid     uint32
	secs    uint16
	flags   uint16
	ciaddr  uint32
	yiaddr  uint32
	siaddr  uint32
	giaddr  uint32
	chaddr  [16]byte
	options [][]byte
}

// dnsQuestion is one parsed DNS question and its offset in the raw query.
type dnsQuestion struct {
	name      string
	rawName   []byte
	qtype     uint16
	qclass    uint16
	nameStart int
}

// dnsPacket is a parsed DNS query awaiting a synthesized answer.
type dnsPacket struct {
	id        uint16
	flags     uint16
	questions []dnsQuestion
	rawQuery  []byte
}

// parseEth decodes an Ethernet frame header and its payload by ethertype.
func parseEth(frame []byte) (*ethPacket, error) {
	if len(frame) < ethHeaderSize {
		return nil, ErrShortFrame
	}
	p := &ethPacket{
		ethType:   binary.BigEndian.Uint16(frame[12:14]),
		payload:   frame[ethHeaderSize:],
		origFrame: frame,
	}
	copy(p.dest[:], frame[0:6])
	copy(p.src[:], frame[6:12])
	if p.ethType == ethTypeARP {
		arp, err := parseARP(p.payload)
		if err != nil {
			return nil, err
		}
		p.arp = arp
		return p, nil
	}
	if p.ethType == ethTypeIPv4 {
		ipv4, err := parseIPv4(p.payload)
		if err != nil {
			return nil, err
		}
		p.ipv4 = ipv4
	}
	return p, nil
}

// parseARP decodes an ARP packet from an Ethernet payload.
func parseARP(dat []byte) (*arpPacket, error) {
	if len(dat) < 28 {
		return nil, ErrShortFrame
	}
	p := &arpPacket{
		htype: binary.BigEndian.Uint16(dat[0:2]),
		ptype: binary.BigEndian.Uint16(dat[2:4]),
		hlen:  dat[4],
		plen:  dat[5],
		oper:  binary.BigEndian.Uint16(dat[6:8]),
		spa:   addr4(dat[14:18]),
		tpa:   addr4(dat[24:28]),
	}
	copy(p.sha[:], dat[8:14])
	copy(p.tha[:], dat[18:24])
	return p, nil
}

// parseIPv4 decodes an IPv4 header and its payload by protocol.
func parseIPv4(dat []byte) (*ipv4Packet, error) {
	if len(dat) < ipv4HeaderSize {
		return nil, ErrShortFrame
	}
	ihl := int(dat[0]&0x0f) * 4
	if ihl < ipv4HeaderSize || len(dat) < ihl {
		return nil, ErrShortFrame
	}
	total := int(binary.BigEndian.Uint16(dat[2:4]))
	if total < ihl {
		return nil, ErrShortFrame
	}
	if total > len(dat) {
		total = len(dat)
	}
	p := &ipv4Packet{
		tos:     dat[1],
		id:      binary.BigEndian.Uint16(dat[4:6]),
		ttl:     dat[8],
		proto:   dat[9],
		src:     addr4(dat[12:16]),
		dest:    addr4(dat[16:20]),
		payload: dat[ihl:total],
	}
	if p.proto == ipProtoICMP {
		icmp, err := parseICMP(p.payload)
		if err != nil {
			return nil, err
		}
		p.icmp = icmp
		return p, nil
	}
	if p.proto == ipProtoUDP {
		udp, err := parseUDP(p.payload)
		if err != nil {
			return nil, err
		}
		p.udp = udp
		return p, nil
	}
	if p.proto == ipProtoTCP {
		tcp, err := parseTCP(p.payload)
		if err != nil {
			return nil, err
		}
		p.tcp = tcp
	}
	return p, nil
}

// parseICMP decodes an ICMP message from an IPv4 payload.
func parseICMP(dat []byte) (*icmpPacket, error) {
	if len(dat) < icmpHeaderSize {
		return nil, ErrShortFrame
	}
	return &icmpPacket{
		typ:  dat[0],
		code: dat[1],
		data: bytes.Clone(dat[icmpHeaderSize:]),
	}, nil
}

// parseUDP decodes a UDP datagram from an IPv4 payload.
func parseUDP(dat []byte) (*udpPacket, error) {
	if len(dat) < udpHeaderSize {
		return nil, ErrShortFrame
	}
	size := int(binary.BigEndian.Uint16(dat[4:6]))
	if size < udpHeaderSize {
		return nil, ErrShortFrame
	}
	if size > len(dat) {
		size = len(dat)
	}
	return &udpPacket{
		sport: binary.BigEndian.Uint16(dat[0:2]),
		dport: binary.BigEndian.Uint16(dat[2:4]),
		data:  bytes.Clone(dat[udpHeaderSize:size]),
	}, nil
}

// parseTCP decodes a TCP segment from an IPv4 payload.
func parseTCP(dat []byte) (*tcpPacket, error) {
	if len(dat) < tcpHeaderSize {
		return nil, ErrShortFrame
	}
	offset := int(dat[12]>>4) * 4
	if offset < tcpHeaderSize || offset > len(dat) {
		return nil, ErrShortFrame
	}
	return &tcpPacket{
		sport:   binary.BigEndian.Uint16(dat[0:2]),
		dport:   binary.BigEndian.Uint16(dat[2:4]),
		seq:     binary.BigEndian.Uint32(dat[4:8]),
		ack:     binary.BigEndian.Uint32(dat[8:12]),
		flags:   dat[13],
		winsize: binary.BigEndian.Uint16(dat[14:16]),
		urgent:  binary.BigEndian.Uint16(dat[18:20]),
		data:    bytes.Clone(dat[offset:]),
	}, nil
}

// buildEth frames an Ethernet header around a payload.
func buildEth(dest [6]byte, src [6]byte, ethType uint16, payload []byte) []byte {
	frame := make([]byte, ethHeaderSize+len(payload))
	copy(frame[0:6], dest[:])
	copy(frame[6:12], src[:])
	binary.BigEndian.PutUint16(frame[12:14], ethType)
	copy(frame[ethHeaderSize:], payload)
	return frame
}

// buildARPReply builds an ARP reply advertising senderIP at routerMAC.
func buildARPReply(dest [6]byte, senderIP netip.Addr, targetIP netip.Addr) []byte {
	payload := make([]byte, 28)
	binary.BigEndian.PutUint16(payload[0:2], 1)
	binary.BigEndian.PutUint16(payload[2:4], ethTypeIPv4)
	payload[4] = 6
	payload[5] = 4
	binary.BigEndian.PutUint16(payload[6:8], 2)
	copy(payload[8:14], routerMAC[:])
	copy(payload[14:18], senderIP.AsSlice())
	copy(payload[18:24], dest[:])
	copy(payload[24:28], targetIP.AsSlice())
	return buildEth(dest, routerMAC, ethTypeARP, payload)
}

// buildIPv4 frames an IPv4 datagram header around a payload.
func buildIPv4(proto byte, src netip.Addr, dest netip.Addr, payload []byte) []byte {
	ip := make([]byte, ipv4HeaderSize+len(payload))
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)))
	ip[6] = 2 << 5
	ip[8] = 32
	ip[9] = proto
	copy(ip[12:16], src.AsSlice())
	copy(ip[16:20], dest.AsSlice())
	binary.BigEndian.PutUint16(ip[10:12], inetChecksum(ip, 0))
	copy(ip[ipv4HeaderSize:], payload)
	return ip
}

// buildICMP frames an ICMP echo reply message.
func buildICMP(typ byte, code byte, data []byte) []byte {
	msg := make([]byte, icmpHeaderSize+len(data))
	msg[0] = typ
	msg[1] = code
	copy(msg[icmpHeaderSize:], data)
	binary.BigEndian.PutUint16(msg[2:4], inetChecksum(msg, 0))
	return msg
}

// buildUDP frames a UDP datagram with checksummed pseudo-header.
func buildUDP(src netip.Addr, dest netip.Addr, sport uint16, dport uint16, data []byte) []byte {
	msg := make([]byte, udpHeaderSize+len(data))
	binary.BigEndian.PutUint16(msg[0:2], sport)
	binary.BigEndian.PutUint16(msg[2:4], dport)
	binary.BigEndian.PutUint16(msg[4:6], uint16(len(msg)))
	copy(msg[udpHeaderSize:], data)
	sum := pseudoHeaderChecksum(src, dest, ipProtoUDP, len(msg))
	binary.BigEndian.PutUint16(msg[6:8], inetChecksum(msg, sum))
	return msg
}

// buildTCP frames a TCP segment, splitting the payload by the MSS when
func buildTCP(src netip.Addr, dest netip.Addr, tcp *tcpPacket, data []byte, mss uint16) []byte {
	offset := tcpHeaderSize
	if mss != 0 {
		offset += 4
	}
	msg := make([]byte, offset+len(data))
	binary.BigEndian.PutUint16(msg[0:2], tcp.sport)
	binary.BigEndian.PutUint16(msg[2:4], tcp.dport)
	binary.BigEndian.PutUint32(msg[4:8], tcp.seq)
	binary.BigEndian.PutUint32(msg[8:12], tcp.ack)
	msg[12] = byte(offset/4) << 4
	msg[13] = tcp.flags
	binary.BigEndian.PutUint16(msg[14:16], tcp.winsize)
	binary.BigEndian.PutUint16(msg[18:20], tcp.urgent)
	if mss != 0 {
		msg[20] = 2
		msg[21] = 4
		binary.BigEndian.PutUint16(msg[22:24], mss)
	}
	copy(msg[offset:], data)
	sum := pseudoHeaderChecksum(src, dest, ipProtoTCP, len(msg))
	binary.BigEndian.PutUint16(msg[16:18], inetChecksum(msg, sum))
	return msg
}

// inetChecksum folds running sum into the ones-complement Internet
func inetChecksum(dat []byte, checksum uint32) uint16 {
	end := len(dat) &^ 1
	for i := 0; i < end; i += 2 {
		checksum += uint32(dat[i])<<8 | uint32(dat[i+1])
	}
	if len(dat)&1 != 0 {
		checksum += uint32(dat[end]) << 8
	}
	for checksum>>16 != 0 {
		checksum = (checksum & 0xffff) + (checksum >> 16)
	}
	return ^uint16(checksum)
}

// pseudoHeaderChecksum seeds the TCP/UDP checksum with the IPv4
func pseudoHeaderChecksum(src netip.Addr, dest netip.Addr, proto byte, size int) uint32 {
	sb := src.AsSlice()
	db := dest.AsSlice()
	return (uint32(sb[0])<<8 | uint32(sb[1])) +
		(uint32(sb[2])<<8 | uint32(sb[3])) +
		(uint32(db[0])<<8 | uint32(db[1])) +
		(uint32(db[2])<<8 | uint32(db[3])) +
		uint32(proto) + uint32(size)
}

// tcpFlag reports whether one TCP flag bit is set.
func tcpFlag(tcp *tcpPacket, flag byte) bool {
	return tcp.flags&flag != 0
}

// parseDHCP decodes a DHCP message and its option fields.
func parseDHCP(dat []byte) (*dhcpPacket, error) {
	if len(dat) < 240 {
		return nil, ErrShortFrame
	}
	p := &dhcpPacket{
		op:     dat[0],
		htype:  dat[1],
		hlen:   dat[2],
		hops:   dat[3],
		xid:    binary.BigEndian.Uint32(dat[4:8]),
		secs:   binary.BigEndian.Uint16(dat[8:10]),
		flags:  binary.BigEndian.Uint16(dat[10:12]),
		ciaddr: binary.BigEndian.Uint32(dat[12:16]),
		yiaddr: binary.BigEndian.Uint32(dat[16:20]),
		siaddr: binary.BigEndian.Uint32(dat[20:24]),
		giaddr: binary.BigEndian.Uint32(dat[24:28]),
	}
	copy(p.chaddr[:], dat[28:44])
	if binary.BigEndian.Uint32(dat[236:240]) != dhcpCookie {
		return nil, ErrUnsupportedPacket
	}
	for i := 240; i < len(dat); i++ {
		code := dat[i]
		if code == 0 {
			continue
		}
		if code == 255 {
			p.options = append(p.options, []byte{255})
			return p, nil
		}
		if i+1 >= len(dat) {
			return nil, ErrShortFrame
		}
		size := int(dat[i+1])
		if i+2+size > len(dat) {
			return nil, ErrShortFrame
		}
		p.options = append(p.options, bytes.Clone(dat[i:i+2+size]))
		i += 1 + size
	}
	return p, nil
}

// buildDHCP constructs a DHCP reply offering the given addresses to chaddr.
func buildDHCP(p *dhcpPacket) []byte {
	msg := make([]byte, 240)
	msg[0] = p.op
	msg[1] = p.htype
	msg[2] = p.hlen
	msg[3] = p.hops
	binary.BigEndian.PutUint32(msg[4:8], p.xid)
	binary.BigEndian.PutUint16(msg[8:10], p.secs)
	binary.BigEndian.PutUint16(msg[10:12], p.flags)
	binary.BigEndian.PutUint32(msg[12:16], p.ciaddr)
	binary.BigEndian.PutUint32(msg[16:20], p.yiaddr)
	binary.BigEndian.PutUint32(msg[20:24], p.siaddr)
	binary.BigEndian.PutUint32(msg[24:28], p.giaddr)
	copy(msg[28:44], p.chaddr[:])
	binary.BigEndian.PutUint32(msg[236:240], dhcpCookie)
	for _, option := range p.options {
		msg = append(msg, option...)
	}
	return msg
}

// parseDNS decodes a DNS query's question section.
func parseDNS(dat []byte) (*dnsPacket, error) {
	if len(dat) < 12 {
		return nil, ErrShortFrame
	}
	p := &dnsPacket{
		id:       binary.BigEndian.Uint16(dat[0:2]),
		flags:    binary.BigEndian.Uint16(dat[2:4]),
		rawQuery: bytes.Clone(dat),
	}
	qdcount := int(binary.BigEndian.Uint16(dat[4:6]))
	offset := 12
	for range qdcount {
		start := offset
		labels := make([]string, 0, 4)
		for {
			if offset >= len(dat) {
				return nil, ErrShortFrame
			}
			size := int(dat[offset])
			offset++
			if size == 0 {
				break
			}
			if size&0xc0 != 0 {
				return nil, ErrUnsupportedPacket
			}
			if offset+size > len(dat) {
				return nil, ErrShortFrame
			}
			labels = append(labels, string(dat[offset:offset+size]))
			offset += size
		}
		if offset+4 > len(dat) {
			return nil, ErrShortFrame
		}
		p.questions = append(p.questions, dnsQuestion{
			name:      strings.Join(labels, "."),
			rawName:   bytes.Clone(dat[start:offset]),
			qtype:     binary.BigEndian.Uint16(dat[offset : offset+2]),
			qclass:    binary.BigEndian.Uint16(dat[offset+2 : offset+4]),
			nameStart: start,
		})
		offset += 4
	}
	return p, nil
}

// buildDNSResponse answers each question: A records resolve to the host
func buildDNSResponse(req *dnsPacket, answers [][]byte, rcode byte) []byte {
	size := 12
	for _, q := range req.questions {
		size += len(q.rawName) + 4
	}
	for _, answer := range answers {
		size += len(answer)
	}
	msg := make([]byte, size)
	binary.BigEndian.PutUint16(msg[0:2], req.id)
	flags := uint16(0x8180) | uint16(rcode&0x0f)
	if req.flags&0x0100 != 0 {
		flags |= 0x0100
	}
	binary.BigEndian.PutUint16(msg[2:4], flags)
	binary.BigEndian.PutUint16(msg[4:6], uint16(len(req.questions)))
	binary.BigEndian.PutUint16(msg[6:8], uint16(len(answers)))
	offset := 12
	for _, q := range req.questions {
		copy(msg[offset:], q.rawName)
		offset += len(q.rawName)
		binary.BigEndian.PutUint16(msg[offset:offset+2], q.qtype)
		binary.BigEndian.PutUint16(msg[offset+2:offset+4], q.qclass)
		offset += 4
	}
	for _, answer := range answers {
		copy(msg[offset:], answer)
		offset += len(answer)
	}
	return msg
}

// buildDNSAnswer appends one A-record answer for a name.
func buildDNSAnswer(q dnsQuestion, dat []byte, ttl uint32) []byte {
	answer := make([]byte, 12+len(dat))
	binary.BigEndian.PutUint16(answer[0:2], 0xc000|uint16(q.nameStart))
	binary.BigEndian.PutUint16(answer[2:4], q.qtype)
	binary.BigEndian.PutUint16(answer[4:6], q.qclass)
	binary.BigEndian.PutUint32(answer[6:10], ttl)
	binary.BigEndian.PutUint16(answer[10:12], uint16(len(dat)))
	copy(answer[12:], dat)
	return answer
}

// addr4 renders a netip.Addr as four network-order bytes.
func addr4(dat []byte) netip.Addr {
	return netip.AddrFrom4([4]byte{dat[0], dat[1], dat[2], dat[3]})
}
