package wayland

import (
	"encoding/binary"
	"strconv"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
)

// errFailed ends a client after a protocol error event.
var errFailed = errors.New("protocol error")

// maxClientID is the highest object id a client allocates. The server's ids
// start above it.
const maxClientID = 0xfeffffff

// resource is a client's binding of a global, or an object made from one,
// with the version the client bound.
type resource struct {
	// id is the object's id.
	id uint32
	// version is the bound version, which bounds the events the object
	// receives.
	version uint32
}

// global is an interface the registry advertises.
type global struct {
	// name is the interface's name.
	name string
	// version is the highest version the compositor implements.
	version uint32
	// bind creates the client's object for the binding r.
	bind func(c *client, r resource) object
}

// globals is the registry's interfaces. A global's registry name is its
// index plus one.
var globals = []global{
	{"wl_compositor", 4, (*client).bindCompositor},
	{"wl_subcompositor", 1, (*client).bindSubcompositor},
	{"wl_shm", 1, (*client).bindShm},
	{"wl_seat", 5, (*client).bindSeat},
	{"wl_output", 4, (*client).bindOutput},
	{"wl_data_device_manager", 3, (*client).bindDataDeviceManager},
	{"xdg_wm_base", 1, (*client).bindWmBase},
	{"zwlr_layer_shell_v1", 4, (*client).bindLayerShell},
	{"zxdg_output_manager_v1", 3, (*client).bindXdgOutputManager},
	{"zxdg_decoration_manager_v1", 1, (*client).bindDecorationManager},
}

// client is one connection to the compositor. Its serve goroutine reads and
// handles the requests, and its write goroutine sends the events.
type client struct {
	// comp is the compositor.
	comp *Compositor
	// conn is the connection.
	conn *kernel.UnixConn
	// objects is the client's objects by id. The serve goroutine owns it.
	objects map[uint32]object
	// files is the files received and not yet taken by a request.
	files []*kernel.File

	// pointers is the client's wl_pointer objects. The compositor's lock
	// guards it.
	pointers []resource
	// keyboards is the client's wl_keyboard objects. The compositor's lock
	// guards it.
	keyboards []resource
	// outputs is the client's wl_output objects. The compositor's lock
	// guards it.
	outputs []resource

	// outBcast guards the events not yet written and wakes the writer.
	outBcast broadcast.Broadcast
	// out is the encoded events not yet written.
	out []byte
	// outFiles is the files to send with out.
	outFiles []*kernel.File
	// closing is set when the writer should stop once out is written.
	closing bool
}

// newClient starts serving conn.
func newClient(comp *Compositor, conn *kernel.UnixConn) *client {
	c := &client{comp: comp, conn: conn, objects: make(map[uint32]object)}
	c.objects[1] = requestFunc(c.displayRequest)
	return c
}

// serve handles the client's requests until it disconnects or fails, then
// cleans up its state.
func (c *client) serve() {
	// Write the events while reading the requests.
	written := make(chan struct{})
	go func() {
		defer close(written)
		c.write()
	}()
	_ = c.read()

	// Send the last events, then disconnect.
	c.outBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.closing = true
		broadcast()
	})
	<-written
	_ = c.conn.Close()

	// Forget the client's surfaces and release its files.
	c.comp.update(func() { c.comp.remove(c) })
	for _, obj := range c.objects {
		switch obj := obj.(type) {
		case *shmPool:
			obj.unref()
		case *shmBuffer:
			obj.destroy()
		}
	}
	for _, f := range c.files {
		_ = f.Close()
	}
}

// read reads and handles requests until the connection ends or a request
// fails.
func (c *client) read() error {
	buf := make([]byte, 0, 8192)
	chunk := make([]byte, 4096)
	for {
		// Handle each whole message buffered.
		for len(buf) >= 8 {
			size := int(binary.LittleEndian.Uint16(buf[6:]))
			if size < 8 || size%4 != 0 {
				return c.fail(1, errInvalidMethod, "bad message size")
			}
			if len(buf) < size {
				break
			}
			id := binary.LittleEndian.Uint32(buf)
			op := binary.LittleEndian.Uint16(buf[4:])
			if err := c.dispatch(id, op, buf[8:size]); err != nil {
				return err
			}
			buf = buf[size:]
		}

		// Read more, keeping the partial message at the buffer's start.
		buf = append(chunk[:0:0], buf...)
		n, files, err := c.conn.ReadMsg(chunk)
		c.files = append(c.files, files...)
		if err != nil {
			return err
		}
		buf = append(buf, chunk[:n]...)
	}
}

// dispatch handles the request op to object id with the argument bytes args.
func (c *client) dispatch(id uint32, op uint16, args []byte) error {
	// Find the object.
	obj, ok := c.objects[id]
	if !ok {
		return c.fail(1, errInvalidObject, "unknown object "+strconv.FormatUint(uint64(id), 10))
	}

	// Handle the request, failing the client on bad arguments.
	m := &message{args: args, files: &c.files}
	err := obj.request(op, m)
	if m.err != nil && err == nil {
		err = c.fail(id, errInvalidMethod, m.err.Error())
	}
	return err
}

