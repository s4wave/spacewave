package wayland

import (
	"encoding/binary"
	"image"
	"slices"

	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// background is the desktop's color behind the windows, in RGBA.
var background = [4]byte{0x2e, 0x34, 0x40, 0xff}

// cascadeStep is the offset between the corners of successive new windows,
// and cascadeSlots is how many fit before the cascade starts over.
const (
	cascadeStep  = 32
	cascadeSlots = 8
)

// mapWindow shows the toplevel s on top of the stack, numbering and placing
// it when it is new, and activates it. The caller holds the lock.
func (c *Compositor) mapWindow(s *surface) {
	// Number and place a new window on the cascade.
	x, t := s.xdg, s.xdg.toplevel
	if !t.placed {
		t.placed = true
		t.pos = c.usable.Min.Add(image.Pt(cascadeStep, cascadeStep).Mul(1 + c.placed%cascadeSlots))
		c.placed++
		c.lastWindow++
		t.windowID = c.lastWindow
	}

	// Show and activate it.
	x.mapped = true
	c.windows = append(c.windows, s)
	c.changed = true
	c.activate(s)
	c.refocusPointer()
}

// mapLayer shows the layer surface s on top of its layer and rearranges the
// layers around it. The caller holds the lock.
func (c *Compositor) mapLayer(s *surface) {
	// Show it, then place the layers and move the focus around it.
	l := s.layer
	l.mapped = true
	c.layers[l.layer] = append(c.layers[l.layer], s)
	c.arrange()
	c.changed = true
	c.refocusKeyboard()
	c.refocusPointer()
}

// unmap takes the window or layer surface s off the screen, dismisses its
// popups and moves the focus it held. A window in its own space ends that
// space and maps again on the desktop. The client must configure s again
// before s maps. The caller holds the lock.
func (c *Compositor) unmap(s *surface) {
	// Hide it and its popups.
	if !s.mapped() {
		return
	}
	c.changed = true
	for _, p := range slices.Clone(s.popups) {
		c.dismiss(p, true)
	}

	// Take a window off the stack or out of its space, or a layer surface
	// off its layer, making room for the windows.
	if x := s.xdg; x != nil {
		x.mapped, x.configured = false, false
		if x.toplevel.own != nil {
			c.leaveSpace(s)
		}
		c.windows = slices.DeleteFunc(c.windows, func(w *surface) bool { return w == s })
		c.changed = true
	} else {
		l := s.layer
		l.mapped, l.configured = false, false
		c.layers[l.layer] = slices.DeleteFunc(c.layers[l.layer], func(t *surface) bool { return t == s })
		c.arrange()
	}

	// Move the focus.
	if c.grab != nil && c.grab.w == s {
		c.grab = nil
	}
	if c.layerFocus == s {
		c.layerFocus = nil
	}
	if c.active == s {
		c.activate(nil)
		if n := len(c.windows); n != 0 {
			c.activate(c.windows[n-1])
		}
	}
	c.refocusKeyboard()
	c.refocusPointer()
}

// arrange places the mapped layer surfaces and gives the windows the area
// the exclusive zones leave. The surfaces reserving an edge go first, from
// the top layer down, each taking its edge from the area the ones before
// left. Maximized windows then fill the new usable area. The caller holds
// the lock.
func (c *Compositor) arrange() {
	// Place the surfaces reserving an edge.
	usable := c.desktop.area
	for _, layer := range slices.Backward(c.layers[:]) {
		for _, s := range layer {
			if s.layer.exclusiveEdge() != 0 {
				s.layer.place(usable)
				usable = s.layer.reserve(usable)
			}
		}
	}

	// Place the rest in the usable area, or on the whole screen for a
	// negative zone.
	for _, layer := range c.layers {
		for _, s := range layer {
			switch l := s.layer; {
			case l.exclusiveEdge() != 0:
			case l.zone < 0:
				l.place(c.desktop.area)
			default:
				l.place(usable)
			}
		}
	}

	// Fit the maximized windows to a changed usable area.
	if usable == c.usable {
		return
	}
	c.usable = usable
	for _, w := range c.windows {
		if t := w.xdg.toplevel; t.maximized && !t.fullscreen {
			r := c.maximized(t)
			t.pos, t.size = r.Min, r.Size()
			c.changed = true
			w.xdg.configure()
		}
	}
}

// raise moves the mapped window w to the top of the stack. The caller holds
// the lock.
func (c *Compositor) raise(w *surface) {
	i := slices.Index(c.windows, w)
	if i == len(c.windows)-1 {
		return
	}
	c.windows = append(slices.Delete(c.windows, i, i+1), w)
	c.changed = true
}

// moveWindow moves the window w so its geometry's corner sits at pos. The
// viewers move the textures they hold, so no pixels are sent. The caller
// holds the lock.
func (c *Compositor) moveWindow(w *surface, pos image.Point) {
	if t := w.xdg.toplevel; t.pos != pos {
		t.pos = pos
		c.changed = true
	}
}

// setState maximizes the window s to the usable area, fills the screen with
// it, or restores it, and configures it at the new size. A window in its own
// space keeps filling it. The caller holds the lock.
func (c *Compositor) setState(s *surface, maximized, fullscreen bool) {
	// Answer a window in its own space with its current state.
	t := s.xdg.toplevel
	if t.own != nil {
		if s.xdg.configured {
			s.xdg.configure()
		}
		return
	}

	// Remember the window's place before it first leaves it.
	wasNormal := !t.maximized && !t.fullscreen
	if wasNormal && (maximized || fullscreen) {
		t.restore = image.Rectangle{Min: t.pos, Max: t.pos.Add(s.xdg.geometry().Size())}
	}
	t.maximized, t.fullscreen = maximized, fullscreen

	// Fill the usable area or the screen, or go back to the remembered
	// place. The client's last image shows at the new place until it draws
	// at the new size.
	switch {
	case fullscreen:
		t.pos, t.size = c.desktop.area.Min, c.desktop.area.Size()
	case maximized:
		r := c.maximized(t)
		t.pos, t.size = r.Min, r.Size()
	case !wasNormal:
		t.pos, t.size = t.restore.Min, t.restore.Size()
	}
	c.changed = true
	if s.xdg.configured {
		s.xdg.configure()
	}
}

// maximized returns the screen rectangle of the geometry of the maximized
// window t: the usable area below its title bar. The caller holds the lock.
func (c *Compositor) maximized(t *toplevel) image.Rectangle {
	r := c.usable
	r.Min.Y = min(r.Min.Y+t.titleHeight(), r.Max.Y-1)
	return r
}

// minTop returns the least top edge of the geometry of the window w, which
// keeps its title bar or its top in the usable area. The caller holds the
// lock.
func (c *Compositor) minTop(w *surface) int {
	return c.usable.Min.Y + w.xdg.toplevel.titleHeight()
}

// rootOf returns the mapped window or layer surface whose tree holds s, or
// nil. The caller holds the lock.
func (c *Compositor) rootOf(s *surface) *surface {
	// Find the root of the tree and check it shows.
	if s == nil {
		return nil
	}
	root := s
	for root.parent != nil {
		root = root.parent
	}
	if !root.mapped() {
		return nil
	}
	return root
}

// roots returns the mapped windows and layer surfaces bottom to top: the
// background and bottom layers, the windows, then the top and overlay
// layers. The caller holds the lock.
func (c *Compositor) roots() []*surface {
	return slices.Concat(c.layers[layerBackground], c.layers[layerBottom], c.windows, c.layers[layerTop], c.layers[layerOverlay])
}

// walk calls fn on each surface of the tree of s, bottom to top, with its
// screen position at when s sits at at, until fn returns true. It reports
// whether fn did. The caller holds the lock.
func (c *Compositor) walk(s *surface, at image.Point, fn func(t *surface, at image.Point) bool) bool {
	for _, t := range s.stack {
		if t == s {
			if s.image != nil && fn(s, at) {
				return true
			}
		} else if c.walk(t, at.Add(t.sub.offset), fn) {
			return true
		}
	}
	for _, p := range s.popups {
		if c.walk(p, p.origin(), fn) {
			return true
		}
	}
	return false
}

// hitAt returns what is under the point pt of the space sp: the topmost
// surface, or else the frame part of the topmost window whose frame holds
// pt, or neither. The caller holds the lock.
func (c *Compositor) hitAt(sp *space, pt image.Point) (*surface, frameHit) {
	for _, w := range slices.Backward(c.spaceRoots(sp)) {
		// Note the last surface of the tree, the topmost, under pt.
		var hit *surface
		c.walk(w, w.origin(), func(t *surface, at image.Point) bool {
			if t.takesInput(pt.Sub(at)) {
				hit = t
			}
			return false
		})
		if hit != nil {
			return hit, frameHit{}
		}

		// Then check the window's frame.
		if w.xdg != nil {
			if f := frameAt(w, pt); f.w != nil {
				return nil, f
			}
		}
	}
	return nil, frameHit{}
}

// readScene returns what the view v of the space sp lacks as a scene
// update, or nil when it lacks nothing: the screen, layers, roots, grab and
// frames when they differ from those last sent, and the damaged areas of
// each texture, all of a new or resized one. It forgets the textures of the
// surfaces no longer shown. The caller holds the lock.
func (c *Compositor) readScene(sp *space, v *view) *s4wave_vm.WasiVmScene {
	// Describe the screen.
	u := &s4wave_vm.WasiVmScene{}
	screen := &s4wave_vm.WasiVmSceneScreen{
		Width:      uint32(sp.area.Dx()), //nolint:gosec // space sizes are small.
		Height:     uint32(sp.area.Dy()), //nolint:gosec // space sizes are small.
		Scale:      uint32(c.scale),      //nolint:gosec // at most maxScale.
		Background: binary.BigEndian.Uint32(background[:]),
	}
	if !screen.EqualVT(v.screen) {
		u.Screen, v.screen = screen, screen
	}

	// Describe each root, its frame and the layers of its tree, reading the
	// textures the view lacks.
	layers, roots := &s4wave_vm.WasiVmSceneLayers{}, &s4wave_vm.WasiVmSceneRoots{}
	frames := &s4wave_vm.WasiVmSceneFrames{}
	shown := make(map[*surface]struct{})
	for _, w := range c.spaceRoots(sp) {
		root := w.origin().Sub(sp.area.Min)
		roots.Roots = append(roots.Roots, &s4wave_vm.WasiVmSceneRoot{
			Id: w.sceneID,
			X:  int32(root.X), //nolint:gosec // screen positions are small.
			Y:  int32(root.Y), //nolint:gosec // screen positions are small.
		})
		if w.xdg != nil {
			if f := c.sceneFrame(sp, w); f != nil {
				frames.Frames = append(frames.Frames, f)
			}
		}
		c.walk(w, w.origin(), func(s *surface, at image.Point) bool {
			// Describe the layer.
			off := at.Sub(w.origin())
			layers.Layers = append(layers.Layers, &s4wave_vm.WasiVmSceneLayer{
				Id:            s.sceneID,
				Root:          w.sceneID,
				X:             int32(off.X),       //nolint:gosec // surface offsets are small.
				Y:             int32(off.Y),       //nolint:gosec // surface offsets are small.
				Width:         uint32(s.size.X),   //nolint:gosec // at most maxImageSize.
				Height:        uint32(s.size.Y),   //nolint:gosec // at most maxImageSize.
				TextureWidth:  uint32(s.pixels.X), //nolint:gosec // at most maxImageSize.
				TextureHeight: uint32(s.pixels.Y), //nolint:gosec // at most maxImageSize.
				Opaque:        s.opaque,
			})
			shown[s] = struct{}{}

			// Read all of a texture the view lacks at this size, or the
			// areas it marked.
			t := v.textures[s]
			if t == nil || t.size != s.pixels {
				t = &texture{size: s.pixels}
				t.add(image.Rectangle{Max: s.pixels})
				v.textures[s] = t
			}
			for _, r := range t.damage {
				u.Patches = append(u.Patches, readTexture(s, r))
			}
			t.damage = t.damage[:0]
			return false
		})
	}
	for s := range v.textures {
		if _, ok := shown[s]; !ok {
			delete(v.textures, s)
		}
	}

	// Send the arrangement, the positions, the grab and the frames when they
	// changed.
	if !layers.EqualVT(v.layers) {
		u.Layers, v.layers = layers, layers
	}
	if !roots.EqualVT(v.roots) {
		u.Roots, v.roots = roots, roots
	}
	if grab := c.sceneGrab(sp); !grab.EqualVT(v.grab) {
		u.Grab, v.grab = grab, grab
	}
	if !frames.EqualVT(v.frames) {
		u.Frames, v.frames = frames, frames
	}
	if u.Screen == nil && u.Layers == nil && u.Roots == nil && len(u.Patches) == 0 && u.Grab == nil && u.Frames == nil {
		return nil
	}
	return u
}

// sceneGrab returns the grab as the space sp shows it: the window move in
// progress when the window is one of its roots. The caller holds the lock.
func (c *Compositor) sceneGrab(sp *space) *s4wave_vm.WasiVmSceneGrab {
	// Check a move holds a window of the space.
	g := c.grab
	if g == nil || g.edges != 0 || !slices.Contains(c.spaceRoots(sp), g.w) {
		return &s4wave_vm.WasiVmSceneGrab{}
	}

	// Describe the move by the rule drag follows, from the window's
	// geometry corner to its root's origin and from the screen to the space.
	off := g.w.origin().Sub(g.w.xdg.toplevel.pos).Sub(sp.area.Min)
	press, root := g.pointer.Sub(sp.area.Min), g.rect.Min.Add(off)
	return &s4wave_vm.WasiVmSceneGrab{Move: &s4wave_vm.WasiVmSceneMove{
		Root:   g.w.sceneID,
		PressX: int32(press.X),               //nolint:gosec // screen positions are small.
		PressY: int32(press.Y),               //nolint:gosec // screen positions are small.
		X:      int32(root.X),                //nolint:gosec // screen positions are small.
		Y:      int32(root.Y),                //nolint:gosec // screen positions are small.
		MinY:   int32(c.minTop(g.w) + off.Y), //nolint:gosec // screen positions are small.
	}}
}

// readTexture returns the area r of the image of s, in its pixels, as a
// patch of its layer. The caller holds the lock.
func readTexture(s *surface, r image.Rectangle) *s4wave_vm.WasiVmScenePatch {
	pix := make([]byte, 4*r.Dx()*r.Dy())
	for y := range r.Dy() {
		copy(pix[4*y*r.Dx():][:4*r.Dx()], s.image[4*((r.Min.Y+y)*s.pixels.X+r.Min.X):])
	}
	return &s4wave_vm.WasiVmScenePatch{
		Layer:  s.sceneID,
		X:      uint32(r.Min.X), //nolint:gosec // inside the image.
		Y:      uint32(r.Min.Y), //nolint:gosec // inside the image.
		Width:  uint32(r.Dx()),  //nolint:gosec // inside the image.
		Height: uint32(r.Dy()),  //nolint:gosec // inside the image.
		Rgba:   pix,
	}
}

// followResize moves the window s so the edges opposite a resize grab's stay
// put as its size changes, and reports whether it moved. The caller holds
// the lock.
func (c *Compositor) followResize(s *surface) bool {
	// Find the corner the grab holds.
	g := c.grab
	if g == nil || g.w != s || g.edges == 0 {
		return false
	}
	t, size := s.xdg.toplevel, s.xdg.geometry().Size()
	pos := t.pos
	if g.edges&resizeLeft != 0 {
		pos.X = g.rect.Max.X - size.X
	}
	if g.edges&resizeTop != 0 {
		pos.Y = g.rect.Max.Y - size.Y
	}

	// Move the window there.
	if pos == t.pos {
		return false
	}
	t.pos = pos
	return true
}

// dismiss hides the popup s and the popups above it, telling the client
// when notify is set. The caller holds the lock.
func (c *Compositor) dismiss(s *surface, notify bool) {
	// Dismiss the popups on this one first.
	for _, p := range slices.Clone(s.popups) {
		c.dismiss(p, notify)
	}

	// Take it off its parent.
	c.changed = true
	s.unparent()
	c.popupGrabs = slices.DeleteFunc(c.popupGrabs, func(p *surface) bool { return p == s })
	if notify && s.xdg != nil && s.xdg.popup != nil {
		s.c.send(newEvent(s.xdg.popup.id, popupDone))
	}
	c.refocusPointer()
}
