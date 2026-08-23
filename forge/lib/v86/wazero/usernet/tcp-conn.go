package usernet

import (
	"bytes"
	"net"
	"net/netip"
	"sync"
)

// tcpConn bridges one guest TCP flow to a host TCP socket: it terminates
type tcpConn struct {
	stack *Stack
	key   string
	host  net.Conn

	hsrc  [6]byte
	hdest [6]byte
	psrc  netip.Addr
	pdest netip.Addr
	sport uint16
	dport uint16

	mtx sync.Mutex
	// state is the TCP state mirrored from v86's browser stack
	state string
	// seq is the next sequence number for guest-inbound data
	seq uint32
	// ack is the next expected guest sequence number
	ack uint32
	// startSeq is the guest initial sequence number
	startSeq uint32
	// winsize is the last guest-advertised receive window
	winsize uint16
	// lastAck tracks acknowledged guest-inbound bytes
	lastAck uint32
	// haveLastAck distinguishes a zero ACK from no ACK yet
	haveLastAck bool
	// sendBuffer contains host data waiting for guest ACKs
	sendBuffer bytes.Buffer
	// pending indicates one guest-inbound segment is awaiting ACK
	pending bool
	// inActiveClose indicates a FIN has been requested toward the guest
	inActiveClose bool
	// delayedSendFIN holds FIN until pending data is ACKed
	delayedSendFIN bool
	// delayedState is the state entered when the delayed FIN is sent
	delayedState string
	// closed indicates the host socket has been closed
	closed bool
}

// newTCPConn builds the connection in SYN-received state from the SYN
func newTCPConn(stack *Stack, key string, host net.Conn, packet *ethPacket) *tcpConn {
	return &tcpConn{
		stack: stack,
		key:   key,
		host:  host,
		hsrc:  routerMAC,
		hdest: packet.src,
		psrc:  packet.ipv4.dest,
		pdest: packet.ipv4.src,
		sport: packet.ipv4.tcp.dport,
		dport: packet.ipv4.tcp.sport,
		state: tcpStateSynReceived,
	}
}

// accept completes the handshake with a SYN-ACK and enters established
func (c *tcpConn) accept(packet *ethPacket) {
	c.mtx.Lock()
	c.seq = 1338
	c.ack = packet.ipv4.tcp.seq + 1
	c.startSeq = packet.ipv4.tcp.seq
	c.winsize = packet.ipv4.tcp.winsize
	c.state = tcpStateEstablished
	reply := &tcpPacket{
		sport:   c.sport,
		dport:   c.dport,
		seq:     1337,
		ack:     c.ack,
		flags:   tcpFlagSYN | tcpFlagACK,
		winsize: c.winsize,
	}
	c.stack.sendTCP(c, reply, nil, defaultMTU-ipv4HeaderSize-tcpHeaderSize)
	c.mtx.Unlock()
}

// start launches the host-to-guest read loop on its own goroutine; the
func (c *tcpConn) start() {
	go c.readLoop()
}

