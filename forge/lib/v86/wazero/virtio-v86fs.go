package v86_wazero

import (
	"context"
	"encoding/binary"
	"sync/atomic"

	"github.com/pkg/errors"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

const (
	virtioV86FSPCIID      = 0x0e << 3
	virtioV86FSIRQ        = 11
	virtioV86FSCommonPort = 0xf800
	virtioV86FSNotifyPort = 0xf900
	virtioV86FSISRPort    = 0xf700

	virtioStatusDriverOK = 4
	virtioStatusFailed   = 128
	virtioISRQueue       = 1

	virtqDescNext     = 1
	virtqDescWrite    = 2
	virtqDescIndirect = 4
	virtqAvailNoIRQ   = 1

	virtqDescIndirectFeature = 1 << 28
	virtqEventIdxFeature     = 1 << 29
)

type virtioV86FSDevice struct {
	host    *HostRuntime
	session *unixfs_v86fs.LocalSession

	deviceFeatureSelect uint32
	driverFeatureSelect uint32
	deviceFeatures      [4]uint32
	driverFeatures      [4]uint32
	featuresOK          bool
	status              uint32
	configGeneration    uint32
	queueSelect         uint32
	queues              [3]*virtioQueue
	isrStatus           uint32

	driverOK      atomic.Bool
	lastStatus    atomic.Uint32
	kicks         [3]atomic.Uint64
	requests      atomic.Uint64
	replies       atomic.Uint64
	notifications atomic.Uint64
}

// v86fsStats is a device-side readback of the v86fs virtio handshake. It exists
// so a boot proof can tell whether the guest reached DRIVER_OK, kicked each
// queue, issued any request, and received any MOUNT_NOTIFY independent of where
// Linux relocated the device BARs.
type v86fsStats struct {
	driverOK      bool
	lastStatus    uint32
	kicks         [3]uint64
	requests      uint64
	replies       uint64
	notifications uint64
}

func (d *virtioV86FSDevice) stats() v86fsStats {
	if d == nil {
		return v86fsStats{}
	}
	return v86fsStats{
		driverOK:      d.driverOK.Load(),
		lastStatus:    d.lastStatus.Load(),
		kicks:         [3]uint64{d.kicks[0].Load(), d.kicks[1].Load(), d.kicks[2].Load()},
		requests:      d.requests.Load(),
		replies:       d.replies.Load(),
		notifications: d.notifications.Load(),
	}
}

type virtioQueueDevice interface {
	virtioHost() *HostRuntime
	virtioRaiseIRQ(context.Context, uint32)
	virtioFeatureNegotiated(uint32) bool
}

type virtioQueue struct {
	device        virtioQueueDevice
	size          uint32
	sizeSupported uint32
	enabled       bool
	notifyOffset  uint32
	descAddr      uint32
	availAddr     uint32
	availLastIdx  uint32
	usedAddr      uint32
	stagedReplies uint32
}

type virtioDesc struct {
	addr  uint32
	len   uint32
	flags uint16
	next  uint16
}

type virtioBufferChain struct {
	queue          *virtioQueue
	headIdx        uint16
	readBuffers    []virtioDesc
	writeBuffers   []virtioDesc
	lengthReadable uint32
	lengthWritable uint32
	lengthWritten  uint32
}

func (h *HostRuntime) registerV86FS(ctx context.Context, server *unixfs_v86fs.Server) {
	if h.pci == nil {
		return
	}
	dev := &virtioV86FSDevice{
		host:       h,
		session:    unixfs_v86fs.NewLocalSession(ctx, server),
		featuresOK: true,
	}
	dev.deviceFeatures[1] = 1 // VIRTIO_F_VERSION_1.
	for i := range dev.queues {
		dev.queues[i] = &virtioQueue{
			device:        dev,
			size:          128,
			sizeSupported: 128,
			notifyOffset:  uint32(i),
		}
	}
	h.v86fs = dev
	h.pci.spaces[virtioV86FSPCIID] = newVirtioV86FSPCISpace()
	h.pci.setBARSize(virtioV86FSPCIID, 0, 64, true)
	h.pci.setBARSize(virtioV86FSPCIID, 1, 16, true)
	h.pci.setBARSize(virtioV86FSPCIID, 2, 16, true)
	dev.registerCommonPorts()
	dev.registerNotifyPorts()
	dev.registerISRPort()
}

func (d *virtioV86FSDevice) registerCommonPorts() {
	fields := []struct {
		offset uint16
		width  int
		read   func() uint32
		write  func(context.Context, uint32)
	}{
		{0, 32, func() uint32 { return d.deviceFeatureSelect }, func(_ context.Context, value uint32) { d.deviceFeatureSelect = value }},
		{4, 32, func() uint32 { return d.deviceFeatures[d.deviceFeatureSelect&3] }, func(context.Context, uint32) {}},
		{8, 32, func() uint32 { return d.driverFeatureSelect }, func(_ context.Context, value uint32) { d.driverFeatureSelect = value }},
		{12, 32, func() uint32 { return d.driverFeatures[d.driverFeatureSelect&3] }, func(_ context.Context, value uint32) {
			idx := d.driverFeatureSelect & 3
			supported := d.deviceFeatures[idx]
			d.driverFeatures[idx] = value & supported
			d.featuresOK = d.featuresOK && value&^supported == 0
		}},
		{16, 16, func() uint32 { return 0xffff }, func(context.Context, uint32) {}},
		{18, 16, func() uint32 { return uint32(len(d.queues)) }, func(context.Context, uint32) {}},
		{20, 8, func() uint32 { return d.status }, func(ctx context.Context, value uint32) { d.writeStatus(ctx, value) }},
		{21, 8, func() uint32 { return d.configGeneration }, func(context.Context, uint32) {}},
		{22, 16, func() uint32 { return d.queueSelect }, func(_ context.Context, value uint32) { d.queueSelect = value }},
		{24, 16, func() uint32 { return d.selectedQueue().size }, func(_ context.Context, value uint32) { d.selectedQueue().setSize(value) }},
		{26, 16, func() uint32 { return 0xffff }, func(context.Context, uint32) {}},
		{28, 16, func() uint32 {
			if d.selectedQueue().enabled {
				return 1
			}
			return 0
		}, func(_ context.Context, value uint32) {
			if value == 1 && d.selectedQueue().canEnable() {
				d.selectedQueue().enabled = true
			}
		}},
		{30, 16, func() uint32 { return d.selectedQueue().notifyOffset }, func(context.Context, uint32) {}},
		{32, 32, func() uint32 { return d.selectedQueue().descAddr }, func(_ context.Context, value uint32) { d.selectedQueue().descAddr = value }},
		{36, 32, func() uint32 { return 0 }, func(context.Context, uint32) {}},
		{40, 32, func() uint32 { return d.selectedQueue().availAddr }, func(_ context.Context, value uint32) { d.selectedQueue().availAddr = value }},
		{44, 32, func() uint32 { return 0 }, func(context.Context, uint32) {}},
		{48, 32, func() uint32 { return d.selectedQueue().usedAddr }, func(_ context.Context, value uint32) { d.selectedQueue().usedAddr = value }},
		{52, 32, func() uint32 { return 0 }, func(context.Context, uint32) {}},
	}
	for _, field := range fields {
		port := virtioV86FSCommonPort + field.offset
		switch field.width {
		case 8:
			d.host.RegisterIORead(port, 8, func(context.Context, uint16) uint32 { return field.read() & 0xff })
			d.host.RegisterIOWrite(port, 8, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xff) })
		case 16:
			d.host.RegisterIORead(port, 16, func(context.Context, uint16) uint32 { return field.read() & 0xffff })
			d.host.RegisterIORead(port, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			d.host.RegisterIORead(port+1, 8, func(_ context.Context, p uint16) uint32 {
				return (field.read() >> ((p - port) * 8)) & 0xff
			})
			d.host.RegisterIOWrite(port, 16, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value&0xffff) })
		case 32:
			d.host.RegisterIORead(port, 32, func(context.Context, uint16) uint32 { return field.read() })
			for i := range uint16(4) {
				offset := i
				d.host.RegisterIORead(port+offset, 8, func(context.Context, uint16) uint32 {
					return (field.read() >> (offset * 8)) & 0xff
				})
			}
			d.host.RegisterIOWrite(port, 32, func(ctx context.Context, _ uint16, value uint32) { field.write(ctx, value) })
		}
	}
}

