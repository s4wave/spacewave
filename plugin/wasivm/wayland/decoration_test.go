package wayland

import (
	"image"
	"slices"
	"testing"

	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// TestTitleBarMove checks a window whose client leaves the decorations to the
// desktop gets a frame in the scene, and a press on its title bar moves the
// window from the first motion with no request from the client, keeping the
// title bar on the screen.
func TestTitleBarMove(t *testing.T) {
	// Map a window that leaves the mode to the compositor, which draws the
	// decorations.
	const width = 96
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	_, mode := c.decoratedWindow(width, 32, bufRed, 0)
	if mode != decorationServerSide {
		t.Fatalf("decoration configured in mode %d, want server side", mode)
	}
	at := image.Pt(cascadeStep, cascadeStep)
	tc.awaitPixel(at, red)

	// The scene shows the active frame: the title bar above the window,
	// close at its right end and maximize beside it.
	f := tc.awaitFrame()
	if got, want := tc.frameRect(f), image.Rect(at.X, at.Y-titleHeight, at.X+width, at.Y+32); got != want {
		t.Fatalf("the frame is at %v, want %v", got, want)
	}
	if !f.GetActive() || f.GetMaximized() || f.GetTitleHeight() != titleHeight || f.GetBorder() != frameBorder {
		t.Fatalf("the frame is %v, want active, not maximized, with the desktop's title bar and band", f)
	}
	closeR := image.Rect(at.X+width-titleHeight, at.Y-titleHeight, at.X+width, at.Y)
	if got := tc.buttonRect(f, buttonClose); got != closeR {
		t.Fatalf("close is at %v, want %v", got, closeR)
	}
	if got, want := tc.buttonRect(f, buttonMaximize), closeR.Sub(image.Pt(titleHeight, 0)); got != want {
		t.Fatalf("maximize is at %v, want %v", got, want)
	}

	// Press on the title bar and move. The client never asks to move, yet
	// the window follows the first motion.
	press := image.Pt(at.X+4, at.Y-10)
	tc.pointer(press.X, press.Y, 1)
	tc.pointer(press.X+20, press.Y+10, 1)
	tc.awaitPixel(at.Add(image.Pt(20, 10)), red)
	tc.awaitPixel(at, background)

	// Moved up past the top of the screen, the title bar stops at the top.
	tc.pointer(press.X+20, 0, 1)
	tc.awaitPixel(image.Pt(at.X+20, titleHeight), red)
	tc.awaitPixel(image.Pt(at.X+20, titleHeight-1), background)
	tc.pointer(press.X+20, 0, 0)
}

// TestFrameButtons checks the frame's maximize button maximizes the window
// below its title bar and restores it, and its close button asks the client
// to close the window.
func TestFrameButtons(t *testing.T) {
	// Map a decorated window.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	w, _ := c.decoratedWindow(48, 32, bufRed, 0)
	at := image.Pt(cascadeStep, cascadeStep)
	tc.awaitPixel(at, red)

	// Click maximize. The window fills the screen below its title bar.
	tc.click(image.Pt(at.X+48-titleHeight-titleHeight/2, at.Y-titleHeight/2))
	if size := c.awaitState(w.toplevel, true); size != image.Pt(160, 120-titleHeight) {
		t.Fatalf("maximized at %v, want 160x%d", size, 120-titleHeight)
	}
	c.draw(w, 160, 120-titleHeight, bufRed)
	tc.awaitPixel(image.Pt(4, titleHeight), red)
	tc.awaitPixel(image.Pt(4, titleHeight-1), background)
	tc.awaitScene("the maximized frame", func(sc *s4wave_vm.WasiVmScene) bool {
		f := sc.GetFrames().GetFrames()
		return len(f) == 1 && f[0].GetMaximized() && f[0].GetBorder() == 0 && f[0].GetWidth() == 160
	})

	// Click it again, now near the right end of the screen. The window goes
	// back to its place and size.
	tc.click(image.Pt(160-titleHeight-titleHeight/2, titleHeight/2))
	if size := c.awaitState(w.toplevel, false); size != image.Pt(48, 32) {
		t.Fatalf("restored at %v, want 48x32", size)
	}
	c.draw(w, 48, 32, bufRed)
	tc.awaitScene("the restored frame", func(sc *s4wave_vm.WasiVmScene) bool {
		f := sc.GetFrames().GetFrames()
		return len(f) == 1 && !f[0].GetMaximized() && f[0].GetWidth() == 48
	})
	tc.awaitPixel(at, red)

	// A press on close released off it does nothing. A click on it asks
	// the client to close the window.
	closeAt := image.Pt(at.X+48-titleHeight/2, at.Y-titleHeight/2)
	tc.pointer(closeAt.X, closeAt.Y, 1)
	tc.pointer(at.X+2, closeAt.Y, 1)
	tc.pointer(at.X+2, closeAt.Y, 0)
	tc.click(closeAt)
	for {
		id, op, _ := c.next()
		if id == w.toplevel && op == toplevelClose {
			break
		}
	}
}

// TestFrameResize checks a press on the band around a frame resizes the
// window from the first motion.
func TestFrameResize(t *testing.T) {
	// Map a decorated window.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	w, _ := c.decoratedWindow(48, 32, bufRed, 0)
	at := image.Pt(cascadeStep, cascadeStep)
	tc.awaitPixel(at, red)

	// Drag its bottom right corner's band by 10,6.
	corner := at.Add(image.Pt(48+1, 32+1))
	tc.pointer(corner.X, corner.Y, 1)
	tc.pointer(corner.X+10, corner.Y+6, 1)
	for {
		size, states := c.awaitStates(w.toplevel)
		if slices.Contains(states, toplevelResizing) && size == image.Pt(58, 38) {
			break
		}
	}
	tc.pointer(corner.X+10, corner.Y+6, 0)
}

// TestClientSideDecorations checks a window whose client draws its own
// decorations gets no frame, and a press above it does not move it.
func TestClientSideDecorations(t *testing.T) {
	// Map a window that asks for client side decorations, and one without a
	// decoration object, on top of it.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	_, mode := c.decoratedWindow(48, 32, bufRed, decorationClientSide)
	if mode != decorationClientSide {
		t.Fatalf("decoration configured in mode %d, want client side", mode)
	}
	c.window(16, 16, bufGreen)

	// Both show, and the scene has no frame.
	at := image.Pt(cascadeStep, cascadeStep)
	tc.awaitPixel(at, red)
	tc.awaitPixel(at.Add(image.Pt(cascadeStep, cascadeStep)), green)
	if f := tc.scene.GetFrames().GetFrames(); len(f) != 0 {
		t.Fatalf("the scene has frames %v, want none", f)
	}

	// A drag from above the window leaves it in place.
	tc.pointer(at.X+4, at.Y-10, 1)
	tc.pointer(at.X+24, at.Y, 1)
	tc.pointer(at.X+24, at.Y, 0)
	c.roundtrip()
	tc.awaitPixel(at, red)
}

// TestOrphanedDecoration checks a decoration kept after its toplevel is
// destroyed no longer acts, so it cannot configure the surface's next
// toplevel.
func TestOrphanedDecoration(t *testing.T) {
	// Map a decorated window, then destroy its toplevel and keep the
	// decoration.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	w := c.newWindow()
	decoration := c.newID()
	c.request(newEvent(idDecorations, decorationManagerGetToplevelDecoration).uint(decoration).uint(w.toplevel))
	c.mapWindow(w, 48, 32, bufRed, decoration)
	c.request(newEvent(w.toplevel, toplevelDestroy))

	// Give the surface a new toplevel and wait for its first configure.
	next := c.newID()
	c.request(newEvent(w.xdg, xdgSurfaceGetToplevel).uint(next))
	c.request(newEvent(w.surface, surfaceCommit))
	c.awaitStates(next)
	c.await(w.xdg, xdgSurfaceConfigure)

	// A mode set on the old decoration configures nothing.
	c.request(newEvent(decoration, decorationSetMode).uint(decorationClientSide))
	sync := c.newID()
	c.request(newEvent(1, displaySync).uint(sync))
	for {
		id, op, _ := c.next()
		if id == sync && op == callbackDone {
			break
		}
		if id == next || id == w.xdg || id == decoration {
			t.Fatalf("object %d got event %d after its old decoration set a mode", id, op)
		}
	}
}

// awaitFrame applies the viewer's updates until the scene shows one frame,
// and returns it.
func (tc *testView) awaitFrame() *s4wave_vm.WasiVmSceneFrame {
	tc.t.Helper()
	tc.awaitScene("a frame", func(sc *s4wave_vm.WasiVmScene) bool {
		return len(sc.GetFrames().GetFrames()) == 1
	})
	return tc.scene.GetFrames().GetFrames()[0]
}

// frameRect returns the screen rectangle of the frame f in the scene so far.
func (tc *testView) frameRect(f *s4wave_vm.WasiVmSceneFrame) image.Rectangle {
	root, _ := rootAt(tc.scene, f.GetRoot())
	return image.Rect(0, 0, int(f.GetWidth()), int(f.GetHeight())).Add(image.Pt(int(f.GetX()), int(f.GetY()))).Add(root)
}

// buttonRect returns the screen rectangle of the button kind on the frame f
// in the scene so far, or the empty rectangle.
func (tc *testView) buttonRect(f *s4wave_vm.WasiVmSceneFrame, kind s4wave_vm.WasiVmSceneFrameButtonKind) image.Rectangle {
	root, _ := rootAt(tc.scene, f.GetRoot())
	for _, b := range f.GetButtons() {
		if b.GetKind() == kind {
			return image.Rect(0, 0, int(b.GetWidth()), int(b.GetHeight())).Add(image.Pt(int(b.GetX()), int(b.GetY()))).Add(root)
		}
	}
	return image.Rectangle{}
}

// click presses and releases the pointer at pt.
func (tc *testCompositor) click(pt image.Point) {
	tc.pointer(pt.X, pt.Y, 1)
	tc.pointer(pt.X, pt.Y, 0)
}

// awaitState reads events until toplevel id is configured maximized, or not
// maximized, and returns the size.
func (c *testClient) awaitState(id uint32, maximized bool) image.Point {
	c.t.Helper()
	for {
		if size, states := c.awaitStates(id); slices.Contains(states, toplevelMaximized) == maximized {
			return size
		}
	}
}

// draw draws the window w at width by height in one opaque color.
func (c *testClient) draw(w testWindow, width, height int, bgr [3]byte) {
	// Attach a buffer of the color, damage all of it and commit.
	c.t.Helper()
	bid := c.buffer(width, height, [4]byte{bgr[0], bgr[1], bgr[2], 0}, shmFormatXRGB8888)
	c.request(newEvent(w.surface, surfaceAttach).uint(bid).int(0).int(0))
	c.request(newEvent(w.surface, surfaceDamage).int(0).int(0).int(int32(width)).int(int32(height))) //nolint:gosec // small windows.
	c.request(newEvent(w.surface, surfaceCommit))
}
