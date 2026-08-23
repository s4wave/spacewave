package v86_wazero

import (
	"context"
	"encoding/binary"
	"strconv"
	"sync"
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

// virtioV86FSDevice exposes the v86 filesystem server to the guest over a
// virtio PCI device.
type virtioV86FSDevice struct {
	virtioCommonConfig

	host    *HostRuntime
	session *unixfs_v86fs.LocalSession

	queues [3]*virtioQueue

	driverOK      atomic.Bool
	lastStatus    atomic.Uint32
	kicks         [3]atomic.Uint64
	requests      atomic.Uint64
	replies       atomic.Uint64
	notifications atomic.Uint64

	traceMu sync.Mutex
	trace   []string
}

// recordTrace appends a wire-type label to the bounded device message trace. The
// guest CPU goroutine records here while a boot proof reads it on failure, so a
// stalled v86fs root mount shows exactly which request/reply/notify frames
// crossed the ring rather than only aggregate counts.
func (d *virtioV86FSDevice) recordTrace(dir string, typeByte byte) {
	d.traceMu.Lock()
	d.trace = append(d.trace, dir+":"+v86fsWireTypeName(typeByte))
	if len(d.trace) > 64 {
		d.trace = d.trace[len(d.trace)-64:]
	}
	d.traceMu.Unlock()
}

// v86fsWireTypeName names a v86fs wire message type byte for the trace.
func v86fsWireTypeName(typeByte byte) string {
	switch typeByte {
	case 0x00:
		return "mount"
	case 0x01:
		return "lookup"
	case 0x02:
		return "getattr"
	case 0x03:
		return "readdir"
	case 0x04:
		return "open"
	case 0x05:
		return "close"
	case 0x06:
		return "read"
	case 0x07:
		return "create"
	case 0x08:
		return "write"
	case 0x09:
		return "mkdir"
	case 0x0a:
		return "setattr"
	case 0x0b:
		return "fsync"
	case 0x0c:
		return "unlink"
	case 0x0d:
		return "rename"
	case 0x0e:
		return "symlink"
	case 0x0f:
		return "readlink"
	case 0x10:
		return "statfs"
	case 0x22:
		return "mount_notify"
	case 0x23:
		return "umount_notify"
	case 0x80:
		return "mount_reply"
	case 0x81:
		return "lookup_reply"
	case 0x82:
		return "getattr_reply"
	case 0x83:
		return "readdir_reply"
	case 0x84:
		return "open_reply"
	case 0x86:
		return "read_reply"
	case 0xff:
		return "error_reply"
	default:
		return "0x" + strconv.FormatUint(uint64(typeByte), 16)
	}
}

// v86fsStats is a device-side readback of the v86fs virtio handshake. It exists
// so a boot proof can tell whether the guest reached DRIVER_OK, kicked each
// queue, issued any request, and received any MOUNT_NOTIFY independent of where
// Linux relocated the device BARs.
type v86fsStats struct {
	driverOK      bool
	lastStatus    uint32
	irqLine       uint32
	kicks         [3]uint64
	requests      uint64
	replies       uint64
	notifications uint64
	trace         []string
}

// stats snapshots the device counters and the bounded wire trace.
func (d *virtioV86FSDevice) stats() v86fsStats {
	if d == nil {
		return v86fsStats{}
	}
	d.traceMu.Lock()
	trace := append([]string(nil), d.trace...)
	d.traceMu.Unlock()
	return v86fsStats{
		driverOK:      d.driverOK.Load(),
		lastStatus:    d.lastStatus.Load(),
		irqLine:       d.assignedIRQ(),
		kicks:         [3]uint64{d.kicks[0].Load(), d.kicks[1].Load(), d.kicks[2].Load()},
		requests:      d.requests.Load(),
		replies:       d.replies.Load(),
		notifications: d.notifications.Load(),
		trace:         trace,
	}
}

// virtioQueueDevice is the seam the virtio queue needs into its owning
type virtioQueueDevice interface {
	virtioHost() *HostRuntime
	virtioRaiseIRQ(context.Context, uint32)
	featureNegotiated(uint32) bool
}

// virtioQueue is one split virtqueue: guest-posted descriptor/avail/used
type virtioQueue struct {
	device        virtioQueueDevice
	size          uint32
	sizeSupported uint32
	enabled       bool
	notifyOffset  uint32
	descAddr      uint32
	availAddr     uint32
	availLastIdx  uint16
	usedAddr      uint32
	stagedReplies uint32
}

// virtioDesc is one split-virtqueue descriptor table entry.
type virtioDesc struct {
	addr  uint32
	len   uint32
	flags uint16
	next  uint16
}

// virtioBufferChain walks one descriptor chain: gathered read buffers for
type virtioBufferChain struct {
	queue          *virtioQueue
	headIdx        uint16
	readBuffers    []virtioDesc
	writeBuffers   []virtioDesc
	lengthReadable uint32
	lengthWritable uint32
	lengthWritten  uint32
}

// registerV86FS attaches a v86fs server to the host runtime as a three-
func (h *HostRuntime) registerV86FS(ctx context.Context, server *unixfs_v86fs.Server) {
	if h.pci == nil {
		return
	}
	dev := &virtioV86FSDevice{
		virtioCommonConfig: virtioCommonConfig{featuresOK: true},
		host:               h,
		session:            unixfs_v86fs.NewLocalSession(ctx, server),
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

// registerCommonPorts wires the shared virtio common configuration.
func (d *virtioV86FSDevice) registerCommonPorts() {
	registerVirtioCommonPorts(virtioV86FSCommonPort, d)
}

// numQueues reports the NUM_QUEUES common configuration value.
func (d *virtioV86FSDevice) numQueues() uint32 { return uint32(len(d.queues)) }

// registerNotifyPorts wires one kick port per queue; queue 2 flushes
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

// registerISRPort wires the shared ISR status port.
func (d *virtioV86FSDevice) registerISRPort() {
	registerVirtioISRPort(virtioV86FSISRPort, d)
}

// selectedQueue returns the queue addressed by queueSelect, or an empty
func (d *virtioV86FSDevice) selectedQueue() *virtioQueue {
	if d.queueSelect >= uint32(len(d.queues)) {
		return &virtioQueue{}
	}
	return d.queues[d.queueSelect]
}

// applyStatus latches the status write and flushes pending notifications
func (d *virtioV86FSDevice) applyStatus(ctx context.Context, value uint32) {
	applied := d.handleStatusWrite(ctx, d, value)
	if applied == 0 {
		return
	}
	d.lastStatus.Store(applied)
	if applied&virtioStatusDriverOK != 0 {
		d.driverOK.Store(true)
		d.flushNotifications(ctx)
	}
}

// reset clears the common configuration and every queue.
func (d *virtioV86FSDevice) reset(ctx context.Context) {
	d.resetState()
	for _, queue := range d.queues {
		queue.reset()
	}
	d.lowerIRQ(ctx)
}

// handleQueue services one available-ring request on the given queue:
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
		if len(req) > 4 {
			d.recordTrace("rq", req[4])
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
			if len(resp) > 4 {
				d.recordTrace("rp", resp[4])
			}
			d.replies.Add(1)
		}
		queue.pushReply(chain)
	}
	queue.flushReplies(ctx)
	d.flushNotifications(ctx)
}

// flushNotifications delivers queued server notifications into queue 2,
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
		if len(frame) > 4 {
			d.recordTrace("nt", frame[4])
		}
		d.notifications.Add(1)
		delivered++
	}
	if delivered < len(pending) {
		d.session.RequeueNotifications(pending[delivered:])
	}
	queue.flushReplies(ctx)
}

