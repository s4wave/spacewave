// Package display is a host X client of the X server running in a WasiVM. It
// streams the screen as the server reports changes and injects pointer and key
// input, so a viewer outside the guest shows and drives the desktop.
package display

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"slices"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/damage"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
	"github.com/pkg/errors"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// SocketPath is the Unix socket of X display 0 in the guest.
const SocketPath = "/tmp/.X11-unix/X0"

// pointerButtons is the number of X pointer buttons the client presses: the
// left, middle and right buttons and the two scroll directions.
const pointerButtons = 5

// Client is an X client of one screen. It reads the screen's changed regions
// and injects input with the XTEST extension.
type Client struct {
	conn   *xgb.Conn
	root   xproto.Window
	width  uint16
	height uint16
	damage damage.Damage
	// damaged receives the damage each subtraction clears.
	damaged xfixes.Region

	// keycodes maps each keysym of the keyboard mapping to its keycode.
	keycodes map[xproto.Keysym]xproto.Keycode
	// buttons is the mask of held buttons, which Pointer changes. Input
	// comes from one goroutine.
	buttons uint32
}

// New starts an X client over conn, a connection to the X server's socket,
// and starts tracking damage on the first screen. The screen must be 24 or
// 32 bits deep with 32-bit little-endian pixels, as Xvfb's are.
func New(conn net.Conn) (*Client, error) {
	// Connect with the client's own cookie. The guest's X server grants
	// local clients without authorization, and the cookie keeps xgb from
	// sending the host user's X authority to the guest.
	cookie := make([]byte, 16)
	_, _ = rand.Read(cookie)
	xc, err := xgb.NewConnNetWithCookieHex(conn, hex.EncodeToString(cookie))
	if err != nil {
		return nil, errors.Wrap(err, "connect to the X server")
	}
	c, err := newClient(xc)
	if err != nil {
		xc.Close()
		return nil, err
	}
	return c, nil
}

// newClient checks the screen's pixel format, starts the extensions and
// creates the screen's damage tracker.
func newClient(xc *xgb.Conn) (*Client, error) {
	// Check that a screen pixel is 32 bits in little-endian order.
	setup := xproto.Setup(xc)
	screen := setup.DefaultScreen(xc)
	if setup.ImageByteOrder != xproto.ImageOrderLSBFirst || pixelBits(setup, screen.RootDepth) != 32 {
		return nil, errors.Errorf("unsupported screen format: depth %d", screen.RootDepth)
	}

	// Start the extensions. DAMAGE uses XFIXES regions, and each extension
	// takes its version before its first request.
	for _, ext := range []struct {
		name  string
		start func() error
	}{
		{"XFIXES", func() error {
			if err := xfixes.Init(xc); err != nil {
				return err
			}
			_, err := xfixes.QueryVersion(xc, 5, 0).Reply()
			return err
		}},
		{"DAMAGE", func() error {
			if err := damage.Init(xc); err != nil {
				return err
			}
			_, err := damage.QueryVersion(xc, 1, 1).Reply()
			return err
		}},
		{"XTEST", func() error {
			if err := xtest.Init(xc); err != nil {
				return err
			}
			if _, err := xtest.GetVersion(xc, 2, 2).Reply(); err != nil {
				return err
			}

			// Keep serving this client while another grabs the server,
			// as a window manager does for a move. Otherwise the button
			// release that ends the move never reaches the server.
			return xtest.GrabControlChecked(xc, true).Check()
		}},
	} {
		if err := ext.start(); err != nil {
			return nil, errors.Wrap(err, ext.name)
		}
	}

	// Track the screen's damage, which the server reports each time its
	// bounding box grows, and create the region a subtraction fills.
	d, err := damage.NewDamageId(xc)
	if err != nil {
		return nil, err
	}
	if err := damage.CreateChecked(xc, d, xproto.Drawable(screen.Root), damage.ReportLevelBoundingBox).Check(); err != nil {
		return nil, errors.Wrap(err, "create the damage tracker")
	}
	damaged, err := xfixes.NewRegionId(xc)
	if err != nil {
		return nil, err
	}
	if err := xfixes.CreateRegionChecked(xc, damaged, nil).Check(); err != nil {
		return nil, errors.Wrap(err, "create the damage region")
	}

	// Read the keyboard mapping.
	keycodes, err := readKeycodes(xc, setup)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:     xc,
		root:     screen.Root,
		width:    screen.WidthInPixels,
		height:   screen.HeightInPixels,
		damage:   d,
		damaged:  damaged,
		keycodes: keycodes,
	}, nil
}