func (d *virtioV86FSDevice) registerNotifyPorts() {
	for i := range d.queues {
		queueID := i
		port := virtioV86FSNotifyPort + uint16(queueID*2)
		d.host.RegisterIORead(port, 16, func(context.Context, uint16) uint32 { return 0xffff })
		d.host.RegisterIOWrite(port, 16, func(ctx context.Context, _ uint16, _ uint32) {
			d.kicks[queueID].Add(1)
			if queueID == 2 {
				d.flushNotifications(ctx)
				return
			}
			d.handleQueue(ctx, queueID)
		})
	}
}

func (d *virtioV86FSDevice) registerISRPort() {
	d.host.RegisterIORead(virtioV86FSISRPort, 8, func(ctx context.Context, _ uint16) uint32 {
		value := d.isrStatus
		d.lowerIRQ(ctx)
		return value
	})
	d.host.RegisterIOWrite(virtioV86FSISRPort, 8, func(context.Context, uint16, uint32) {})
}

func (d *virtioV86FSDevice) selectedQueue() *virtioQueue {
	if d.queueSelect >= uint32(len(d.queues)) {
		return &virtioQueue{}
	}
	return d.queues[d.queueSelect]
}

func (d *virtioV86FSDevice) writeStatus(ctx context.Context, value uint32) {
	if value == 0 {
		d.reset(ctx)
		return
	}
	if !d.featuresOK {
		value &^= 8
	}
	d.status = value
	d.lastStatus.Store(value)
	if value&virtioStatusFailed != 0 {
		d.raiseIRQ(ctx, virtioISRQueue)
	}
	if value&virtioStatusDriverOK != 0 {
		d.driverOK.Store(true)
		d.flushNotifications(ctx)
	}
}

