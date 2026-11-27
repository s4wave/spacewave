// Package wayland is a Wayland compositor for the clients in a WasiVM. It
// listens on a socket in the guest's filesystem, reads each client's shared
// memory buffers through the kernel, streams the screen to viewers outside
// the guest and turns their pointer and key input into Wayland events.
//
// The compositor is a stacking window manager. Windows open on a cascade at
// the size their clients choose and show over a plain background with their
// subsurfaces and popups. A window whose client asks for server side
// decorations gets a frame the viewers draw: a title bar with close and
// maximize buttons, and a band around it that resizes the window. A press
// raises and activates the window under the pointer, which then holds the
// keyboard. A press on a title bar, or a client's move or resize request,
// drags the window with the pointer. Layer shell clients draw the rest of
// the desktop, such as a wallpaper, a panel or a launcher, below or above
// the windows, and an exclusive zone keeps the windows off a panel's edge.
package wayland

import (
	"context"
	"image"
	"slices"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// SocketPath is the compositor's socket in the guest. A client finds it with
// XDG_RUNTIME_DIR=/run and WAYLAND_DISPLAY=wayland-0.
const SocketPath = "/run/wayland-0"

// Compositor serves the Wayland clients of one WasiVM kernel.
type Compositor struct {
	// listener accepts the guest's connections.
	listener *kernel.UnixListener
	// start is the origin of event timestamps.
	start time.Time
	// lastSceneID is the last scene id given to a surface. Scene ids
	// identify layers and roots to the viewers. Object ids repeat across
	// clients.
	lastSceneID atomic.Uint32
	// keymap is the keyboard's xkbcommon keymap, sent to each keyboard.
	keymap *kernel.File

	// bcast guards the fields below and the surfaces' screen state, and
	// wakes the viewers and the window watchers when they change.
	bcast broadcast.Broadcast
	// desktop is the screen's space: the output's size in screen units,
	// with the layers and the windows not popped out.
	desktop *space
	// scale is the output's pixels a side per screen unit.
	scale int
	// clients is the connected clients.
	clients map[*client]struct{}
	// closed is set once Close runs, so no client connects after it.
	closed bool
	// connected is set once a client has connected.
	connected bool
	// serial numbers input and configure events.
	serial uint32
	// windows is the mapped toplevels on the desktop, bottom to top.
	windows []*surface
	// apart is the mapped toplevels popped out to spaces of their own.
	apart []*surface
	// lastWindow is the last id given to a window.
	lastWindow uint32
	// layers is the mapped layer surfaces of each layer, bottom to top.
	layers [layerCount][]*surface
	// usable is the screen area the exclusive zones leave to the windows.
	usable image.Rectangle
	// placed counts the windows placed on the cascade.
	placed int
	// active is the activated window, or nil.
	active *surface
	// focus is the surface holding the keyboard focus, or nil: the active
	// window, or a layer surface that takes the keyboard.
	focus *surface
	// layerFocus is the layer surface a press gave the keyboard, or nil.
	layerFocus *surface
	// hover is the surface holding the pointer focus, or nil.
	hover *surface
	// grab is the window move or resize in progress, or nil.
	grab *grab
	// framePress is the frame button the held buttons pressed, or none.
	framePress frameHit
	// popupGrabs is the popups that close on a press outside them, oldest
	// first.
	popupGrabs []*surface
	// changed is set when what the viewers or the window watchers show
	// changed under the current lock.
	changed bool
	// pointerSpace is the space the pointer is on.
	pointerSpace *space
	// pointer is the pointer's position on pointerSpace.
	pointer image.Point
	// buttons is the mask of held pointer buttons, in viewer bit order.
	buttons uint32
	// pressed is where the pointer was when the held buttons were first
	// pressed. A grab starts from it.
	pressed image.Point
	// keys is the held keys, as evdev key codes.
	keys []uint32
	// locked is the mask of locked modifiers: Caps Lock.
	locked uint32
}

// Listen starts a compositor with a screen of width by height on the
// kernel's socket at SocketPath. Serve accepts its clients.
func Listen(k *kernel.Kernel, width, height int) (*Compositor, error) {
	l, err := k.ListenUnix(SocketPath)
	if err != nil {
		return nil, errors.Wrap(err, "listen for wayland clients")
	}
	desktop := newSpace(image.Rect(0, 0, width, height), nil)
	return &Compositor{
		listener:     l,
		desktop:      desktop,
		scale:        1,
		pointerSpace: desktop,
		usable:       desktop.area,
		start:        time.Now(),
		keymap:       k.NewMemfd("keymap", append([]byte(keymap), 0)),
		clients:      make(map[*client]struct{}),
	}, nil
}

// Serve accepts and serves clients until Close or ctx ends.
func (c *Compositor) Serve(ctx context.Context) error {
	// End the accept loop with ctx.
	stop := context.AfterFunc(ctx, func() { _ = c.listener.Close() })
	defer stop()

	// Serve each connection.
	for {
		conn, err := c.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			return err
		}
		cl := newClient(c, conn)
		var closed bool
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			closed = c.closed
			if !closed {
				c.clients[cl] = struct{}{}
				c.connected = true
				broadcast()
			}
		})
		if closed {
			_ = conn.Close()
			return nil
		}
		go cl.serve()
	}
}