// pixelBits returns the bits per pixel of the pixmap format of depth, or 0.
func pixelBits(setup *xproto.SetupInfo, depth byte) byte {
	for _, f := range setup.PixmapFormats {
		if f.Depth == depth {
			return f.BitsPerPixel
		}
	}
	return 0
}

// readKeycodes maps each keysym of the keyboard mapping to the first keycode
// that produces it, in any column.
func readKeycodes(xc *xgb.Conn, setup *xproto.SetupInfo) (map[xproto.Keysym]xproto.Keycode, error) {
	// Read the mapping of every keycode.
	count := byte(setup.MaxKeycode - setup.MinKeycode + 1)
	reply, err := xproto.GetKeyboardMapping(xc, setup.MinKeycode, count).Reply()
	if err != nil {
		return nil, errors.Wrap(err, "read the keyboard mapping")
	}

	// Index it by keysym, keeping the lowest keycode. Each keycode has a row
	// of keysyms.
	keycodes := make(map[xproto.Keysym]xproto.Keycode)
	code := setup.MinKeycode
	for row := range slices.Chunk(reply.Keysyms, int(reply.KeysymsPerKeycode)) {
		for _, sym := range row {
			if _, ok := keycodes[sym]; sym != 0 && !ok {
				keycodes[sym] = code
			}
		}
		code++
	}
	return keycodes, nil
}

// Run sends the screen to send as a scene of one opaque layer, then each
// region that changes as a patch of the layer, until the X server closes the
// connection or ctx ends. Each region is the bounding box of the changes since
// the last one.
func (c *Client) Run(ctx context.Context, send func(*s4wave_vm.WasiVmScene) error) error {
	// End the connection with ctx, which ends the wait for events.
	stop := context.AfterFunc(ctx, c.conn.Close)
	defer stop()

	// Send the scene with the whole screen.
	w, h := uint32(c.width), uint32(c.height)
	scene := &s4wave_vm.WasiVmScene{
		Screen: &s4wave_vm.WasiVmSceneScreen{Width: w, Height: h, Scale: 1, Background: 0x000000ff},
		Layers: &s4wave_vm.WasiVmSceneLayers{Layers: []*s4wave_vm.WasiVmSceneLayer{{
			Id: screenLayer, Root: screenLayer, Width: w, Height: h, TextureWidth: w, TextureHeight: h, Opaque: true,
		}}},
		Roots: &s4wave_vm.WasiVmSceneRoots{Roots: []*s4wave_vm.WasiVmSceneRoot{{Id: screenLayer}}},
	}
	patch, err := c.readRegion(xproto.Rectangle{Width: c.width, Height: c.height})
	if err != nil {
		return err
	}
	scene.Patches = append(scene.Patches, patch)
	if err := send(scene); err != nil {
		return err
	}

	// Send each damaged region.
	for {
		if err := c.sendDamage(send); err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			return err
		}
	}
}

// sendDamage waits for damage, clears it and sends the bounding box of the
// cleared region. The cleared region, not the events, bounds what to read, so
// damage that lands after the last event is sent too.
func (c *Client) sendDamage(send func(*s4wave_vm.WasiVmScene) error) error {
	// Wait for the first event, then discard those queued behind it.
	ev, xerr := c.conn.WaitForEvent()
	if ev == nil && xerr == nil {
		return net.ErrClosed
	}
	for ev != nil || xerr != nil {
		if xerr != nil {
			return xerr
		}
		ev, xerr = c.conn.PollForEvent()
	}

	// Clear the damage before reading the screen, so changes made during
	// the read are reported again.
	damage.Subtract(c.conn, c.damage, xfixes.Region(0), c.damaged)
	region, err := xfixes.FetchRegion(c.conn, c.damaged).Reply()
	if err != nil {
		return errors.Wrap(err, "read the damage")
	}
	patch, err := c.readRegion(region.Extents)
	if err != nil || patch == nil {
		return err
	}
	return send(&s4wave_vm.WasiVmScene{Patches: []*s4wave_vm.WasiVmScenePatch{patch}})
}