func (d *virtioV86FSDevice) reset(ctx context.Context) {
	d.driverFeatureSelect = 0
	d.deviceFeatureSelect = 0
	d.driverFeatures = d.deviceFeatures
	d.featuresOK = true
	d.status = 0
	d.queueSelect = 0
	for _, queue := range d.queues {
		queue.reset()
	}
	d.lowerIRQ(ctx)
}

func (d *virtioV86FSDevice) handleQueue(ctx context.Context, queueID int) {
	queue := d.queues[queueID]
	for queue.configured() && queue.hasRequest() {
		chain, err := queue.popRequest()
		if err != nil {
			d.raiseIRQ(ctx, virtioISRQueue)
			return
		}
		req, err := chain.readAll()
		if err != nil {
			d.raiseIRQ(ctx, virtioISRQueue)
			return
		}
		msg, err := unixfs_v86fs.DecodeBinaryFrame(req)
		if err != nil {
			d.raiseIRQ(ctx, virtioISRQueue)
			return
		}
		d.requests.Add(1)
		reply, err := d.session.HandleMessage(ctx, msg)
		if err != nil {
			d.raiseIRQ(ctx, virtioISRQueue)
			return
		}
		if reply != nil && chain.lengthWritable != 0 {
			resp, err := unixfs_v86fs.EncodeBinaryFrame(reply)
			if err != nil {
				d.raiseIRQ(ctx, virtioISRQueue)
				return
			}
			_ = chain.write(resp)
			d.replies.Add(1)
		}
		queue.pushReply(chain)
	}
	queue.flushReplies(ctx)
	d.flushNotifications(ctx)
}

func (d *virtioV86FSDevice) flushNotifications(ctx context.Context) {
	queue := d.queues[2]
	if !queue.configured() {
		return
	}
	// A notification leaves the session pending queue only once it lands in a
	// guest receive buffer. flushNotifications runs at DRIVER_OK before the guest
	// has posted any buffer, so undelivered frames are requeued for the next
	// queue-2 kick instead of being dropped, otherwise the seed MOUNT_NOTIFY is
	// lost and root never mounts.
	pending := d.session.DrainNotifications()
	delivered := 0
	for _, msg := range pending {
		if !queue.hasRequest() {
			break
		}
		frame, err := unixfs_v86fs.EncodeBinaryFrame(msg)
		if err != nil {
			break
		}
		chain, err := queue.popRequest()
		if err != nil {
			break
		}
		_ = chain.write(frame)
		queue.pushReply(chain)
		d.notifications.Add(1)
		delivered++
	}
	if delivered < len(pending) {
		d.session.RequeueNotifications(pending[delivered:])
	}
	queue.flushReplies(ctx)
}

func (d *virtioV86FSDevice) raiseIRQ(ctx context.Context, typ uint32) {
	d.isrStatus |= typ
	_ = d.host.raiseIRQ(ctx, virtioV86FSIRQ)
}

func (d *virtioV86FSDevice) lowerIRQ(ctx context.Context) {
	d.isrStatus = 0
	_ = d.host.lowerIRQ(ctx, virtioV86FSIRQ)
}

func (d *virtioV86FSDevice) virtioHost() *HostRuntime {
	return d.host
}