// assignedIRQ returns the IRQ the guest actually bound to this device. SeaBIOS
// and Linux route the PCI INTx pin through the PIIX3 PIRQ router by slot and
// write the resulting line into config register 0x3c; the device must signal
// that line, not its power-on default, or the guest's handler never sees the
// completion interrupt and wait_for_completion in v86fs_request hangs the mount.
func (d *virtioV86FSDevice) assignedIRQ() uint32 {
	space := d.host.pci.spaces[virtioV86FSPCIID]
	if len(space) > 0x3c {
		if line := space[0x3c]; line != 0 && line != 0xff {
			return uint32(line)
		}
	}
	return virtioV86FSIRQ
}

// raiseIRQ asserts the routed device interrupt line with ISR bits set.
func (d *virtioV86FSDevice) raiseIRQ(ctx context.Context, typ uint32) {
	d.isrStatus |= typ
	_ = d.host.raiseIRQ(ctx, d.assignedIRQ())
}

// lowerIRQ clears the ISR state and deasserts the interrupt line.
func (d *virtioV86FSDevice) lowerIRQ(ctx context.Context) {
	d.isrStatus = 0
	_ = d.host.lowerIRQ(ctx, d.assignedIRQ())
}

// virtioHost exposes the owning host runtime to the queue machinery.
func (d *virtioV86FSDevice) virtioHost() *HostRuntime {
	return d.host
}

// virtioRaiseIRQ routes queue completion interrupts through the device.
func (d *virtioV86FSDevice) virtioRaiseIRQ(ctx context.Context, typ uint32) {
	d.raiseIRQ(ctx, typ)
}

// reset clears the queue state back to its supported size.
func (q *virtioQueue) reset() {
	q.enabled = false
	q.descAddr = 0
	q.availAddr = 0
	q.availLastIdx = 0
	q.usedAddr = 0
	q.stagedReplies = 0
	q.setSize(q.sizeSupported)
}

// setSize clamps the driver-requested size to the supported maximum and
func (q *virtioQueue) setSize(size uint32) {
	if size == 0 || size > q.sizeSupported {
		size = q.sizeSupported
	}
	q.size = nextPowerOfTwo(size)
}

// configured reports whether the queue is enabled with all addresses set.
func (q *virtioQueue) configured() bool {
	return q.enabled && q.descAddr != 0 && q.availAddr != 0 && q.usedAddr != 0 && q.size != 0
}

