package wayland

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// watchWindows streams the compositor's window lists, each described by
// describeWindows.
func (tc *testCompositor) watchWindows() chan string {
	lists := make(chan string, 16)
	go func() {
		_ = tc.comp.WatchWindows(tc.t.Context(), func(l *s4wave_vm.WasiVmWindowList) error {
			lists <- describeWindows(l)
			return nil
		})
	}()
	return lists
}

// describeWindows describes each window of l as its id, title and place.
func describeWindows(l *s4wave_vm.WasiVmWindowList) string {
	var parts []string
	for _, w := range l.GetWindows() {
		place := strings.TrimPrefix(w.GetPlace().String(), "WasiVmWindowPlace_")
		parts = append(parts, fmt.Sprintf("%d %q %s", w.GetId(), w.GetTitle(), place))
	}
	return strings.Join(parts, ", ")
}

// awaitWindows reads lists until one is want.
func awaitWindows(t *testing.T, lists chan string, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	var got string
	for got != want {
		select {
		case got = <-lists:
		case <-deadline:
			t.Fatalf("windows %s, want %s", got, want)
		}
	}
}

// awaitSize reads the toplevel id's configures until one has size, returning
// its states.
func (c *testClient) awaitSize(id uint32, size image.Point) []uint32 {
	c.t.Helper()
	for {
		if got, states := c.awaitStates(id); got == size {
			return states
		}
	}
}

// awaitEnd waits for the viewer's stream to end, returning its error.
func (tc *testView) awaitEnd() error {
	tc.t.Helper()
	select {
	case err := <-tc.done:
		return err
	case <-time.After(10 * time.Second):
		tc.t.Fatal("the stream did not end")
		return nil
	}
}

// TestPopOut checks a window opened in a viewer leaves the desktop, fills the
// viewer at its size, follows a resize, and returns to the top of the desktop
// when the viewer closes it.
func TestPopOut(t *testing.T) {
	// Map two overlapping windows, naming the first.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	first := c.window(48, 48, bufRed)
	c.request(newEvent(first.toplevel, toplevelSetTitle).string("first"))
	c.window(48, 48, bufGreen)
	lists := tc.watchWindows()
	awaitWindows(t, lists, `1 "first" DESKTOP, 2 "" DESKTOP`)

	// Open the first in a viewer, which configures it maximized at the
	// viewer's size.
	win, err := tc.comp.OpenWindow(1, image.Pt(60, 50))
	if err != nil {
		t.Fatal(err)
	}
	if states := c.awaitSize(first.toplevel, image.Pt(60, 50)); !slices.Contains(states, toplevelMaximized) {
		t.Fatalf("configured with states %v, want maximized", states)
	}
	awaitWindows(t, lists, `1 "first" VIEWER, 2 "" DESKTOP`)

	// The viewer shows it at the origin over the background, and the
	// desktop shows the background where it was.
	tv := newTestView(t, win.Run)
	tv.awaitPixel(image.Pt(0, 0), red)
	tv.awaitPixel(image.Pt(55, 45), background)
	tc.awaitPixel(image.Pt(cascadeStep+2, cascadeStep+2), background)

	// A resize configures it at the new size.
	win.Resize(image.Pt(70, 20))
	c.awaitSize(first.toplevel, image.Pt(70, 20))
	tv.awaitPixel(image.Pt(65, 10), background)

	// Closing the viewer returns it to the top of the desktop, ending the
	// stream, and lets the client choose its size again.
	win.Close()
	if states := c.awaitSize(first.toplevel, image.Point{}); slices.Contains(states, toplevelMaximized) {
		t.Fatalf("configured with states %v, want not maximized", states)
	}
	tc.awaitPixel(image.Pt(2*cascadeStep+4, 2*cascadeStep+4), red)
	if err := tv.awaitEnd(); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("the stream ended with %v, want %v", err, ErrWindowClosed)
	}
	awaitWindows(t, lists, `1 "first" DESKTOP, 2 "" DESKTOP`)
}

// TestPopOutUnmap checks a popped window unmapping ends its viewer's stream
// and leaves the list.
func TestPopOutUnmap(t *testing.T) {
	// Map a window and watch the list.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	w := c.window(32, 32, bufRed)
	lists := tc.watchWindows()

	// Open it in a viewer.
	win, err := tc.comp.OpenWindow(1, image.Pt(40, 30))
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()
	tv := newTestView(t, win.Run)
	tv.awaitPixel(image.Pt(0, 0), red)

	// Unmap it with a null buffer.
	c.request(newEvent(w.surface, surfaceAttach).uint(0).int(0).int(0))
	c.request(newEvent(w.surface, surfaceCommit))
	if err := tv.awaitEnd(); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("the stream ended with %v, want %v", err, ErrWindowClosed)
	}
	awaitWindows(t, lists, "")

	// Opening it again finds no window.
	if _, err := tc.comp.OpenWindow(1, image.Pt(40, 30)); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("reopening returned %v, want %v", err, ErrWindowClosed)
	}
}

// TestPopOutInput checks a key from a window's viewer activates the window
// before it is sent, and a press there reaches the window at the viewer's
// point.
func TestPopOutInput(t *testing.T) {
	// Map two windows. The second takes the keyboard.
	tc := newTestCompositor(t, 160, 120)
	c := tc.connect()
	first := c.window(32, 32, bufRed)
	second := c.window(32, 32, bufGreen)
	keyboard, pointer := c.newID(), c.newID()
	c.request(newEvent(idSeat, seatGetKeyboard).uint(keyboard))
	c.request(newEvent(idSeat, seatGetPointer).uint(pointer))
	if m := c.await(keyboard, keyboardEnter); func() uint32 { m.uint(); return m.uint() }() != second.surface {
		t.Fatal("the keyboard did not enter the second window")
	}

	// A key from the first window's viewer moves the keyboard to it.
	win, err := tc.comp.OpenWindow(1, image.Pt(40, 30))
	if err != nil {
		t.Fatal(err)
	}
	defer win.Close()
	win.Input(&s4wave_vm.WasiVmDisplayInput{Body: &s4wave_vm.WasiVmDisplayInput_Key{Key: &s4wave_vm.WasiVmKeyEvent{Keysym: 'a', Down: true}}})
	m := c.await(keyboard, keyboardEnter)
	if _, s := m.uint(), m.uint(); s != first.surface {
		t.Fatalf("the keyboard entered %d, want %d", s, first.surface)
	}

	// A press in the viewer enters the pointer at the viewer's point.
	win.Input(&s4wave_vm.WasiVmDisplayInput{Body: &s4wave_vm.WasiVmDisplayInput_Pointer{Pointer: &s4wave_vm.WasiVmPointerEvent{X: 5, Y: 6, Buttons: 1}}})
	m = c.await(pointer, pointerEnter)
	if _, s, x, y := m.uint(), m.uint(), m.int(), m.int(); s != first.surface || x != 5*256 || y != 6*256 {
		t.Fatalf("pointer entered %d at %d,%d, want %d at 5,6 in 24.8", s, x, y, first.surface)
	}
}