func (d *virtioV86FSDevice) virtioRaiseIRQ(ctx context.Context, typ uint32) {
	d.raiseIRQ(ctx, typ)
}

func (d *virtioV86FSDevice) virtioFeatureNegotiated(bit uint32) bool {
	idx := bit >> 5
	if idx >= uint32(len(d.driverFeatures)) {
		return false
	}
	return d.driverFeatures[idx]&(1<<(bit&31)) != 0
}

func (q *virtioQueue) reset() {
	q.enabled = false
	q.descAddr = 0
	q.availAddr = 0
	q.availLastIdx = 0
	q.usedAddr = 0
	q.stagedReplies = 0
	q.setSize(q.sizeSupported)
}

func (q *virtioQueue) setSize(size uint32) {
	if size == 0 || size > q.sizeSupported {
		size = q.sizeSupported
	}
	q.size = nextPowerOfTwo(size)
}

func (q *virtioQueue) configured() bool {
	return q.enabled && q.descAddr != 0 && q.availAddr != 0 && q.usedAddr != 0 && q.size != 0
}

func (q *virtioQueue) canEnable() bool {
	return q.descAddr != 0 && q.availAddr != 0 && q.usedAddr != 0 && q.size != 0
}

func (q *virtioQueue) mask() uint32 {
	return q.size - 1
}

func (q *virtioQueue) hasRequest() bool {
	return q.requestCount() != 0
}

func (q *virtioQueue) requestCount() uint32 {
	return uint32(uint16(q.availIdx() - uint16(q.availLastIdx)))
}

func (q *virtioQueue) popRequest() (*virtioBufferChain, error) {
	if !q.hasRequest() {
		return nil, errors.New("virtio queue has no request")
	}
	head := q.availEntry(q.availLastIdx)
	q.availLastIdx = uint32(uint16(q.availLastIdx + 1))
	return newVirtioBufferChain(q, head)
}

func (q *virtioQueue) pushReply(chain *virtioBufferChain) {
	usedIdx := (q.usedIdx() + uint16(q.stagedReplies)) & uint16(q.mask())
	host := q.device.virtioHost()
	host.guestWriteUint32(q.usedAddr+4+uint32(usedIdx)*8, uint32(chain.headIdx))
	host.guestWriteUint32(q.usedAddr+8+uint32(usedIdx)*8, chain.lengthWritten)
	q.stagedReplies++
}

func (q *virtioQueue) flushReplies(ctx context.Context) {
	if q.stagedReplies == 0 {
		return
	}
	q.device.virtioHost().guestWriteUint16(q.usedAddr+2, uint16(uint32(q.usedIdx())+q.stagedReplies))
	q.stagedReplies = 0
	if q.device.virtioFeatureNegotiated(29) || q.availFlags()&virtqAvailNoIRQ == 0 {
		q.device.virtioRaiseIRQ(ctx, virtioISRQueue)
	}
}

func (q *virtioQueue) notifyMeAfter(skipped uint32) {
	if q.usedAddr == 0 || q.size == 0 {
		return
	}
	availEvent := uint16(uint32(q.availIdx()) + skipped)
	q.device.virtioHost().guestWriteUint16(q.usedAddr+4+q.size*8, availEvent)
}

func (q *virtioQueue) availFlags() uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr)
}

func (q *virtioQueue) availIdx() uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr + 2)
}

func (q *virtioQueue) availEntry(idx uint32) uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr + 4 + 2*(idx&q.mask()))
}

func (q *virtioQueue) usedIdx() uint16 {
	return q.device.virtioHost().guestReadUint16(q.usedAddr + 2)
}

func newVirtioBufferChain(q *virtioQueue, head uint16) (*virtioBufferChain, error) {
	chain := &virtioBufferChain{queue: q, headIdx: head}
	tableAddr := q.descAddr
	descIdx := head
	limit := q.size
	for count := uint32(0); count <= limit; count++ {
		desc := q.descriptor(tableAddr, descIdx)
		if desc.flags&virtqDescIndirect != 0 {
			tableAddr = desc.addr
			descIdx = 0
			limit = desc.len / 16
			continue
		}
		if desc.flags&virtqDescWrite != 0 {
			chain.writeBuffers = append(chain.writeBuffers, desc)
			chain.lengthWritable += desc.len
		} else {
			chain.readBuffers = append(chain.readBuffers, desc)
			chain.lengthReadable += desc.len
		}
		if desc.flags&virtqDescNext == 0 {
			return chain, nil
		}
		descIdx = desc.next
	}
	return nil, errors.New("virtio descriptor chain cycle")
}