// Close stops listening and disconnects every client.
func (c *Compositor) Close() {
	// Stop accepting.
	_ = c.listener.Close()

	// Disconnect the clients, whose serve loops then clean up, and end the
	// waits and the viewers.
	var clients []*client
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.closed = true
		for cl := range c.clients {
			clients = append(clients, cl)
		}
		broadcast()
	})
	for _, cl := range clients {
		_ = cl.conn.Close()
	}
	_ = c.keymap.Close()
}

// WaitClient waits until a client has connected, or until Close or ctx ends.
func (c *Compositor) WaitClient(ctx context.Context) error {
	for {
		var connected, closed bool
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			connected, closed, waitCh = c.connected, c.closed, getWaitCh()
		})
		switch {
		case connected:
			return nil
		case closed:
			return errors.New("compositor closed")
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// Run sends the desktop's whole scene to send, then each change, until Close
// or ctx ends.
func (c *Compositor) Run(ctx context.Context, send func(*s4wave_vm.WasiVmScene) error) error {
	return c.run(ctx, c.desktop, send)
}

// Input sends a viewer's pointer or key event to the surfaces holding the
// focus, or sets the output's scale. A keysym the keyboard lacks is dropped.
func (c *Compositor) Input(in *s4wave_vm.WasiVmDisplayInput) {
	c.input(c.desktop, in)
}

// run sends the whole scene of the space sp to send, then each change,
// until Close, the window leaves sp, or ctx ends.
func (c *Compositor) run(ctx context.Context, sp *space, send func(*s4wave_vm.WasiVmScene) error) error {
	// Register the viewer holding nothing.
	v := newView()
	c.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		sp.viewers[v] = struct{}{}
	})
	defer c.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		delete(sp.viewers, v)
	})

	// Send what the viewer lacks each time the scene changes.
	for {
		var update *s4wave_vm.WasiVmScene
		var closed, done bool
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			closed, done, waitCh = c.closed, sp.done, getWaitCh()
			if !closed && !done {
				update = c.readScene(sp, v)
			}
		})
		switch {
		case closed:
			return nil
		case done:
			return ErrWindowClosed
		}
		if update != nil {
			if err := send(update); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// update runs fn under the lock, then wakes the viewers and the window
// watchers when fn changed what they show.
func (c *Compositor) update(fn func()) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		fn()
		if c.changed {
			c.changed = false
			broadcast()
		}
	})
}

// damageTexture marks the area r of the image of s, in its pixels, for each
// viewer of its space holding its texture to receive again. A viewer without
// it receives all of it once s shows. The caller holds the lock through
// update, which wakes the viewers.
func (c *Compositor) damageTexture(s *surface, r image.Rectangle) {
	// Skip an empty area or a surface not shown.
	r = r.Intersect(image.Rectangle{Max: s.pixels})
	w := c.rootOf(s)
	if r.Empty() || w == nil {
		return
	}

	// Mark it on each texture of the same size.
	for v := range c.spaceOf(w).viewers {
		if t := v.textures[s]; t != nil && t.size == s.pixels {
			t.add(r)
		}
	}
	c.changed = true
}

// nextSerial returns a new event serial. The caller holds the lock.
func (c *Compositor) nextSerial() uint32 {
	c.serial++
	return c.serial
}

// now returns the event timestamp: milliseconds since the start.
func (c *Compositor) now() uint32 {
	return uint32(time.Since(c.start).Milliseconds()) //nolint:gosec // wraps as Wayland timestamps do.
}

// remove forgets a disconnected client and unmaps its windows and layer
// surfaces. The caller holds the lock.
func (c *Compositor) remove(cl *client) {
	// Drop the client and the focus it held, then hide its surfaces.
	delete(c.clients, cl)
	if c.hover != nil && c.hover.c == cl {
		c.hover = nil
	}
	if c.focus != nil && c.focus.c == cl {
		c.focus = nil
	}
	for _, w := range c.roots() {
		if w.c == cl {
			c.unmap(w)
		}
	}
	c.popupGrabs = slices.DeleteFunc(c.popupGrabs, func(p *surface) bool { return p.c == cl })
}
