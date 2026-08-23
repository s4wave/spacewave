package v86_wazero

import (
	"context"
	"encoding/binary"
)

const (
	virtioHost9PPCIID      = 0x06 << 3
	virtioHost9PIRQ        = 10
	virtioHost9PCommonPort = 0xa800
	virtioHost9PNotifyPort = 0xa900
	virtioHost9PISRPort    = 0xa700
	virtioHost9PConfigPort = 0xa600

	virtioHost9PMountTagFeature = 1
)

// virtioHost9PDevice exposes a Host9PFS to the guest over a virtio-mmio
// style PCI device.
type virtioHost9PDevice struct {
	virtioCommonConfig

	host *HostRuntime
	fs   *Host9PFS

	queue *virtioQueue
}

func (h *HostRuntime) registerHost9P(fs *Host9PFS) {
	if h.pci == nil || fs == nil {
		return
	}
	dev := &virtioHost9PDevice{
		virtioCommonConfig: virtioCommonConfig{featuresOK: true},
		host:               h,
		fs:                 fs,
	}
	dev.deviceFeatures[0] = virtioHost9PMountTagFeature | virtqDescIndirectFeature | virtqEventIdxFeature
	dev.deviceFeatures[1] = 1 // VIRTIO_F_VERSION_1.
	dev.queue = &virtioQueue{
		device:        dev,
		size:          32,
		sizeSupported: 32,
	}
	h.pci.spaces[virtioHost9PPCIID] = newVirtioHost9PPCISpace()
	h.pci.setBARSize(virtioHost9PPCIID, 0, 64, true)
	h.pci.setBARSize(virtioHost9PPCIID, 1, 16, true)
	h.pci.setBARSize(virtioHost9PPCIID, 2, 16, true)
	h.pci.setBARSize(virtioHost9PPCIID, 3, 256, true)
	dev.registerCommonPorts()
	dev.registerNotifyPorts()
	dev.registerISRPort()
	dev.registerConfigPorts()
}

func (d *virtioHost9PDevice) registerCommonPorts() {
	registerVirtioCommonPorts(virtioHost9PCommonPort, d)
}

// numQueues reports the NUM_QUEUES common configuration value.
func (d *virtioHost9PDevice) numQueues() uint32 { return 1 }

func (d *virtioHost9PDevice) registerNotifyPorts() {
	d.host.RegisterIORead(virtioHost9PNotifyPort, 16, func(context.Context, uint16) uint32 { return 0xffff })
	d.host.RegisterIOWrite(virtioHost9PNotifyPort, 16, func(ctx context.Context, _ uint16, _ uint32) {
		d.handleQueue(ctx)
	})
}

func (d *virtioHost9PDevice) registerISRPort() {
	registerVirtioISRPort(virtioHost9PISRPort, d)
}

func (d *virtioHost9PDevice) registerConfigPorts() {
	tag := []byte("host9p")
	d.host.RegisterIORead(virtioHost9PConfigPort, 16, func(context.Context, uint16) uint32 {
		return uint32(len(tag))
	})
	for i := range uint16(254) {
		offset := i
		d.host.RegisterIORead(virtioHost9PConfigPort+2+offset, 8, func(context.Context, uint16) uint32 {
			if int(offset) >= len(tag) {
				return 0
			}
			return uint32(tag[offset])
		})
		d.host.RegisterIOWrite(virtioHost9PConfigPort+2+offset, 8, func(context.Context, uint16, uint32) {})
	}
	d.host.RegisterIOWrite(virtioHost9PConfigPort, 16, func(context.Context, uint16, uint32) {})
}

func (d *virtioHost9PDevice) selectedQueue() *virtioQueue {
	if d.queueSelect != 0 {
		return &virtioQueue{}
	}
	return d.queue
}

func (d *virtioHost9PDevice) applyStatus(ctx context.Context, value uint32) {
	applied := d.handleStatusWrite(ctx, d, value)
	if applied&virtioStatusDriverOK != 0 {
		d.handleQueue(ctx)
	}
}

func (d *virtioHost9PDevice) reset(ctx context.Context) {
	d.resetState()
	d.queue.reset()
	d.lowerIRQ(ctx)
}

func (d *virtioHost9PDevice) handleQueue(ctx context.Context) {
	queue := d.queue
	d.fs.notifies.Add(1)
	if queue.configured() {
		d.fs.queueConfigured.Store(1)
	} else {
		d.fs.queueConfigured.Store(0)
	}
	d.fs.availIdx.Store(uint32(queue.availIdx()))
	d.fs.availLastIdx.Store(uint32(queue.availLastIdx))
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
		reply := d.fs.Handle(req)
		if len(reply) != 0 && chain.lengthWritable != 0 {
			_ = chain.write(reply)
		}
		queue.pushReply(chain)
	}
	queue.notifyMeAfter(0)
	queue.flushReplies(ctx)
}

func (d *virtioHost9PDevice) raiseIRQ(ctx context.Context, typ uint32) {
	d.isrStatus |= typ
	_ = d.host.raiseIRQ(ctx, virtioHost9PIRQ)
}

func (d *virtioHost9PDevice) lowerIRQ(ctx context.Context) {
	d.isrStatus = 0
	_ = d.host.lowerIRQ(ctx, virtioHost9PIRQ)
}

func (d *virtioHost9PDevice) virtioHost() *HostRuntime {
	return d.host
}

func (d *virtioHost9PDevice) virtioRaiseIRQ(ctx context.Context, typ uint32) {
	d.raiseIRQ(ctx, typ)
}

func newVirtioHost9PPCISpace() []byte {
	space := make([]byte, 256)
	copy(space, []byte{
		0xf4, 0x1a, 0x49, 0x10,
		0x07, 0x05, 0x10, 0x00,
		0x01, 0x00, 0x02, 0x00,
		0x00, 0x00, 0x00, 0x00,
	})
	binary.LittleEndian.PutUint32(space[0x10:], virtioHost9PCommonPort|1)
	binary.LittleEndian.PutUint32(space[0x14:], virtioHost9PNotifyPort|1)
	binary.LittleEndian.PutUint32(space[0x18:], virtioHost9PISRPort|1)
	binary.LittleEndian.PutUint32(space[0x1c:], virtioHost9PConfigPort|1)
	binary.LittleEndian.PutUint16(space[0x2c:], 0x1af4)
	binary.LittleEndian.PutUint16(space[0x2e:], 9)
	space[0x34] = 0x40
	space[0x3c] = virtioHost9PIRQ
	space[0x3d] = 1
	writeVirtioPCICap(space, 0x40, 0x50, 1, 0, 0, 64, nil)
	writeVirtioPCICap(space, 0x50, 0x64, 2, 1, 0, 16, []byte{2, 0, 0, 0})
	writeVirtioPCICap(space, 0x64, 0x74, 3, 2, 0, 16, nil)
	writeVirtioPCICap(space, 0x74, 0x84, 4, 3, 0, 256, nil)
	writeVirtioPCICap(space, 0x84, 0, 5, 0, 0, 0, []byte{0, 0, 0, 0})
	return space
}