// canEnable reports whether the driver posted all queue addresses.
func (q *virtioQueue) canEnable() bool {
	return q.descAddr != 0 && q.availAddr != 0 && q.usedAddr != 0 && q.size != 0
}

// mask returns the ring index wrap mask derived from the queue size.
func (q *virtioQueue) mask() uint32 {
	return q.size - 1
}

// hasRequest reports whether an unconsumed available buffer remains.
func (q *virtioQueue) hasRequest() bool {
	return q.requestCount() != 0
}

// requestCount returns the number of buffers the driver has made available
func (q *virtioQueue) requestCount() uint32 {
	return uint32(q.availIdx() - q.availLastIdx)
}

// popRequest takes the next available buffer head as a descriptor chain.
func (q *virtioQueue) popRequest() (*virtioBufferChain, error) {
	if !q.hasRequest() {
		return nil, errors.New("virtio queue has no request")
	}
	head := q.availEntry(q.availLastIdx)
	q.availLastIdx++
	return newVirtioBufferChain(q, head)
}

// pushReply stages one used-ring entry; entries publish on flushReplies.
func (q *virtioQueue) pushReply(chain *virtioBufferChain) {
	usedIdx := (q.usedIdx() + uint16(q.stagedReplies)) & uint16(q.mask())
	host := q.device.virtioHost()
	host.guestWriteUint32(q.usedAddr+4+uint32(usedIdx)*8, uint32(chain.headIdx))
	host.guestWriteUint32(q.usedAddr+8+uint32(usedIdx)*8, chain.lengthWritten)
	q.stagedReplies++
}

// flushReplies publishes staged used entries, advances the used index, and
func (q *virtioQueue) flushReplies(ctx context.Context) {
	if q.stagedReplies == 0 {
		return
	}
	q.device.virtioHost().guestWriteUint16(q.usedAddr+2, uint16(uint32(q.usedIdx())+q.stagedReplies))
	q.stagedReplies = 0
	if q.device.featureNegotiated(29) || q.availFlags()&virtqAvailNoIRQ == 0 {
		q.device.virtioRaiseIRQ(ctx, virtioISRQueue)
	}
}

// notifyMeAfter arms the avail no-interrupt watermark after skipped
func (q *virtioQueue) notifyMeAfter(skipped uint32) {
	if q.usedAddr == 0 || q.size == 0 {
		return
	}
	availEvent := uint16(uint32(q.availIdx()) + skipped)
	q.device.virtioHost().guestWriteUint16(q.usedAddr+4+q.size*8, availEvent)
}

// availFlags reads the avail ring flag word.
func (q *virtioQueue) availFlags() uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr)
}

// availIdx reads the driver write index of the avail ring.
func (q *virtioQueue) availIdx() uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr + 2)
}

// availEntry reads the descriptor head at one avail ring slot.
func (q *virtioQueue) availEntry(idx uint16) uint16 {
	return q.device.virtioHost().guestReadUint16(q.availAddr + 4 + 2*(uint32(idx)&q.mask()))
}

// usedIdx reads the device write index of the used ring.
func (q *virtioQueue) usedIdx() uint16 {
	return q.device.virtioHost().guestReadUint16(q.usedAddr + 2)
}

// newVirtioBufferChain walks a descriptor chain from its head, following
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

// descriptor reads one descriptor table entry from guest memory.
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

// readAll gathers the chain read buffers into one byte slice.
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

// write scatters data across the chain write buffers and reports how many
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

// guestRead copies bytes out of guest linear memory, reporting ok=false
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

// guestWrite copies bytes into guest linear memory, reporting success.
func (h *HostRuntime) guestWrite(addr uint32, data []byte) bool {
	if uint64(addr)+uint64(len(data)) > uint64(h.guestMemorySize) {
		return false
	}
	return h.Module.Memory().Write(h.guestMemoryOffset+addr, data)
}

// guestReadUint16 reads one little-endian uint16 from guest memory.
func (h *HostRuntime) guestReadUint16(addr uint32) uint16 {
	data, ok := h.guestRead(addr, 2)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint16(data)
}

// guestReadUint32 reads one little-endian uint32 from guest memory.
func (h *HostRuntime) guestReadUint32(addr uint32) uint32 {
	data, ok := h.guestRead(addr, 4)
	if !ok {
		return 0
	}
	return binary.LittleEndian.Uint32(data)
}

// guestWriteUint16 writes one little-endian uint16 to guest memory.
func (h *HostRuntime) guestWriteUint16(addr uint32, value uint16) {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	h.guestWrite(addr, data[:])
}

// guestWriteUint32 writes one little-endian uint32 to guest memory.
func (h *HostRuntime) guestWriteUint32(addr, value uint32) {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	h.guestWrite(addr, data[:])
}

// newVirtioV86FSPCISpace builds the device config space with its four PCI
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

// writeVirtioPCICap writes one virtio PCI capability structure.
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

// nextPowerOfTwo rounds value up to the next power of two.
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
