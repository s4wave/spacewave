package wayland

import (
	"image"
	"slices"
)

// xdgSurface is the xdg_surface role, which makes a surface a window or a
// popup. A new role's first commit sends its configure, and a later commit
// with a buffer maps it.
type xdgSurface struct {
	// s is the surface.
	s *surface
	// id is the xdg_surface's object id.
	id uint32
	// pendingGeometry is the window geometry set for the next commit.
	pendingGeometry image.Rectangle
	// configured is set once the role's first configure is sent.
	configured bool

	// The compositor's lock guards the fields below.

	// window is the window geometry in surface coordinates: the window
	// without its shadows, or empty for the whole surface.
	window image.Rectangle
	// mapped is set while a toplevel shows in the compositor's stack.
	mapped bool
	// toplevel is the xdg_toplevel role, or nil.
	toplevel *toplevel
	// popup is the xdg_popup role, or nil.
	popup *popup
}

// toplevel is the xdg_toplevel role: a window in the compositor's stack.
type toplevel struct {
	// x is the window's xdg_surface.
	x *xdgSurface
	// id is the xdg_toplevel's object id.
	id uint32
	// The compositor's lock guards the fields below.

	// decoration is the window's zxdg_toplevel_decoration_v1 object id, or
	// zero without one.
	decoration uint32
	// mode is the decoration mode the client asked for, or zero to take the
	// compositor's.
	mode uint32

	// title and appID name the window.
	title, appID string
	// windowID identifies the window to viewers while the window is mapped.
	// windowID is unique across the compositor's life.
	windowID uint32
	// own is the window's space while it is popped out of the desktop.
	own *space
	// home is the window's desktop rectangle while it is popped out.
	home image.Rectangle

	// minSize and maxSize bound the window's size. Zero leaves a side
	// unbounded.
	minSize, maxSize image.Point
	// pos is the screen position of the window geometry's corner.
	pos image.Point
	// placed is set once the window has a position.
	placed bool
	// size is the size last configured, or zero to let the client choose.
	size image.Point
	// maximized and fullscreen are the requested states.
	maximized, fullscreen bool
	// restore is the window's screen rectangle before it maximized or went
	// fullscreen.
	restore image.Rectangle
}

// popup is the xdg_popup role: a menu or tooltip drawn above its parent.
type popup struct {
	// id is the xdg_popup's object id.
	id uint32
	// pos is the positioner's rules, kept to place a popup that gets its
	// parent later.
	pos positioner
	// place is the popup's window geometry in the parent's window geometry
	// coordinates. The compositor's lock guards it.
	place image.Rectangle
}

// positioner is an xdg_positioner: the rules that place a popup.
type positioner struct {
	// c is the client.
	c *client
	// id is the positioner's object id.
	id uint32
	// size is the popup's size.
	size image.Point
	// anchorRect is the rectangle of the parent's window geometry the
	// popup attaches to.
	anchorRect image.Rectangle
	// anchor is the edge or corner of anchorRect to attach to.
	anchor uint32
	// gravity is the direction the popup extends from the anchor.
	gravity uint32
	// offset moves the popup from the anchor.
	offset image.Point
}

// request handles an xdg_positioner request.
func (p *positioner) request(op uint16, m *message) error {
	switch op {
	case positionerDestroy:
		p.c.destroy(p.id)
	case positionerSetSize:
		w, h := m.int(), m.int()
		if m.err == nil && (w <= 0 || h <= 0) {
			return p.c.fail(p.id, wmBaseErrInvalidPosition, "bad popup size")
		}
		p.size = image.Pt(int(w), int(h))
	case positionerSetAnchorRect:
		x, y, w, h := m.int(), m.int(), m.int(), m.int()
		if m.err == nil && (w < 0 || h < 0) {
			return p.c.fail(p.id, wmBaseErrInvalidPosition, "bad anchor rectangle")
		}
		p.anchorRect = image.Rect(int(x), int(y), int(x+w), int(y+h))
	case positionerSetAnchor:
		p.anchor = m.uint()
	case positionerSetGravity:
		p.gravity = m.uint()
	case positionerSetOffset:
		x, y := m.int(), m.int()
		p.offset = image.Pt(int(x), int(y))
	}
	return nil
}

