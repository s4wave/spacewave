package wayland

import (
	"cmp"
	"context"
	"image"
	"slices"

	"github.com/pkg/errors"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// maxWindowSize bounds each side of a window's own space in screen units.
const maxWindowSize = 4096

// ErrWindowClosed ends a window's viewer stream when the window unmaps or
// returns to the desktop.
var ErrWindowClosed = errors.New("the window closed")

// space is an area surfaces show on, with the viewers drawing it: the
// desktop, or one window popped out to viewers of its own. The compositor's
// lock guards it.
type space struct {
	// area is the space's extent: the screen, or the window's size with its
	// window geometry's corner at the origin.
	area image.Rectangle
	// window is the window shown alone, or nil for the desktop.
	window *surface
	// viewers holds what each viewer of the space has.
	viewers map[*view]struct{}
	// held counts the viewers holding the window in the space.
	held int
	// done is set once the window left the space, which ends its viewers.
	done bool
}

// newSpace returns a space of area showing window, or the desktop for nil.
func newSpace(area image.Rectangle, window *surface) *space {
	return &space{area: area, window: window, viewers: make(map[*view]struct{})}
}

// spaceOf returns the space the tree of the root w shows on: its own space
// while popped out, else the desktop. The caller holds the lock.
func (c *Compositor) spaceOf(w *surface) *space {
	if w != nil && w.xdg != nil && w.xdg.toplevel != nil && w.xdg.toplevel.own != nil {
		return w.xdg.toplevel.own
	}
	return c.desktop
}

// spaceRoots returns the roots shown on sp, bottom to top. The caller holds
// the lock.
func (c *Compositor) spaceRoots(sp *space) []*surface {
	if sp.window != nil {
		return []*surface{sp.window}
	}
	return c.roots()
}

// Window is a viewer's hold on a window popped out of the desktop. The
// window returns to the desktop once every hold closes.
type Window struct {
	// c is the compositor.
	c *Compositor
	// sp is the window's space.
	sp *space
	// closed is set once Close runs. The compositor's lock guards it.
	closed bool
}

// OpenWindow takes the mapped window id off the desktop into a space of its
// own of size, or joins the space it shows in and resizes it.
func (c *Compositor) OpenWindow(id uint32, size image.Point) (*Window, error) {
	var w *Window
	var err error
	c.update(func() {
		// Find the window.
		i := slices.IndexFunc(c.toplevels(), func(s *surface) bool { return s.xdg.toplevel.windowID == id })
		if i < 0 {
			err = ErrWindowClosed
			return
		}
		s := c.toplevels()[i]

		// Pop it out, or hold the space it has.
		sp := s.xdg.toplevel.own
		if sp == nil {
			sp = c.popOut(s)
		}
		sp.held++
		c.resizeSpace(sp, size)
		w = &Window{c: c, sp: sp}
	})
	return w, err
}

// Input sends a viewer's pointer or key event, in window pixels, to the
// window. Either activates the window.
func (w *Window) Input(in *s4wave_vm.WasiVmDisplayInput) {
	w.c.input(w.sp, in)
}

// Resize configures the window at the viewer's new size.
func (w *Window) Resize(size image.Point) {
	w.c.update(func() {
		if !w.sp.done {
			w.c.resizeSpace(w.sp, size)
		}
	})
}

// Run streams the window's scene, then each change, until the window closes or ctx
// ends.
func (w *Window) Run(ctx context.Context, send func(*s4wave_vm.WasiVmScene) error) error {
	return w.c.run(ctx, w.sp, send)
}

// Close releases the hold, returning the window to the desktop when the hold
// was the last hold.
func (w *Window) Close() {
	w.c.update(func() {
		if w.closed {
			return
		}
		w.closed = true
		w.sp.held--
		if w.sp.held == 0 && !w.sp.done {
			w.c.popIn(w.sp.window)
		}
	})
}

// popOut moves the mapped desktop window s into a space of its own and
// returns the space. The window keeps its desktop place to return to, and is
// configured maximized at the space's size. The caller holds the lock.
func (c *Compositor) popOut(s *surface) *space {
	// Take it off the desktop and out of a grab.
	c.windows = slices.DeleteFunc(c.windows, func(w *surface) bool { return w == s })
	c.apart = append(c.apart, s)
	if c.grab != nil && c.grab.w == s {
		c.grab = nil
	}

	// Give it its space, with its geometry's corner at the origin.
	t := s.xdg.toplevel
	t.home = image.Rectangle{Min: t.pos, Max: t.pos.Add(t.size)}
	t.pos = image.Point{}
	t.own = newSpace(image.Rectangle{}, s)
	c.changed = true
	c.refocusPointer()
	return t.own
}

// resizeSpace sets the window space sp to size, within one unit and
// maxWindowSize a side, and configures its window to fill it. The caller
// holds the lock.
func (c *Compositor) resizeSpace(sp *space, size image.Point) {
	// Size the space.
	size = image.Pt(max(min(size.X, maxWindowSize), 1), max(min(size.Y, maxWindowSize), 1))
	if sp.area.Size() == size {
		return
	}
	sp.area = image.Rectangle{Max: size}
	c.changed = true

	// Fit the window to it.
	t := sp.window.xdg.toplevel
	t.size = size
	if sp.window.xdg.configured {
		sp.window.xdg.configure()
	}
}

// leaveSpace ends the space of the popped window s, which ends its viewers,
// and puts it back at its desktop place and size. The caller holds the lock
// and places the window in the desktop stack.
func (c *Compositor) leaveSpace(s *surface) {
	// End the space, moving the pointer out of it.
	t := s.xdg.toplevel
	sp := t.own
	sp.done = true
	t.own = nil
	c.apart = slices.DeleteFunc(c.apart, func(w *surface) bool { return w == s })
	if c.pointerSpace == sp {
		c.leavePointer()
		c.pointerSpace = c.desktop
	}

	// Go back to the desktop place.
	t.pos, t.size = t.home.Min, t.home.Size()
	c.changed = true
}

// popIn returns the popped window s to the top of the desktop stack at its
// desktop place, and configures it there. The caller holds the lock.
func (c *Compositor) popIn(s *surface) {
	// Put it back on top of the desktop, then tell the client and the
	// pointer.
	c.leaveSpace(s)
	c.windows = append(c.windows, s)
	c.changed = true
	s.xdg.configure()
	c.refocusPointer()
}

// toplevels returns the mapped windows: those on the desktop, bottom to top,
// then those in spaces of their own. The caller holds the lock.
func (c *Compositor) toplevels() []*surface {
	return slices.Concat(c.windows, c.apart)
}

// WatchWindows sends the list of mapped windows, then the list again each
// time it changes, until Close or ctx ends.
func (c *Compositor) WatchWindows(ctx context.Context, send func(*s4wave_vm.WasiVmWindowList) error) error {
	var last *s4wave_vm.WasiVmWindowList
	for {
		// Read the list.
		var list *s4wave_vm.WasiVmWindowList
		var closed bool
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			list, closed, waitCh = c.windowList(), c.closed, getWaitCh()
		})
		if closed {
			return nil
		}

		// Send it when it changed.
		if last == nil || !list.EqualVT(last) {
			if err := send(list); err != nil {
				return err
			}
			last = list
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// windowList returns the mapped windows in the order they first mapped. The
// caller holds the lock.
func (c *Compositor) windowList() *s4wave_vm.WasiVmWindowList {
	// List the windows by id.
	wins := c.toplevels()
	slices.SortFunc(wins, func(a, b *surface) int {
		return cmp.Compare(a.xdg.toplevel.windowID, b.xdg.toplevel.windowID)
	})

	// Describe each window.
	list := &s4wave_vm.WasiVmWindowList{}
	for _, s := range wins {
		t := s.xdg.toplevel
		place := s4wave_vm.WasiVmWindowPlace_WasiVmWindowPlace_DESKTOP
		if t.own != nil {
			place = s4wave_vm.WasiVmWindowPlace_WasiVmWindowPlace_VIEWER
		}
		list.Windows = append(list.Windows, &s4wave_vm.WasiVmWindow{
			Id:    t.windowID,
			Title: t.title,
			AppId: t.appID,
			Place: place,
		})
	}
	return list
}
