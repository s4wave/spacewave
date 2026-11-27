package wayland

import (
	"image"
	"slices"
)

// surface is a wl_surface. Its pending state builds up until commit, which
// copies the attached buffer's damaged pixels into the surface's image and
// releases the buffer. A role places it on the screen: an xdg surface is a
// window or a popup, a layer surface is part of the desktop, and a
// subsurface is drawn on its parent.
type surface struct {
	// c is the client.
	c *client
	// id is the surface's object id.
	id uint32
	// sceneID is the surface's id in the viewers' scenes.
	sceneID uint32

	// attached is set when the pending state attaches a buffer, or none.
	attached bool
	// buffer is the attached buffer, or nil to unmap.
	buffer *shmBuffer
	// damaged is the pending damage in buffer pixels.
	damaged image.Rectangle
	// scaleNext is the pending buffer scale.
	scaleNext int
	// callbacks is the pending frame callbacks.
	callbacks []uint32
	// opaqueSet is set when the pending state sets the opaque region.
	opaqueSet bool
	// opaqueNext is the pending opaque region, or nil for none.
	opaqueNext *region
	// inputSet is set when the pending state sets the input region.
	inputSet bool
	// inputNext is the pending input region, or nil for the whole surface.
	inputNext *region

	// xdg is the surface's xdg_surface role, or nil.
	xdg *xdgSurface
	// sub is the surface's wl_subsurface role, or nil.
	sub *subsurface
	// layer is the surface's zwlr_layer_surface_v1 role, or nil.
	layer *layerSurface

	// The compositor's lock guards the fields below. The client's serve
	// goroutine alone writes them, so it reads them without the lock.

	// size is the surface's size in screen units: the image's size divided
	// by its scale.
	size image.Point
	// scale is the image's pixels a side per screen unit, as the client
	// drew it.
	scale int
	// pixels is the image's size: the committed buffer's, clipped to
	// maxImageSize.
	pixels image.Point
	// image is the committed pixels in premultiplied RGBA, or nil with no
	// buffer.
	image []byte
	// entered is set once the surface was told it shows on the output.
	entered bool
	// opaqueFormat is set when the image's format has no alpha.
	opaqueFormat bool
	// opaqueRegion is the area the client promised is opaque, or nil.
	opaqueRegion *region
	// opaque is set when every pixel of the image is opaque: its format
	// has no alpha or its opaque region covers it.
	opaque bool
	// input is the area that takes pointer input, clipped to the image, or
	// nil for the whole image.
	input *region
	// parent is the surface a subsurface or a popup is drawn on, or nil.
	parent *surface
	// stack is the surface and its subsurfaces, bottom to top.
	stack []*surface
	// popups is the popups on the surface, drawn above its stack, bottom to
	// top.
	popups []*surface
}

// subsurface is the wl_subsurface role: a surface drawn at an offset on its
// parent, in the parent's stack. Its own commits apply at once, as in the
// desynchronized mode, while its position applies at the parent's commit.
type subsurface struct {
	// id is the wl_subsurface's object id.
	id uint32
	// offset is the position in the parent's coordinates. The compositor's
	// lock guards it.
	offset image.Point
	// pending is the position set for the parent's next commit.
	pending image.Point
}

// newSurface makes a surface with no role.
func newSurface(c *client, id uint32) *surface {
	s := &surface{c: c, id: id, sceneID: c.comp.lastSceneID.Add(1), scaleNext: 1, scale: 1}
	s.stack = []*surface{s}
	return s
}

// request handles a wl_surface request.
func (s *surface) request(op uint16, m *message) error {
	switch op {
	case surfaceDestroy:
		s.c.comp.update(s.detach)
		s.c.destroy(s.id)
	case surfaceAttach:
		bid, _, _ := m.uint(), m.int(), m.int()
		if m.err != nil {
			return nil
		}
		buf, ok := s.c.objects[bid].(*shmBuffer)
		if bid != 0 && !ok {
			return s.c.fail(s.id, errInvalidObject, "attached a non-buffer")
		}
		s.attached, s.buffer = true, buf
	case surfaceDamage, surfaceDamageBuffer:
		// Note the damage in buffer pixels.
		x, y, w, h := m.int(), m.int(), m.int(), m.int()
		if m.err != nil {
			return nil
		}
		r := image.Rect(int(x), int(y), int(x)+int(w), int(y)+int(h))
		if op == surfaceDamage {
			r = scaleRect(r, s.scaleNext)
		}
		s.damaged = s.damaged.Union(r)
	case surfaceSetBufferScale:
		n := m.int()
		if m.err != nil {
			return nil
		}
		if n < 1 || n > maxScale {
			return s.c.fail(s.id, surfaceErrInvalidScale, "bad buffer scale")
		}
		s.scaleNext = int(n)
	case surfaceFrame:
		id := m.uint()
		if m.err != nil {
			return nil
		}
		if err := s.c.add(id, noRequests); err != nil {
			return err
		}
		s.callbacks = append(s.callbacks, id)
	case surfaceSetOpaqueRegion, surfaceSetInputRegion:
		rid := m.uint()
		if m.err != nil {
			return nil
		}
		r, ok := s.c.objects[rid].(*region)
		if rid != 0 && !ok {
			return s.c.fail(s.id, errInvalidObject, "region argument is not a region")
		}
		if r != nil {
			r = r.clone()
		}
		if op == surfaceSetOpaqueRegion {
			s.opaqueSet, s.opaqueNext = true, r
		} else {
			s.inputSet, s.inputNext = true, r
		}
	case surfaceCommit:
		return s.commit()
	}
	return nil
}