// screenLayer is the id of the scene's one layer and its root: the screen.
const screenLayer = 1

// readRegion reads the screen inside area as a patch of the screen's layer,
// or returns nil when the area misses the screen.
func (c *Client) readRegion(area xproto.Rectangle) (*s4wave_vm.WasiVmScenePatch, error) {
	// Clip the area to the screen.
	area = clip(area, c.width, c.height)
	if area.Width == 0 || area.Height == 0 {
		return nil, nil
	}

	// Read its pixels, which arrive as blue, green, red and padding, and
	// reorder them as red, green, blue and an opaque alpha.
	img, err := xproto.GetImage(c.conn, xproto.ImageFormatZPixmap, xproto.Drawable(c.root),
		area.X, area.Y, area.Width, area.Height, ^uint32(0)).Reply()
	if err != nil {
		return nil, errors.Wrap(err, "read the screen")
	}
	pix := img.Data
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2], pix[i+3] = pix[i+2], pix[i], 0xff
	}
	return &s4wave_vm.WasiVmScenePatch{
		Layer:  screenLayer,
		X:      uint32(area.X), //nolint:gosec // clipped to the screen.
		Y:      uint32(area.Y), //nolint:gosec // clipped to the screen.
		Width:  uint32(area.Width),
		Height: uint32(area.Height),
		Rgba:   pix,
	}, nil
}

// clip returns the part of r inside a screen of width by height.
func clip(r xproto.Rectangle, width, height uint16) xproto.Rectangle {
	// Take the inner edges of the region and the screen.
	x0, y0 := max(int32(r.X), 0), max(int32(r.Y), 0)
	x1 := min(int32(r.X)+int32(r.Width), int32(width))
	y1 := min(int32(r.Y)+int32(r.Height), int32(height))
	if x1 <= x0 || y1 <= y0 {
		return xproto.Rectangle{}
	}
	return xproto.Rectangle{X: int16(x0), Y: int16(y0), Width: uint16(x1 - x0), Height: uint16(y1 - y0)} //nolint:gosec // inside the screen.
}

// Input injects a viewer's pointer or key event.
func (c *Client) Input(in *s4wave_vm.WasiVmDisplayInput) {
	switch {
	case in.GetPointer() != nil:
		c.pointer(in.GetPointer())
	case in.GetKey() != nil:
		c.key(in.GetKey())
	}
}

// pointer moves the pointer, then presses and releases the buttons whose
// state changed.
func (c *Client) pointer(ev *s4wave_vm.WasiVmPointerEvent) {
	// Move the pointer.
	x, y := clampInt16(ev.GetX()), clampInt16(ev.GetY())
	xtest.FakeInput(c.conn, xproto.MotionNotify, 0, 0, c.root, x, y, 0)

	// Press and release the buttons.
	buttons := ev.GetButtons()
	for b := range pointerButtons {
		bit := uint32(1) << b
		if (buttons^c.buttons)&bit == 0 {
			continue
		}
		typ := byte(xproto.ButtonRelease)
		if buttons&bit != 0 {
			typ = xproto.ButtonPress
		}
		xtest.FakeInput(c.conn, typ, byte(b+1), 0, c.root, x, y, 0)
	}
	c.buttons = buttons & (1<<pointerButtons - 1)
}

// key presses or releases the key that produces a keysym. A keysym the
// keyboard mapping lacks is dropped.
func (c *Client) key(ev *s4wave_vm.WasiVmKeyEvent) {
	// Find the key.
	code, ok := c.keycodes[xproto.Keysym(ev.GetKeysym())]
	if !ok {
		return
	}

	// Press or release it.
	typ := byte(xproto.KeyRelease)
	if ev.GetDown() {
		typ = xproto.KeyPress
	}
	xtest.FakeInput(c.conn, typ, byte(code), 0, c.root, 0, 0, 0)
}

// clampInt16 limits v to the range of an X coordinate.
func clampInt16(v int32) int16 {
	return int16(max(min(v, 1<<15-1), -1<<15))
}

// Close closes the connection to the X server.
func (c *Client) Close() {
	c.conn.Close()
}