// place returns the popup's rectangle in the parent's window geometry
// coordinates, slid to stay inside area, its space, when the parent's
// geometry sits at at.
func (p *positioner) place(at image.Point, area image.Rectangle) image.Rectangle {
	// Find the anchor point on the anchor rectangle.
	a, pt := p.anchorRect, p.anchorRect.Min
	pt.X += a.Dx() / 2
	pt.Y += a.Dy() / 2
	switch p.anchor {
	case edgeLeft, edgeTopLeft, edgeBottomLeft:
		pt.X = a.Min.X
	case edgeRight, edgeTopRight, edgeBottomRight:
		pt.X = a.Max.X
	}
	switch p.anchor {
	case edgeTop, edgeTopLeft, edgeTopRight:
		pt.Y = a.Min.Y
	case edgeBottom, edgeBottomLeft, edgeBottomRight:
		pt.Y = a.Max.Y
	}

	// Extend from it toward the gravity, centered on an axis without one.
	pt = pt.Add(p.offset)
	corner := pt.Sub(p.size.Div(2))
	switch p.gravity {
	case edgeLeft, edgeTopLeft, edgeBottomLeft:
		corner.X = pt.X - p.size.X
	case edgeRight, edgeTopRight, edgeBottomRight:
		corner.X = pt.X
	}
	switch p.gravity {
	case edgeTop, edgeTopLeft, edgeTopRight:
		corner.Y = pt.Y - p.size.Y
	case edgeBottom, edgeBottomLeft, edgeBottomRight:
		corner.Y = pt.Y
	}

	// Slide it into the area.
	r := image.Rectangle{Min: corner, Max: corner.Add(p.size)}
	on := r.Add(at)
	shift := image.Pt(max(area.Min.X-on.Min.X, min(0, area.Max.X-on.Max.X)),
		max(area.Min.Y-on.Min.Y, min(0, area.Max.Y-on.Max.Y)))
	return r.Add(shift)
}

// bindWmBase binds xdg_wm_base, which gives surfaces the xdg_surface role.
func (c *client) bindWmBase(r resource) object {
	return requestFunc(func(op uint16, m *message) error {
		switch op {
		case wmBaseDestroy:
			c.destroy(r.id)
		case wmBaseCreatePositioner:
			id := m.uint()
			if m.err == nil {
				return c.add(id, &positioner{c: c, id: id})
			}
		case wmBaseGetXdgSurface:
			// Give the surface the role.
			id, sid := m.uint(), m.uint()
			if m.err != nil {
				return nil
			}
			s, ok := c.objects[sid].(*surface)
			switch {
			case !ok:
				return c.fail(r.id, wmBaseErrInvalidSurface, "bad surface")
			case s.xdg != nil || s.sub != nil || s.layer != nil:
				return c.fail(r.id, wmBaseErrRole, "surface has a role")
			}
			x := &xdgSurface{s: s, id: id}
			if err := c.add(id, x); err != nil {
				return err
			}
			c.comp.update(func() { s.xdg = x })
		}
		return nil
	})
}

// geometry returns the window geometry in surface coordinates. The caller
// holds the compositor's lock or is the client's serve goroutine.
func (x *xdgSurface) geometry() image.Rectangle {
	if x.window.Empty() {
		return image.Rectangle{Max: x.s.size}
	}
	return x.window
}