// commit applies the pending state.
func (s *surface) commit() error {
	// Check the role's pending state.
	if s.layer != nil {
		if err := s.layer.check(); err != nil {
			return err
		}
	}

	// Read the attached buffer's damaged pixels, or all of them when its
	// size changed, then release it.
	buf := s.buffer
	if buf != nil && buf.destroyed {
		buf = nil
	}
	size, area := s.pixels, image.Rectangle{}
	var pix []byte
	if s.attached && buf != nil {
		size = image.Pt(min(buf.width, maxImageSize), min(buf.height, maxImageSize))
		area = s.damaged.Intersect(image.Rectangle{Max: size})
		if size != s.pixels || s.image == nil {
			area = image.Rectangle{Max: size}
		}
		var err error
		if pix, err = buf.read(area); err != nil {
			return s.c.fail(s.id, errImplementation, "read buffer: "+err.Error())
		}
		s.c.send(newEvent(buf.id, bufferRelease))
	}

	// Apply the state, then send the pixels the state changed, and the layout
	// when the surface moved.
	comp := s.c.comp
	comp.update(func() {
		// Note which tree the surface was in.
		w := comp.rootOf(s)

		// Apply the image and its scale.
		relayout := s.attached && (buf == nil || size != s.pixels)
		if s.attached {
			s.update(buf, size, area, pix)
		}
		if s.scale != s.scaleNext {
			s.scale, relayout = s.scaleNext, true
		}
		s.size = s.pixels.Div(s.scale)

		// Apply the regions. The image is opaque when its format or its
		// opaque region says so.
		if s.opaqueSet {
			s.opaqueRegion = s.opaqueNext
		}
		s.opaque = s.opaqueFormat || s.opaqueRegion != nil && s.opaqueRegion.covers(image.Rectangle{Max: s.size})
		if s.inputSet {
			s.input = s.inputNext
		}

		// Apply the subsurfaces' positions and the role.
		for _, t := range s.stack {
			if t.sub != nil && t.sub.offset != t.sub.pending {
				t.sub.offset, relayout = t.sub.pending, true
			}
		}
		if s.xdg != nil && s.xdg.commit() {
			relayout = true
		}
		if s.layer != nil && s.layer.commit() {
			relayout = true
		}

		// Send the committed pixels, and have the viewers compare the layout
		// when the tree shows or just stopped showing.
		if s.attached && s.image != nil {
			comp.damageTexture(s, area)
		}
		if w != nil || comp.rootOf(s) != nil {
			comp.changed = true
		}
		if relayout && w != nil && w == comp.rootOf(s) {
			comp.refocusPointer()
		}
		if s.inputSet {
			comp.refocusPointer()
		}

		// Tell a surface showing its first image that it is on the output,
		// which sets the scale the client draws at.
		if s.image != nil && !s.entered {
			s.entered = true
			s.c.enterOutputs(s)
		}
	})

	// Clear the pending state and tell the client to draw its next frame.
	s.attached, s.buffer, s.damaged = false, nil, image.Rectangle{}
	s.opaqueSet, s.opaqueNext = false, nil
	s.inputSet, s.inputNext = false, nil
	now := comp.now()
	for _, id := range s.callbacks {
		s.c.done(id, now)
	}
	s.callbacks = nil
	return nil
}

// update replaces the image when the surface lost its buffer or changed
// size, then copies pix, the pixels of area, into it. The caller holds the
// compositor's lock.
func (s *surface) update(buf *shmBuffer, size image.Point, area image.Rectangle, pix []byte) {
	// Clear or resize the image.
	switch {
	case buf == nil:
		s.pixels, s.image = image.Point{}, nil
		return
	case size != s.pixels || s.image == nil:
		s.pixels, s.image = size, make([]byte, 4*size.X*size.Y)
	}
	s.opaqueFormat = buf.opaque

	// Copy the area's rows.
	for y := range area.Dy() {
		to := 4 * ((area.Min.Y+y)*size.X + area.Min.X)
		copy(s.image[to:to+4*area.Dx()], pix[4*y*area.Dx():])
	}
}