// process services one inbound segment: ACK accounting, FIN handling, and
func (c *tcpConn) process(packet *ethPacket) {
	tcp := packet.ipv4.tcp
	var data []byte
	c.mtx.Lock()
	if c.state == tcpStateClosed {
		reply := c.packetReplyLocked(tcp, tcpFlagRST)
		c.stack.sendTCP(c, reply, nil, 0)
		c.mtx.Unlock()
		return
	}
	if tcpFlag(tcp, tcpFlagRST) {
		c.state = tcpStateClosed
		c.mtx.Unlock()
		c.release(true)
		return
	}
	if tcpFlag(tcp, tcpFlagSYN) {
		c.mtx.Unlock()
		return
	}
	if tcpFlag(tcp, tcpFlagACK) {
		if c.state == tcpStateSynReceived {
			c.state = tcpStateEstablished
		}
		if c.state == tcpStateFinWait1 && !tcpFlag(tcp, tcpFlagFIN) {
			c.state = tcpStateFinWait2
		}
		if c.state == tcpStateClosing || c.state == tcpStateLastAck {
			c.state = tcpStateClosed
			c.mtx.Unlock()
			c.release(true)
			return
		}
		c.consumeAckLocked(tcp)
	}
	if tcpFlag(tcp, tcpFlagFIN) {
		release := c.processFINLocked(tcp)
		c.mtx.Unlock()
		if release {
			c.release(true)
		}
		return
	}
	if c.ack != tcp.seq {
		if c.ack != tcp.seq+1 {
			reply := c.packetReplyLocked(tcp, tcpFlagACK)
			c.stack.sendTCP(c, reply, nil, 0)
		}
		c.mtx.Unlock()
		return
	}
	if tcpFlag(tcp, tcpFlagACK) && len(tcp.data) > 0 {
		c.ack += uint32(len(tcp.data))
		reply := c.ipv4ReplyLocked(tcpFlagACK)
		c.stack.sendTCP(c, reply, nil, 0)
		data = bytes.Clone(tcp.data)
	}
	c.pumpLocked()
	c.mtx.Unlock()
	if len(data) != 0 {
		c.writeHost(data)
	}
}

// writeHost queues host data for delivery to the guest under the lock.
func (c *tcpConn) writeHost(dat []byte) {
	_, err := c.host.Write(dat)
	if err != nil {
		c.release(true)
	}
}

// writeFromHost segments queued host data into guest-inbound packets.
func (c *tcpConn) writeFromHost(dat []byte) {
	c.mtx.Lock()
	if !c.inActiveClose && c.state != tcpStateClosed {
		_, err := c.sendBuffer.Write(dat)
		if err != nil {
			c.mtx.Unlock()
			c.release(true)
			return
		}
	}
	c.pumpLocked()
	c.mtx.Unlock()
}

// closeFromHost begins an active close: send or defer FIN per pending data.
func (c *tcpConn) closeFromHost() {
	c.mtx.Lock()
	if !c.inActiveClose {
		c.inActiveClose = true
		next := ""
		if c.state == tcpStateEstablished || c.state == tcpStateSynReceived {
			next = tcpStateFinWait1
		}
		if c.state == tcpStateCloseWait {
			next = tcpStateLastAck
		}
		if next == "" {
			c.state = tcpStateClosed
			c.mtx.Unlock()
			c.release(true)
			return
		}
		if c.sendBuffer.Len() != 0 || c.pending {
			c.delayedSendFIN = true
			c.delayedState = next
			c.mtx.Unlock()
			return
		}
		c.state = next
		reply := c.ipv4ReplyLocked(tcpFlagACK | tcpFlagFIN)
		c.stack.sendTCP(c, reply, nil, 0)
	}
	c.pumpLocked()
	c.mtx.Unlock()
}

// closeHost closes the host socket exactly once.
func (c *tcpConn) closeHost() error {
	c.mtx.Lock()
	if c.closed {
		c.mtx.Unlock()
		return nil
	}
	c.closed = true
	c.mtx.Unlock()
	return c.host.Close()
}

// readLoop pumps host bytes toward the guest until the host socket ends.
func (c *tcpConn) readLoop() {
	buf := make([]byte, defaultMTU-ipv4HeaderSize-tcpHeaderSize)
	for {
		n, err := c.host.Read(buf)
		if n > 0 {
			c.writeFromHost(buf[:n])
		}
		if err != nil {
			c.closeFromHost()
			return
		}
	}
}

