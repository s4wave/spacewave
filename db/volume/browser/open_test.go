//go:build !js

package volume_browser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/volume/device"
)

// heldDevice is a memory device opened through a heldOpener. Its first List
// runs fail, which models a failure after the device opened.
type heldDevice struct {
	*device.Memory
	// o is the opener that holds the device.
	o *heldOpener
	// fail runs on the first List, when set.
	fail func() error
}

// List runs the injected failure once, then lists the files.
func (d *heldDevice) List(ctx context.Context) ([]device.File, error) {
	if fail := d.fail; fail != nil {
		d.fail = nil
		if err := fail(); err != nil {
			return nil, err
		}
	}
	return d.Memory.List(ctx)
}

// Close releases the device's hold.
func (d *heldDevice) Close() error {
	d.o.mtx.Lock()
	defer d.o.mtx.Unlock()
	d.o.held = false
	d.o.closes++
	return nil
}

// heldOpener opens one memory device for one opener at a time, as the Web
// Lock does, failing with device.ErrHeld while it is held.
type heldOpener struct {
	// mem holds the device's files across opens.
	mem *device.Memory
	// fail is the failure the next open injects after the device opens.
	fail func() error

	// mtx guards held and closes.
	mtx sync.Mutex
	// held is set while an opener holds the device.
	held bool
	// closes counts the releases.
	closes int
}

// open takes the device.
func (o *heldOpener) open(context.Context, string) (openedDevice, error) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	if o.held {
		return nil, device.ErrHeld
	}
	o.held = true
	fail := o.fail
	o.fail = nil
	return &heldDevice{Memory: o.mem, o: o, fail: fail}, nil
}

// TestOpenVolumeFailureReleasesDevice fails an open after the device opened,
// by an error and by a panic, and checks that the failure surfaces with its
// text, the device is released exactly once, and the next open succeeds.
func TestOpenVolumeFailureReleasesDevice(t *testing.T) {
	failure := errors.New("injected open failure")
	for _, tc := range []struct {
		name string
		fail func() error
	}{
		{name: "error", fail: func() error { return failure }},
		{name: "panic", fail: func() error { panic("injected open panic") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Fail the first open after the device opened.
			ctx := t.Context()
			o := &heldOpener{mem: device.NewMemory(), fail: tc.fail}
			conf := &Config{Name: "test"}
			open := func() (vol *Volume, recovered any, err error) {
				defer func() { recovered = recover() }()
				vol, err = openVolume(ctx, conf, o.open, nil)
				return vol, nil, err
			}
			vol, recovered, err := open()
			switch {
			case vol != nil:
				t.Fatal("failed open returned a volume")
			case tc.name == "error" && !errors.Is(err, failure):
				t.Fatalf("open error = %v, want the injected failure", err)
			case tc.name == "panic" && !strings.Contains(errText(recovered), "injected open panic"):
				t.Fatalf("open panic = %v, want the injected panic's text", recovered)
			}
			if o.held || o.closes != 1 {
				t.Fatalf("after the failed open: held=%v closes=%d, want released once", o.held, o.closes)
			}

			// The next open takes the device and builds the volume.
			vol, recovered, err = open()
			if err != nil || recovered != nil || vol == nil {
				t.Fatalf("second open: err=%v panic=%v", err, recovered)
			}
			if err := vol.Close(); err != nil {
				t.Fatal(err)
			}
			if o.held || o.closes != 2 {
				t.Fatalf("after close: held=%v closes=%d, want released twice", o.held, o.closes)
			}
		})
	}
}

// errText returns the text of a recovered panic value.
func errText(r any) string {
	if err, ok := r.(error); ok {
		return err.Error()
	}
	return ""
}