// request handles an xdg_surface request.
func (x *xdgSurface) request(op uint16, m *message) error {
	c := x.s.c
	switch op {
	case xdgSurfaceDestroy:
		if x.toplevel != nil || x.popup != nil {
			return c.fail(x.id, wmBaseErrRole, "xdg_surface destroyed before its role")
		}
		c.destroy(x.id)
		c.comp.update(func() { x.s.xdg = nil })
	case xdgSurfaceGetToplevel:
		id := m.uint()
		if m.err != nil {
			return nil
		}
		if x.toplevel != nil || x.popup != nil {
			return c.fail(x.id, wmBaseErrRole, "xdg_surface already has a role")
		}
		t := &toplevel{x: x, id: id}
		if err := c.add(id, t); err != nil {
			return err
		}
		c.comp.update(func() { x.toplevel = t })
	case xdgSurfaceGetPopup:
		id, parentID, posID := m.uint(), m.uint(), m.uint()
		if m.err != nil {
			return nil
		}
		return x.getPopup(id, parentID, posID)
	case xdgSurfaceSetWindowGeometry:
		px, py, w, h := m.int(), m.int(), m.int(), m.int()
		if m.err != nil {
			return nil
		}
		if w <= 0 || h <= 0 {
			return c.fail(x.id, wmBaseErrInvalidSurface, "bad window geometry")
		}
		x.pendingGeometry = image.Rect(int(px), int(py), int(px+w), int(py+h))
	}
	return nil
}

// getPopup gives the surface the popup role as id, placed on the parent
// xdg_surface parentID by positioner posID.
func (x *xdgSurface) getPopup(id, parentID, posID uint32) error {
	// Find the parent and the positioner. A popup without a parent stays
	// hidden until a layer surface takes it.
	c := x.s.c
	if x.toplevel != nil || x.popup != nil {
		return c.fail(x.id, wmBaseErrRole, "xdg_surface already has a role")
	}
	pos, ok := c.objects[posID].(*positioner)
	if !ok || pos.size.X == 0 {
		return c.fail(x.id, wmBaseErrInvalidPosition, "bad positioner")
	}
	parent, ok := c.objects[parentID].(*xdgSurface)
	if parentID != 0 && !ok {
		return c.fail(x.id, wmBaseErrInvalidPopup, "bad popup parent")
	}

	// Place it on the parent.
	if err := c.add(id, requestFunc(x.popupRequest)); err != nil {
		return err
	}
	c.comp.update(func() {
		x.popup = &popup{id: id, pos: *pos}
		if parent != nil {
			x.attachPopup(parent.s)
		}
	})
	return nil
}

// attachPopup places the popup on parent, relative to the parent's window
// geometry, above its other popups. The caller holds the compositor's lock.
func (x *xdgSurface) attachPopup(parent *surface) {
	// Place it inside the parent's space, then link it.
	comp := x.s.c.comp
	at := parent.origin().Add(parent.geometry().Min)
	x.popup.place = x.popup.pos.place(at, comp.spaceOf(comp.rootOf(parent)).area)
	x.s.parent = parent
	parent.popups = append(parent.popups, x.s)
}

// commit applies the pending geometry, sends a new role its first
// configure, and maps or unmaps it to match its image. It reports whether
// the window's layout changed. The caller holds the compositor's lock.
func (x *xdgSurface) commit() bool {
	// Apply the geometry.
	comp := x.s.c.comp
	relayout := false
	if !x.pendingGeometry.Empty() && x.pendingGeometry != x.window {
		x.window, relayout = x.pendingGeometry, true
	}

	// Send the first configure.
	if !x.configured && (x.toplevel != nil || x.popup != nil) {
		x.configured = true
		x.configure()
		return relayout
	}

	// Map or unmap a toplevel, or follow a resize grab. A popup shows
	// while it has an image.
	if x.toplevel == nil {
		return relayout
	}
	switch mapped := x.s.image != nil; {
	case mapped && !x.mapped:
		comp.mapWindow(x.s)
	case !mapped && x.mapped:
		comp.unmap(x.s)
	case mapped:
		relayout = comp.followResize(x.s) || relayout
	}
	return relayout
}