// write sends the queued events until the client closes and every event is
// written, or a write fails.
func (c *client) write() {
	for {
		// Take the queued events, or wait for some.
		var out []byte
		var files []*kernel.File
		var closing bool
		var waitCh <-chan struct{}
		c.outBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			out, files, closing = c.out, c.outFiles, c.closing
			c.out, c.outFiles = nil, nil
			waitCh = getWaitCh()
		})
		if len(out) == 0 {
			if closing {
				return
			}
			<-waitCh
			continue
		}

		// Write them with their files.
		if _, err := c.conn.WriteMsg(out, files); err != nil {
			return
		}
	}
}

// send queues an event.
func (c *client) send(e *event) {
	c.sendFile(e, nil)
}

// sendFile queues an event and a file it carries. The caller keeps its
// reference to f, which must stay open until the write.
func (c *client) sendFile(e *event, f *kernel.File) {
	c.outBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.out = append(c.out, e.bytes()...)
		if f != nil {
			c.outFiles = append(c.outFiles, f)
		}
		broadcast()
	})
}

// fail sends a protocol error on object id and returns errFailed, which
// ends the client.
func (c *client) fail(id, code uint32, msg string) error {
	c.send(newEvent(1, displayError).uint(id).uint(code).string(msg))
	return errFailed
}

// add registers a new object under the id the client chose.
func (c *client) add(id uint32, obj object) error {
	if _, ok := c.objects[id]; ok || id == 0 || id > maxClientID {
		return c.fail(1, errInvalidObject, "bad new id "+strconv.FormatUint(uint64(id), 10))
	}
	c.objects[id] = obj
	return nil
}

// destroy forgets object id and tells the client the id is free.
func (c *client) destroy(id uint32) {
	delete(c.objects, id)
	c.send(newEvent(1, displayDeleteID).uint(id))
}

// done sends a callback's done event, which destroys it.
func (c *client) done(id, data uint32) {
	c.send(newEvent(id, callbackDone).uint(data))
	c.destroy(id)
}

// inert returns an object whose requests do nothing but the destructor op.
func (c *client) inert(id uint32, destroyOp uint16) object {
	return requestFunc(func(op uint16, _ *message) error {
		if op == destroyOp {
			c.destroy(id)
		}
		return nil
	})
}

// displayRequest handles a wl_display request.
func (c *client) displayRequest(op uint16, m *message) error {
	switch op {
	case displaySync:
		// Answer at once: every earlier request is handled.
		id := m.uint()
		if m.err == nil {
			c.done(id, 0)
		}
	case displayGetRegistry:
		// Advertise the globals.
		id := m.uint()
		if m.err != nil {
			return nil
		}
		if err := c.add(id, requestFunc(c.registryRequest)); err != nil {
			return err
		}
		for i, g := range globals {
			c.send(newEvent(id, registryGlobal).uint(uint32(i + 1)).string(g.name).uint(g.version)) //nolint:gosec // few globals.
		}
	default:
		return c.fail(1, errInvalidMethod, "bad wl_display request")
	}
	return nil
}

// registryRequest handles wl_registry.bind.
func (c *client) registryRequest(op uint16, m *message) error {
	// Read the global, its interface and version, and the new id.
	name, iface, version, id := m.uint(), m.string(), m.uint(), m.uint()
	if op != registryBind || m.err != nil {
		return nil
	}
	if name == 0 || int(name) > len(globals) {
		return c.fail(1, errInvalidObject, "bad global "+iface)
	}
	g := globals[name-1]
	if g.name != iface || version == 0 || version > g.version {
		return c.fail(1, errInvalidObject, "bad binding of "+iface)
	}

	// Bind it.
	if err := c.add(id, nil); err != nil {
		return err
	}
	c.objects[id] = g.bind(c, resource{id: id, version: version})
	return nil
}

// bindCompositor binds wl_compositor, which creates surfaces and regions.
func (c *client) bindCompositor(resource) object {
	return requestFunc(func(op uint16, m *message) error {
		id := m.uint()
		if m.err != nil {
			return nil
		}
		switch op {
		case compositorCreateSurface:
			return c.add(id, newSurface(c, id))
		case compositorCreateRegion:
			return c.add(id, &region{c: c, id: id})
		}
		return nil
	})
}

// bindSubcompositor binds wl_subcompositor, which makes subsurfaces.
func (c *client) bindSubcompositor(r resource) object {
	return requestFunc(func(op uint16, m *message) error {
		switch op {
		case subcompositorDestroy:
			c.destroy(r.id)
		case subcompositorGetSubsurface:
			id, sid, pid := m.uint(), m.uint(), m.uint()
			if m.err != nil {
				return nil
			}
			s, ok := c.objects[sid].(*surface)
			parent, parentOK := c.objects[pid].(*surface)
			if !ok || !parentOK {
				return c.fail(r.id, subcompositorErrBadSurface, "bad surface")
			}
			return c.getSubsurface(id, s, parent)
		}
		return nil
	})
}

