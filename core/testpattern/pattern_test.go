package testpattern

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// TestPatternDrawAndEncode checks that successive frames update the existing
// image and that the latest frame can be decoded as a JPEG.
func TestPatternDrawAndEncode(t *testing.T) {
	// Frame zero has a lit marker and the left stroke of the digit zero.
	p := New()
	frame := p.frame
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if frame.RGBAAt(4, markerY+6) != white || frame.RGBAAt(counterX, counterY) != white {
		t.Fatal("frame zero is missing its marker or counter")
	}

	// Only the latest pointer position should appear in the next frame.
	first := image.Pt(32, 32)
	last := image.Pt(64, 64)
	p.SetPointer(first)
	p.SetPointer(last)
	p.Draw()
	if p.frame != frame {
		t.Fatal("Draw replaced the working image")
	}
	if frame.RGBAAt(first.X, first.Y) != bars.RGBAAt(first.X, first.Y) {
		t.Fatal("frame draws a superseded pointer position")
	}
	if frame.RGBAAt(last.X, last.Y+8) != white || frame.RGBAAt(last.X+8, last.Y) != white {
		t.Fatal("frame is missing one or both arms of the cross")
	}

	// Frame one shifts the marker right and replaces the zero with a one.
	if frame.RGBAAt(4, markerY+6) == white || frame.RGBAAt(13, markerY+6) != white {
		t.Fatal("frame one did not advance the marker")
	}
	if frame.RGBAAt(counterX, counterY) == white || frame.RGBAAt(counterX+digitScale, counterY) != white {
		t.Fatal("frame one does not display the digit one")
	}

	// The cross persists without new input and disappears outside the frame.
	p.Draw()
	if frame.RGBAAt(last.X, last.Y) != white {
		t.Fatal("frame lost the last pointer position")
	}
	p.SetPointer(image.Pt(-1, -1))
	p.Draw()
	if frame.RGBAAt(last.X, last.Y) != bars.RGBAAt(last.X, last.Y) {
		t.Fatal("frame kept a stale cross after the pointer left")
	}

	// Decode the rendered frame through the standard JPEG reader.
	encoded, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decoded.Bounds(), image.Rect(0, 0, 640, 360); got != want {
		t.Fatalf("JPEG bounds = %v, want %v", got, want)
	}
}

// BenchmarkPatternDrawEncode measures one draw and JPEG encode per frame.
func BenchmarkPatternDrawEncode(b *testing.B) {
	// Encode each frame into new bytes, as the source does for the track.
	p := New()
	p.SetPointer(image.Pt(Width/2, Height/2))
	b.ReportAllocs()
	for b.Loop() {
		p.Draw()
		if _, err := p.Encode(); err != nil {
			b.Fatal(err)
		}
	}
}