// configure sends the role's configure events. The caller holds the
// compositor's lock.
func (x *xdgSurface) configure() {
	c, comp := x.s.c, x.s.c.comp
	switch {
	case x.toplevel != nil:
		t := x.toplevel
		var states []uint32
		if t.maximized || t.own != nil {
			states = append(states, toplevelMaximized)
		}
		if t.fullscreen {
			states = append(states, toplevelFullscreen)
		}
		if g := comp.grab; g != nil && g.w == x.s && g.edges != 0 {
			states = append(states, toplevelResizing)
		}
		if comp.active == x.s {
			states = append(states, toplevelActivated)
		}
		if t.decoration != 0 {
			c.send(newEvent(t.decoration, decorationConfigure).uint(t.decorationMode()))
		}
		c.send(newEvent(t.id, toplevelConfigure).
			int(int32(t.size.X)).int(int32(t.size.Y)). //nolint:gosec // window sizes are small.
			uints(states...))
	case x.popup != nil:
		r := x.popup.place
		c.send(newEvent(x.popup.id, popupConfigure).
			int(int32(r.Min.X)).int(int32(r.Min.Y)). //nolint:gosec // popup places are small.
			int(int32(r.Dx())).int(int32(r.Dy())))   //nolint:gosec // popup sizes are small.
	default:
		return
	}
	c.send(newEvent(x.id, xdgSurfaceConfigure).uint(comp.nextSerial()))
}

// request handles an xdg_toplevel request.
func (t *toplevel) request(op uint16, m *message) error {
	x := t.x
	c := x.s.c
	switch op {
	case toplevelDestroy:
		// Unmap the window, then forget the role. A decoration the client
		// keeps no longer acts, so it cannot configure a later role.
		var decoration uint32
		c.comp.update(func() {
			c.comp.unmap(x.s)
			x.toplevel, x.configured = nil, false
			decoration = t.decoration
		})
		if decoration != 0 {
			c.objects[decoration] = c.inert(decoration, decorationDestroy)
		}
		c.destroy(t.id)
	case toplevelSetTitle:
		title := m.string()
		c.comp.update(func() { t.title, c.comp.changed = title, true })
	case toplevelSetAppID:
		appID := m.string()
		c.comp.update(func() { t.appID, c.comp.changed = appID, true })
	case toplevelMove, toplevelResize:
		// Start a grab while the client's surface holds a pressed pointer.
		_, _ = m.uint(), m.uint()
		var edges uint32
		if op == toplevelResize {
			edges = m.uint()
		}
		if m.err == nil {
			c.comp.update(func() { c.comp.startGrab(x.s, edges) })
		}
	case toplevelSetMaxSize, toplevelSetMinSize:
		w, h := m.int(), m.int()
		if m.err != nil {
			return nil
		}
		c.comp.update(func() {
			if op == toplevelSetMaxSize {
				t.maxSize = image.Pt(int(w), int(h))
			} else {
				t.minSize = image.Pt(int(w), int(h))
			}
		})
	case toplevelSetMaximized, toplevelUnsetMaximized:
		c.comp.update(func() { c.comp.setState(x.s, op == toplevelSetMaximized, t.fullscreen) })
	case toplevelSetFullscreen, toplevelUnsetFullscreen:
		c.comp.update(func() { c.comp.setState(x.s, t.maximized, op == toplevelSetFullscreen) })
	}
	return nil
}

// popupRequest handles an xdg_popup request.
func (x *xdgSurface) popupRequest(op uint16, _ *message) error {
	c, p := x.s.c, x.popup
	switch op {
	case popupDestroy:
		// Hide the popup, then forget the role.
		c.comp.update(func() {
			c.comp.dismiss(x.s, false)
			x.popup, x.configured = nil, false
		})
		c.destroy(p.id)
	case popupGrab:
		// Dismiss the popup on the next press outside it.
		c.comp.update(func() {
			if x.s.parent != nil && !slices.Contains(c.comp.popupGrabs, x.s) {
				c.comp.popupGrabs = append(c.comp.popupGrabs, x.s)
			}
		})
	}
	return nil
}

// _ is a type assertion.
var (
	_ object = (*xdgSurface)(nil)
	_ object = (*toplevel)(nil)
	_ object = (*positioner)(nil)
)