// origin returns the screen position of the surface's top left corner. The
// caller holds the compositor's lock.
func (s *surface) origin() image.Point {
	switch {
	case s.sub != nil && s.parent != nil:
		return s.parent.origin().Add(s.sub.offset)
	case s.layer != nil:
		return s.layer.pos
	case s.xdg == nil:
		return image.Point{}
	case s.xdg.toplevel != nil:
		return s.xdg.toplevel.pos.Sub(s.xdg.geometry().Min)
	case s.xdg.popup != nil && s.parent != nil:
		at := s.parent.geometry().Min.Add(s.xdg.popup.place.Min)
		return s.parent.origin().Add(at).Sub(s.xdg.geometry().Min)
	}
	return image.Point{}
}

// takesInput reports whether the point pt in surface coordinates is in the
// surface's input region. The caller holds the compositor's lock.
func (s *surface) takesInput(pt image.Point) bool {
	if !pt.In(image.Rectangle{Max: s.size}) {
		return false
	}
	return s.input == nil || s.input.contains(pt)
}

// geometry returns the window geometry of an xdg surface, or the whole
// surface for another role, in surface coordinates. The caller holds the
// compositor's lock or is the client's serve goroutine.
func (s *surface) geometry() image.Rectangle {
	if s.xdg != nil {
		return s.xdg.geometry()
	}
	return image.Rectangle{Max: s.size}
}

// mapped reports whether the surface shows as a window or in a layer. The
// caller holds the compositor's lock.
func (s *surface) mapped() bool {
	return (s.xdg != nil && s.xdg.mapped) || (s.layer != nil && s.layer.mapped)
}

// detach takes the destroyed surface off the screen: it leaves its parent,
// its subsurfaces and popups lose theirs, and it unmaps. The caller holds
// the compositor's lock.
func (s *surface) detach() {
	// Have the viewers drop it.
	comp := s.c.comp
	comp.changed = true

	// Unmap it, leave the parent and orphan the children.
	comp.unmap(s)
	s.unparent()
	for _, t := range s.stack {
		if t != s {
			t.parent = nil
		}
	}
	for _, p := range s.popups {
		p.parent = nil
	}
	s.stack, s.popups = []*surface{s}, nil
	comp.forget(s)
	comp.refocusPointer()
}

// unparent removes the surface from its parent's stack or popups. The
// caller holds the compositor's lock.
func (s *surface) unparent() {
	if p := s.parent; p != nil {
		p.stack = slices.DeleteFunc(p.stack, func(t *surface) bool { return t == s })
		p.popups = slices.DeleteFunc(p.popups, func(t *surface) bool { return t == s })
		s.parent = nil
	}
}

// ancestor reports whether a is s or holds s in its tree of subsurfaces and
// popups. The caller holds the compositor's lock.
func (s *surface) ancestor(a *surface) bool {
	for t := s; t != nil; t = t.parent {
		if t == a {
			return true
		}
	}
	return false
}

// getSubsurface gives s the subsurface role on parent, on top of the
// parent's stack, as wl_subsurface id.
func (c *client) getSubsurface(id uint32, s, parent *surface) error {
	// Check the role and that the parent is not inside s.
	if s.sub != nil || s.xdg != nil || s.layer != nil || parent.ancestor(s) {
		return c.fail(id, subcompositorErrBadSurface, "bad subsurface")
	}

	// Add it to the parent's stack.
	if err := c.add(id, requestFunc(func(op uint16, m *message) error {
		return s.subsurfaceRequest(op, m)
	})); err != nil {
		return err
	}
	c.comp.update(func() {
		s.sub = &subsurface{id: id}
		s.parent = parent
		parent.stack = append(parent.stack, s)
	})
	return nil
}

// subsurfaceRequest handles a wl_subsurface request. Restacking applies at
// once, not at the parent's commit.
func (s *surface) subsurfaceRequest(op uint16, m *message) error {
	switch op {
	case subsurfaceDestroy:
		// Take the surface off its parent.
		id := s.sub.id
		s.c.comp.update(func() {
			// Remove it from its tree.
			s.c.comp.changed = true
			s.unparent()
			s.sub = nil
			s.c.comp.refocusPointer()
		})
		s.c.destroy(id)
	case subsurfaceSetPosition:
		x, y := m.int(), m.int()
		if m.err == nil {
			s.sub.pending = image.Pt(int(x), int(y))
		}
	case subsurfacePlaceAbove, subsurfacePlaceBelow:
		// Find the sibling: the parent or another of its subsurfaces.
		sid := m.uint()
		sibling, ok := s.c.objects[sid].(*surface)
		if m.err != nil || s.parent == nil {
			return nil
		}
		if !ok || sibling == s || !slices.Contains(s.parent.stack, sibling) {
			return s.c.fail(s.sub.id, subcompositorErrBadSurface, "bad sibling")
		}

		// Move the surface beside it.
		s.c.comp.update(func() {
			// Insert it above or below the sibling.
			p := s.parent
			p.stack = slices.DeleteFunc(p.stack, func(t *surface) bool { return t == s })
			at := slices.Index(p.stack, sibling)
			if op == subsurfacePlaceAbove {
				at++
			}
			p.stack = slices.Insert(p.stack, at, s)
			s.c.comp.changed = true
		})
	}
	return nil
}

// _ is a type assertion.
var _ object = (*surface)(nil)
