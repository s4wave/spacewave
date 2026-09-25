//go:build !js

package devicetest

import (
	"testing"

	"github.com/s4wave/spacewave/db/volume/device"
)

// TestDevices checks the Device contract on the native implementations.
func TestDevices(t *testing.T) {
	devices := map[string]func(t *testing.T) device.Device{
		"memory": func(*testing.T) device.Device { return device.NewMemory() },
		"dir": func(t *testing.T) device.Device {
			d, err := device.OpenDir(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = d.Close() })
			return d
		},
	}
	for name, open := range devices {
		t.Run(name, func(t *testing.T) {
			if err := Check(t.Context(), open(t)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDirReopen checks that flushed files survive reopening a directory.
func TestDirReopen(t *testing.T) {
	root := t.TempDir()
	d, err := device.OpenDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Write(t.Context(), []device.Write{{Name: "a", Data: []byte("kept")}}, true); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = device.OpenDir(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := ExpectFile(t.Context(), d, "a", []byte("kept")); err != nil {
		t.Fatal(err)
	}
}