func (q *virtioQueue) descriptor(tableAddr uint32, idx uint16) virtioDesc {
	base := tableAddr + uint32(idx)*16
	host := q.device.virtioHost()
	return virtioDesc{
		addr:  host.guestReadUint32(base),
		len:   host.guestReadUint32(base + 8),
		flags: host.guestReadUint16(base + 12),
		next:  host.guestReadUint16(base + 14),
	}
}

func (c *virtioBufferChain) readAll() ([]byte, error) {
	out := make([]byte, 0, c.lengthReadable)
	host := c.queue.device.virtioHost()
	for _, buf := range c.readBuffers {
		data, ok := host.guestRead(buf.addr, buf.len)
		if !ok {
			return nil, errors.Errorf("read guest buffer at %#x", buf.addr)
		}
		out = append(out, data...)
	}
	return out, nil
}

func (c *virtioBufferChain) write(data []byte) uint32 {
	var written uint32
	host := c.queue.device.virtioHost()
	for _, buf := range c.writeBuffers {
		if len(data) == 0 {
			break
		}
		n := min(len(data), int(buf.len))
		if host.guestWrite(buf.addr, data[:n]) {
			written += uint32(n)
		}
		data = data[n:]
	}
	c.lengthWritten += written
	return written
}

func (h *HostRuntime) guestRead(addr, size uint32) ([]byte, bool) {
	if uint64(addr)+uint64(size) > uint64(h.guestMemorySize) {
		return nil, false
	}
	data, ok := h.Module.Memory().Read(h.guestMemoryOffset+addr, size)
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

func (h *HostRuntime) guestWrite(addr uint32, data []byte) bool {
	if uint64(addr)+uint64(len(data)) > uint64(h.guestMemorySize) {
		return false
	}
	return h.Module.Memory().Write(h.guestMemoryOffset+addr, data)
}

func (h *HostRuntime) guestReadUint16(addr uint32) uint16 {
	data, ok := h.guestRead(addr, 2)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint16(data)
}

func (h *HostRuntime) guestReadUint32(addr uint32) uint32 {
	data, ok := h.guestRead(addr, 4)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

func (h *HostRuntime) guestWriteUint16(addr uint32, value uint16) {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	h.guestWrite(addr, data[:])
}

func (h *HostRuntime) guestWriteUint32(addr, value uint32) {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	h.guestWrite(addr, data[:])
}

func newVirtioV86FSPCISpace() []byte {
	space := make([]byte, 256)
	copy(space, []byte{
		0xf4, 0x1a, 0x7f, 0x10,
		0x07, 0x05, 0x10, 0x00,
		0x01, 0x00, 0x02, 0x00,
		0x00, 0x00, 0x00, 0x00,
	})
	binary.LittleEndian.PutUint32(space[0x10:], virtioV86FSCommonPort|1)
	binary.LittleEndian.PutUint32(space[0x14:], virtioV86FSNotifyPort|1)
	binary.LittleEndian.PutUint32(space[0x18:], virtioV86FSISRPort|1)
	binary.LittleEndian.PutUint16(space[0x2c:], 0x1af4)
	binary.LittleEndian.PutUint16(space[0x2e:], 63)
	space[0x34] = 0x40
	space[0x3c] = virtioV86FSIRQ
	space[0x3d] = 1
	writeVirtioPCICap(space, 0x40, 0x50, 1, 0, 0, 64, nil)
	writeVirtioPCICap(space, 0x50, 0x64, 2, 1, 0, 16, []byte{2, 0, 0, 0})
	writeVirtioPCICap(space, 0x64, 0x74, 3, 2, 0, 16, nil)
	writeVirtioPCICap(space, 0x74, 0, 5, 0, 0, 0, []byte{0, 0, 0, 0})
	return space
}

func writeVirtioPCICap(space []byte, off, next int, typ, bar byte, capOffset, size uint32, extra []byte) {
	space[off] = 0x09
	space[off+1] = byte(next)
	space[off+2] = byte(16 + len(extra))
	space[off+3] = typ
	space[off+4] = bar
	binary.LittleEndian.PutUint32(space[off+8:], capOffset)
	binary.LittleEndian.PutUint32(space[off+12:], size)
	copy(space[off+16:], extra)
}

func nextPowerOfTwo(value uint32) uint32 {
	if value <= 1 {
		return 1
	}
	value--
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	return value + 1
}
