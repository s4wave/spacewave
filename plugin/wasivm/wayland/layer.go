package wayland

import (
	"image"
	"slices"
)

// layerSurface is the zwlr_layer_surface_v1 role: a desktop surface, such as
// a wallpaper, a panel or a launcher, in a layer below or above the windows.
// The compositor places it by its anchors and margins, and an exclusive zone
// keeps the windows off the edge it holds.
type layerSurface struct {
	// s is the surface.
	s *surface
	// id is the layer surface's object id.
	id uint32
	// pending is the state set for the next commit.
	pending layerState

	// The compositor's lock guards the fields below.

	// layerState is the committed state.
	layerState
	// configured is set once the role's first configure is sent.
	configured bool
	// mapped is set while the surface shows in its layer.
	mapped bool
	// size is the size last configured.
	size image.Point
	// pos is the screen position of the surface's corner.
	pos image.Point
}

// layerState is a layer surface's double-buffered state.
type layerState struct {
	// layer is the layer the surface shows in.
	layer uint32
	// size is the requested size. A zero side spans the area between its
	// anchors.
	size image.Point
	// anchor is the mask of the area's edges the surface holds to.
	anchor uint32
	// zone is the exclusive zone: positive to reserve that much of the
	// anchored edge, zero to stay off the reserved edges, and negative to
	// span the whole screen.
	zone int
	// margin is the distance kept from each anchored edge.
	margin margins
	// interactivity is when the surface takes the keyboard.
	interactivity uint32
}

// margins is the space kept from each edge of an area.
type margins struct {
	// top, right, bottom and left are the space at each edge.
	top, right, bottom, left int
}

// bindLayerShell binds zwlr_layer_shell_v1, which gives surfaces the layer
// surface role.
func (c *client) bindLayerShell(r resource) object {
	return requestFunc(func(op uint16, m *message) error {
		switch op {
		case layerShellDestroy:
			c.destroy(r.id)
		case layerShellGetLayerSurface:
			// Read the surface and its layer. The one output serves every
			// surface.
			id, sid, _, layer, _ := m.uint(), m.uint(), m.uint(), m.uint(), m.string()
			if m.err != nil {
				return nil
			}
			s, ok := c.objects[sid].(*surface)
			switch {
			case !ok:
				return c.fail(r.id, errInvalidObject, "bad surface")
			case s.xdg != nil || s.sub != nil || s.layer != nil || s.image != nil:
				return c.fail(r.id, layerShellErrRole, "surface has a role or a buffer")
			case layer >= layerCount:
				return c.fail(r.id, layerShellErrInvalidLayer, "bad layer")
			}

			// Give the surface the role.
			l := &layerSurface{s: s, id: id, pending: layerState{layer: layer}}
			if err := c.add(id, l); err != nil {
				return err
			}
			c.comp.update(func() {
				l.layerState = l.pending
				s.layer = l
			})
		}
		return nil
	})
}

// request handles a zwlr_layer_surface_v1 request.
func (l *layerSurface) request(op uint16, m *message) error {
	c := l.s.c
	switch op {
	case layerSurfaceSetSize:
		w, h := m.uint(), m.uint()
		l.pending.size = image.Pt(int(w), int(h))
	case layerSurfaceSetAnchor:
		a := m.uint()
		if a > anchorTop|anchorBottom|anchorLeft|anchorRight {
			return c.fail(l.id, layerSurfaceErrInvalidAnchor, "bad anchor")
		}
		l.pending.anchor = a
	case layerSurfaceSetExclusiveZone:
		l.pending.zone = int(m.int())
	case layerSurfaceSetMargin:
		top, right, bottom, left := m.int(), m.int(), m.int(), m.int()
		l.pending.margin = margins{top: int(top), right: int(right), bottom: int(bottom), left: int(left)}
	case layerSurfaceSetKeyboardInteractivity:
		k := m.uint()
		if k > interactivityOnDemand {
			return c.fail(l.id, layerSurfaceErrInvalidKeyboardInteractivity, "bad keyboard interactivity")
		}
		l.pending.interactivity = k
	case layerSurfaceGetPopup:
		id := m.uint()
		if m.err == nil {
			l.getPopup(id)
		}
	case layerSurfaceDestroy:
		// Unmap the surface, then forget the role.
		c.comp.update(func() {
			c.comp.unmap(l.s)
			l.s.layer = nil
		})
		c.destroy(l.id)
	case layerSurfaceSetLayer:
		layer := m.uint()
		if layer >= layerCount {
			return c.fail(l.id, layerShellErrInvalidLayer, "bad layer")
		}
		l.pending.layer = layer
	}
	return nil
}

// getPopup places the parentless xdg_popup id on the surface.
func (l *layerSurface) getPopup(id uint32) {
	for _, o := range l.s.c.objects {
		if x, ok := o.(*xdgSurface); ok && x.popup != nil && x.popup.id == id && x.s.parent == nil {
			l.s.c.comp.update(func() { x.attachPopup(l.s) })
			return
		}
	}
}

