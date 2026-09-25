// Package devicetest checks a device.Device against its contract, so native
// tests and browser fixtures run the same checks on every implementation.
package devicetest

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/rand/v2"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/device"
)

// Check runs the contract checks on an empty device.
func Check(ctx context.Context, d device.Device) error {
	if err := checkContract(ctx, d); err != nil {
		return err
	}
	if err := d.Remove(ctx, []string{"a"}); err != nil {
		return err
	}
	return checkModel(ctx, d, 1)
}

// checkContract checks each method's documented behavior on small files.
func checkContract(ctx context.Context, d device.Device) error {
	// Writes apply in order, and a write past the end zero fills the gap.
	writes := []device.Write{
		{Name: "a", Offset: 0, Data: []byte("hello")},
		{Name: "a", Offset: 3, Data: []byte("LO world")},
		{Name: "b", Offset: 4, Data: []byte("x")},
	}
	if err := d.Write(ctx, writes, true); err != nil {
		return err
	}
	if err := ExpectFile(ctx, d, "a", []byte("helLO world")); err != nil {
		return err
	}
	if err := ExpectFile(ctx, d, "b", []byte("\x00\x00\x00\x00x")); err != nil {
		return err
	}

	// A batch of reads fills each range, and a range past the end fails.
	reads := []device.Read{
		{Name: "a", Offset: 6, Data: make([]byte, 5)},
		{Name: "b", Offset: 4, Data: make([]byte, 1)},
	}
	if err := d.Read(ctx, reads); err != nil {
		return err
	}
	if string(reads[0].Data) != "world" || string(reads[1].Data) != "x" {
		return errors.Errorf("read %q and %q", reads[0].Data, reads[1].Data)
	}
	err := d.Read(ctx, []device.Read{{Name: "a", Offset: 8, Data: make([]byte, 4)}})
	if !errors.Is(err, device.ErrShortRead) {
		return errors.Errorf("read past end: %v", err)
	}
	err = d.Read(ctx, []device.Read{{Name: "missing", Data: make([]byte, 1)}})
	if !errors.Is(err, device.ErrShortRead) {
		return errors.Errorf("read of missing file: %v", err)
	}

	// Truncate shrinks and zero fills growth.
	if err := d.Truncate(ctx, "a", 3); err != nil {
		return err
	}
	if err := d.Truncate(ctx, "a", 5); err != nil {
		return err
	}
	if err := ExpectFile(ctx, d, "a", []byte("hel\x00\x00")); err != nil {
		return err
	}

	// Remove ignores missing files, and List reports what remains.
	if err := d.Remove(ctx, []string{"b", "missing"}); err != nil {
		return err
	}
	files, err := d.List(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(files, []device.File{{Name: "a", Size: 5}}) {
		return errors.Errorf("list %v", files)
	}

	// Names must be flat.
	if err := d.Write(ctx, []device.Write{{Name: "x/y", Data: []byte("z")}}, false); err == nil {
		return errors.New("nested name accepted")
	}
	return nil
}

// modelFiles are the files checkModel writes.
var modelFiles = []string{"m0", "m1", "m2"}

// modelSpan bounds the offsets and lengths checkModel uses, so writes cross
// the chunk and page boundaries of block-structured devices.
const modelSpan = 300 << 10

// checkModel applies random writes, truncates, and removes to the device and
// to an in-memory image, then compares every file with the image.
func checkModel(ctx context.Context, d device.Device, seed uint64) error {
	rng := rand.New(rand.NewPCG(seed, seed)) //nolint:gosec
	image := make(map[string][]byte)
	for step := range 64 {
		name := modelFiles[rng.IntN(len(modelFiles))]
		switch n := rng.IntN(10); {
		case n < 7:
			// Write a batch of random ranges.
			var writes []device.Write
			for range 1 + rng.IntN(4) {
				off := rng.Int64N(modelSpan)
				n := 1 + rng.IntN(modelSpan/3)
				data := make([]byte, 0, n+8)
				for len(data) < n {
					data = binary.LittleEndian.AppendUint64(data, rng.Uint64())
				}
				data = data[:n]
				writes = append(writes, device.Write{Name: name, Offset: off, Data: data})
				image[name] = writeImage(image[name], off, data)
			}
			if err := d.Write(ctx, writes, step%4 == 0); err != nil {
				return err
			}
		case n < 9:
			// Truncate an existing file.
			if _, ok := image[name]; !ok {
				continue
			}
			size := rng.Int64N(modelSpan)
			if err := d.Truncate(ctx, name, size); err != nil {
				return err
			}
			image[name] = resize(image[name], size)
		default:
			// Remove the file.
			if err := d.Remove(ctx, []string{name}); err != nil {
				return err
			}
			delete(image, name)
		}
	}

	// Every file matches the image, and no other model file exists.
	for _, name := range modelFiles {
		want, ok := image[name]
		if !ok {
			continue
		}
		if err := ExpectFile(ctx, d, name, want); err != nil {
			return errors.Wrap(err, "model")
		}
	}
	files, err := d.List(ctx)
	if err != nil {
		return err
	}
	if len(files) != len(image) {
		return errors.Errorf("model: list %v, want %d files", files, len(image))
	}
	return nil
}

// writeImage applies one write to a file image.
func writeImage(data []byte, off int64, p []byte) []byte {
	if end := off + int64(len(p)); end > int64(len(data)) {
		data = resize(data, end)
	}
	copy(data[off:], p)
	return data
}

// resize sets an image's length, zero filling growth.
func resize(data []byte, size int64) []byte {
	if size <= int64(len(data)) {
		return data[:size]
	}
	return append(data, make([]byte, size-int64(len(data)))...)
}

// ExpectFile checks that the device lists name with want's length and reads
// back want.
func ExpectFile(ctx context.Context, d device.Device, name string, want []byte) error {
	files, err := d.List(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(files, func(f device.File) bool { return f.Name == name })
	if i < 0 || files[i].Size != int64(len(want)) {
		return errors.Errorf("%s: listed %v, want size %d", name, files, len(want))
	}
	got := make([]byte, len(want))
	if err := d.Read(ctx, []device.Read{{Name: name, Data: got}}); err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return errors.Errorf("%s: contents differ", name)
	}
	return nil
}