// consumeAckLocked advances past acknowledged send-buffer bytes and
func (c *tcpConn) consumeAckLocked(tcp *tcpPacket) {
	if !c.haveLastAck {
		c.lastAck = tcp.ack
		c.haveLastAck = true
		return
	}
	nack := int32(tcp.ack - c.lastAck)
	if nack > 0 {
		c.lastAck = tcp.ack
		if int(nack) > c.sendBuffer.Len() {
			nack = int32(c.sendBuffer.Len())
		}
		if nack > 0 {
			c.sendBuffer.Next(int(nack))
		}
		c.seq += uint32(nack)
		c.pending = false
		if c.delayedSendFIN && c.sendBuffer.Len() == 0 {
			c.delayedSendFIN = false
			c.state = c.delayedState
			reply := c.ipv4ReplyLocked(tcpFlagACK | tcpFlagFIN)
			c.stack.sendTCP(c, reply, nil, 0)
		}
		return
	}
	if nack < 0 {
		reply := c.packetReplyLocked(tcp, tcpFlagRST)
		c.stack.sendTCP(c, reply, nil, 0)
		c.state = tcpStateClosed
	}
}

// processFINLocked applies a guest FIN and reports whether the connection
func (c *tcpConn) processFINLocked(tcp *tcpPacket) bool {
	c.ack++
	reply := c.packetReplyLocked(tcp, tcpFlagACK)
	if c.state == tcpStateEstablished {
		c.state = tcpStateCloseWait
		c.stack.sendTCP(c, reply, nil, 0)
		c.shutdownHostWriteLocked()
		return false
	}
	if c.state == tcpStateFinWait1 {
		if tcpFlag(tcp, tcpFlagACK) {
			c.state = tcpStateClosed
			c.stack.sendTCP(c, reply, nil, 0)
			return true
		}
		c.state = tcpStateClosing
		c.stack.sendTCP(c, reply, nil, 0)
		return false
	}
	if c.state == tcpStateFinWait2 {
		c.state = tcpStateClosed
		c.stack.sendTCP(c, reply, nil, 0)
		return true
	}
	reply.flags = tcpFlagRST
	c.state = tcpStateClosed
	c.stack.sendTCP(c, reply, nil, 0)
	return true
}

// pumpLocked sends buffered host data while the guest window allows.
func (c *tcpConn) pumpLocked() {
	if c.sendBuffer.Len() == 0 || c.pending {
		return
	}
	size := c.sendBuffer.Len()
	maxSize := defaultMTU - ipv4HeaderSize - tcpHeaderSize
	if size > maxSize {
		size = maxSize
	}
	dat := bytes.Clone(c.sendBuffer.Bytes()[:size])
	reply := c.ipv4ReplyLocked(tcpFlagACK | tcpFlagPSH)
	c.stack.sendTCP(c, reply, dat, 0)
	c.pending = true
}

// ipv4ReplyLocked builds an empty reply segment with current sequence
func (c *tcpConn) ipv4ReplyLocked(flags byte) *tcpPacket {
	return &tcpPacket{
		sport:   c.sport,
		dport:   c.dport,
		seq:     c.seq,
		ack:     c.ack,
		flags:   flags,
		winsize: c.winsize,
	}
}

// packetReplyLocked builds a reply echoing one received segment numbers.
func (c *tcpConn) packetReplyLocked(tcp *tcpPacket, flags byte) *tcpPacket {
	return &tcpPacket{
		sport:   tcp.dport,
		dport:   tcp.sport,
		seq:     c.seq,
		ack:     c.ack,
		flags:   flags,
		winsize: tcp.winsize,
	}
}

// shutdownHostWriteLocked half-closes the host side after guest FIN.
func (c *tcpConn) shutdownHostWriteLocked() {
	type closeWriter interface {
		CloseWrite() error
	}
	if cw, ok := c.host.(closeWriter); ok {
		err := cw.CloseWrite()
		if err != nil {
			c.stack.le.WithError(err).Debug("tcp close write")
		}
	}
}

// release unregisters the flow and optionally closes the host socket.
func (c *tcpConn) release(closeHost bool) {
	c.stack.releaseTCP(c.key, c)
	if closeHost {
		err := c.closeHost()
		if err != nil {
			c.stack.le.WithError(err).Debug("tcp close host")
		}
	}
}