// check returns the protocol error the pending state makes at commit: a side
// left to the compositor must lie between opposite anchors.
func (l *layerSurface) check() error {
	// Check each side left to the compositor spans its axis.
	p := l.pending
	spansX := p.anchor&(anchorLeft|anchorRight) == anchorLeft|anchorRight
	spansY := p.anchor&(anchorTop|anchorBottom) == anchorTop|anchorBottom
	if (p.size.X == 0 && !spansX) || (p.size.Y == 0 && !spansY) {
		return l.s.c.fail(l.id, layerSurfaceErrInvalidSize, "a zero side needs opposite anchors")
	}
	return nil
}

// commit applies the pending state, sends a new role its first configure, and
// maps, unmaps or rearranges the surface to match its image. It reports
// whether the surface's layout changed. The caller holds the compositor's
// lock.
func (l *layerSurface) commit() bool {
	// Apply the state, moving a mapped surface to its new layer.
	comp := l.s.c.comp
	changed := l.pending != l.layerState
	if l.mapped && l.pending.layer != l.layer {
		comp.layers[l.layer] = slices.DeleteFunc(comp.layers[l.layer], func(t *surface) bool { return t == l.s })
		comp.layers[l.pending.layer] = append(comp.layers[l.pending.layer], l.s)
	}
	l.layerState = l.pending

	// Send the first configure, sized for the area the surface would take.
	if !l.configured {
		area := comp.usable
		if l.zone < 0 {
			area = comp.desktop.area
		}
		l.place(area)
		return changed
	}

	// Map, unmap or rearrange it.
	switch mapped := l.s.image != nil; {
	case mapped && !l.mapped:
		comp.mapLayer(l.s)
	case !mapped && l.mapped:
		comp.unmap(l.s)
	case mapped:
		comp.arrange()
	}
	return changed
}

// exclusiveEdge returns the edge the surface reserves: the one it anchors to
// alone or between the two beside it, when its zone is positive. It returns
// zero for none. The caller holds the compositor's lock.
func (l *layerSurface) exclusiveEdge() uint32 {
	if l.zone <= 0 {
		return 0
	}
	switch l.anchor {
	case anchorTop, anchorTop | anchorLeft | anchorRight:
		return anchorTop
	case anchorBottom, anchorBottom | anchorLeft | anchorRight:
		return anchorBottom
	case anchorLeft, anchorLeft | anchorTop | anchorBottom:
		return anchorLeft
	case anchorRight, anchorRight | anchorTop | anchorBottom:
		return anchorRight
	}
	return 0
}

// reserve returns area without the surface's exclusive zone and its margin
// on the edge it reserves. The caller holds the compositor's lock.
func (l *layerSurface) reserve(area image.Rectangle) image.Rectangle {
	switch l.exclusiveEdge() {
	case anchorTop:
		area.Min.Y += l.zone + l.margin.top
	case anchorBottom:
		area.Max.Y -= l.zone + l.margin.bottom
	case anchorLeft:
		area.Min.X += l.zone + l.margin.left
	case anchorRight:
		area.Max.X -= l.zone + l.margin.right
	}
	return area
}

// place sizes the surface for area, configuring it when the size changes,
// and moves it to its anchors there. The caller holds the compositor's lock.
func (l *layerSurface) place(area image.Rectangle) {
	// Span a side left to the compositor across the area inside the margins.
	m := l.margin
	in := image.Rect(area.Min.X+m.left, area.Min.Y+m.top, area.Max.X-m.right, area.Max.Y-m.bottom)
	size := l.layerState.size
	if size.X == 0 {
		size.X = max(in.Dx(), 1)
	}
	if size.Y == 0 {
		size.Y = max(in.Dy(), 1)
	}

	// Configure the new size.
	c := l.s.c
	if !l.configured || size != l.size {
		l.configured, l.size = true, size
		c.send(newEvent(l.id, layerSurfaceConfigure).uint(c.comp.nextSerial()).
			uint(uint32(size.X)).uint(uint32(size.Y))) //nolint:gosec // positive sizes.
	}

	// Hold the drawn surface to its anchors, centering it on an axis
	// anchored at neither or both ends.
	drawn := size
	if l.s.image != nil {
		drawn = l.s.size
	}
	pos := in.Min.Add(in.Size().Sub(drawn).Div(2))
	switch l.anchor & (anchorLeft | anchorRight) {
	case anchorLeft:
		pos.X = in.Min.X
	case anchorRight:
		pos.X = in.Max.X - drawn.X
	}
	switch l.anchor & (anchorTop | anchorBottom) {
	case anchorTop:
		pos.Y = in.Min.Y
	case anchorBottom:
		pos.Y = in.Max.Y - drawn.Y
	}

	// Move it there.
	if pos != l.pos {
		l.pos = pos
		c.comp.changed = true
	}
}

// _ is a type assertion.
var _ object = (*layerSurface)(nil)