// bindShm binds wl_shm and lists its pixel formats.
func (c *client) bindShm(r resource) object {
	c.send(newEvent(r.id, shmFormat).uint(shmFormatARGB8888))
	c.send(newEvent(r.id, shmFormat).uint(shmFormatXRGB8888))
	return requestFunc(func(op uint16, m *message) error {
		// Read the pool's id, file and size, closing the file on a bad
		// request.
		if op != shmCreatePool {
			return nil
		}
		id, file, size := m.uint(), m.fd(), m.int()
		if m.err != nil {
			if file != nil {
				_ = file.Close()
			}
			return nil
		}
		if size <= 0 {
			_ = file.Close()
			return c.fail(r.id, shmErrInvalidFD, "bad pool size")
		}
		return c.add(id, &shmPool{c: c, id: id, file: file, size: size, refs: 1})
	})
}

// bindSeat binds wl_seat, which has a pointer and a keyboard.
func (c *client) bindSeat(r resource) object {
	// Describe the seat.
	c.send(newEvent(r.id, seatCapabilities).uint(seatCapPointer | seatCapKeyboard))
	if r.version >= 2 {
		c.send(newEvent(r.id, seatName).string("seat0"))
	}

	// Hand out the devices.
	return requestFunc(func(op uint16, m *message) error {
		switch op {
		case seatGetPointer:
			id := m.uint()
			if m.err == nil {
				return c.addPointer(resource{id: id, version: r.version})
			}
		case seatGetKeyboard:
			id := m.uint()
			if m.err == nil {
				return c.addKeyboard(resource{id: id, version: r.version})
			}
		case seatGetTouch:
			id := m.uint()
			if m.err == nil {
				return c.add(id, c.inert(id, 0))
			}
		case seatRelease:
			c.destroy(r.id)
		}
		return nil
	})
}

// addPointer creates a wl_pointer and enters the hovered surface when it is
// the client's.
func (c *client) addPointer(p resource) error {
	// Register the pointer, which forgets itself on release.
	err := c.add(p.id, requestFunc(func(op uint16, _ *message) error {
		if op == pointerRelease {
			c.comp.update(func() {
				c.pointers = deleteResource(c.pointers, p.id)
			})
			c.destroy(p.id)
		}
		return nil
	}))
	if err != nil {
		return err
	}

	// Track it for input.
	c.comp.update(func() {
		c.pointers = append(c.pointers, p)
		if s := c.comp.hover; s != nil && s.c == c {
			c.comp.enterPointer(s, p)
		}
	})
	return nil
}

// addKeyboard creates a wl_keyboard, sends it the keymap and enters the
// focused surface when it is the client's.
func (c *client) addKeyboard(k resource) error {
	// Register the keyboard, which forgets itself on release.
	err := c.add(k.id, requestFunc(func(op uint16, _ *message) error {
		if op == keyboardRelease {
			c.comp.update(func() {
				c.keyboards = deleteResource(c.keyboards, k.id)
			})
			c.destroy(k.id)
		}
		return nil
	}))
	if err != nil {
		return err
	}

	// Send the keymap and the key repeat: 25 a second after 600 ms.
	c.sendFile(newEvent(k.id, keyboardKeymap).uint(keymapXKBV1).uint(uint32(len(keymap)+1)), c.comp.keymap) //nolint:gosec // a small keymap.
	if k.version >= 4 {
		c.send(newEvent(k.id, keyboardRepeatInfo).int(25).int(600))
	}

	// Track it for input.
	c.comp.update(func() {
		c.keyboards = append(c.keyboards, k)
		if s := c.comp.focus; s != nil && s.c == c {
			c.comp.enterKeyboard(s, k)
		}
	})
	return nil
}

// deleteResource removes the resource id from rs.
func deleteResource(rs []resource, id uint32) []resource {
	out := rs[:0]
	for _, r := range rs {
		if r.id != id {
			out = append(out, r)
		}
	}
	return out
}

// pointerFrame ends a group of pointer events, for a pointer bound at
// version 5 or later.
func (c *client) pointerFrame(p resource) {
	if p.version >= 5 {
		c.send(newEvent(p.id, pointerFrame))
	}
}

// bindDataDeviceManager binds wl_data_device_manager. Its devices never
// offer data, so the clipboard stays empty.
func (c *client) bindDataDeviceManager(resource) object {
	return requestFunc(func(op uint16, m *message) error {
		id := m.uint()
		if m.err != nil {
			return nil
		}
		switch op {
		case dataDeviceManagerCreateDataSource:
			return c.add(id, c.inert(id, 1))
		case dataDeviceManagerGetDataDevice:
			return c.add(id, c.inert(id, 2))
		}
		return nil
	})
}
