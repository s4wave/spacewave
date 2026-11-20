// Package testpattern renders a synthetic video source with SMPTE-style bars.
package testpattern

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"strconv"
)

const (
	// Width is the frame width in pixels.
	Width = 640
	// Height is the frame height in pixels.
	Height = 360
	// markerSize is the width and height of the moving square in pixels.
	markerSize = 12
	// markerStep is the horizontal advance per frame in pixels.
	markerStep = 4
	// markerY is the top of the marker's lane in the lower band.
	markerY = 286
	// counterX is the left edge of the first digit in pixels.
	counterX = 14
	// counterY is the top edge of the counter's digits in pixels.
	counterY = 326
	// digitScale is the number of pixels on each side of a glyph cell.
	digitScale = 4
)

// bars is the shared background that each Pattern copies into its frame.
var bars = drawBars()

// digits holds the decimal glyphs as rows of three bits, left to right.
var digits = [10][5]uint8{
	{0b111, 0b101, 0b101, 0b101, 0b111},
	{0b010, 0b110, 0b010, 0b010, 0b111},
	{0b111, 0b001, 0b111, 0b100, 0b111},
	{0b111, 0b001, 0b111, 0b001, 0b111},
	{0b101, 0b101, 0b111, 0b001, 0b001},
	{0b111, 0b100, 0b111, 0b001, 0b111},
	{0b111, 0b100, 0b111, 0b101, 0b111},
	{0b111, 0b001, 0b010, 0b010, 0b010},
	{0b111, 0b101, 0b111, 0b101, 0b111},
	{0b111, 0b101, 0b111, 0b001, 0b111},
}

// Pattern draws successive frames into one reusable image.
// A single caller serializes SetPointer, Draw, and Encode.
type Pattern struct {
	// frame holds the most recently drawn image.
	frame *image.RGBA
	// number is the counter value for the next draw.
	number uint64
	// pointer is the latest pointer position in frame pixels.
	pointer image.Point
}

// New constructs a Pattern with frame zero drawn and the pointer hidden.
func New() *Pattern {
	// Allocate the working image once and fill it with the opening frame.
	p := &Pattern{frame: image.NewRGBA(bars.Bounds()), pointer: image.Pt(-1, -1)}
	p.Draw()
	return p
}

// SetPointer sets the cross position for future frames in frame pixels.
// A point outside the frame hides the cross.
func (p *Pattern) SetPointer(pointer image.Point) {
	p.pointer = pointer
}

// Draw advances the marker and counter and draws the latest pointer position.
func (p *Pattern) Draw() {
	// Restore the bars and move the marker one step along its lane.
	copy(p.frame.Pix, bars.Pix)
	const positions = (Width-markerSize)/markerStep + 1
	x := int(p.number%positions) * markerStep
	marker := image.Rect(x, markerY, x+markerSize, markerY+markerSize)
	draw.Draw(p.frame, marker, image.Black, image.Point{}, draw.Src)
	draw.Draw(p.frame, marker.Inset(2), image.White, image.Point{}, draw.Src)

	// Keep the pointer above the counter and bars.
	p.drawCounter()
	if p.pointer.In(p.frame.Bounds()) {
		p.drawPointer()
	}
	p.number++
}

// Encode writes the most recently drawn frame as a JPEG image.
func (p *Pattern) Encode(w io.Writer) error {
	return jpeg.Encode(w, p.frame, &jpeg.Options{Quality: 85})
}

// drawCounter paints a high-contrast decimal counter over the lower band.
func (p *Pattern) drawCounter() {
	// Give the decimal digits a black backing, even over a bright bar.
	var buf [20]byte
	label := strconv.AppendUint(buf[:0], p.number, 10)
	const step = 4 * digitScale
	backing := image.Rect(counterX-digitScale, counterY-digitScale, counterX+len(label)*step, counterY+6*digitScale)
	draw.Draw(p.frame, backing, image.Black, image.Point{}, draw.Src)

	// Expand each glyph cell to a square large enough to survive JPEG.
	for i, digit := range label {
		for row, bits := range digits[digit-'0'] {
			for col := range 3 {
				if bits&(0b100>>col) != 0 {
					x := counterX + i*step + col*digitScale
					y := counterY + row*digitScale
					cell := image.Rect(x, y, x+digitScale, y+digitScale)
					draw.Draw(p.frame, cell, image.White, image.Point{}, draw.Src)
				}
			}
		}
	}
}

// drawPointer outlines the cross so it remains visible against every bar.
func (p *Pattern) drawPointer() {
	// Paint the dark outline before the two bright arms.
	x, y := p.pointer.X, p.pointer.Y
	across := image.Rect(x-10, y-2, x+11, y+3)
	down := image.Rect(x-2, y-10, x+3, y+11)
	draw.Draw(p.frame, across, image.Black, image.Point{}, draw.Src)
	draw.Draw(p.frame, down, image.Black, image.Point{}, draw.Src)
	draw.Draw(p.frame, across.Inset(1), image.White, image.Point{}, draw.Src)
	draw.Draw(p.frame, down.Inset(1), image.White, image.Point{}, draw.Src)
}

// drawBars builds the reference bands shared by all frames.
func drawBars() *image.RGBA {
	// Use the conventional order of 75-percent colors in the upper band.
	frame := image.NewRGBA(image.Rect(0, 0, Width, Height))
	const level = 191
	top := [...]color.RGBA{
		{R: level, G: level, B: level, A: 255},
		{R: level, G: level, A: 255},
		{G: level, B: level, A: 255},
		{G: level, A: 255},
		{R: level, B: level, A: 255},
		{R: level, A: 255},
		{B: level, A: 255},
	}
	for i, c := range top {
		bar := image.Rect(i*Width/len(top), 0, (i+1)*Width/len(top), 240)
		draw.Draw(frame, bar, image.NewUniform(c), image.Point{}, draw.Src)
	}

	// Alternate reversed colors with black in the shorter middle band.
	black := color.RGBA{A: 255}
	middle := [...]color.RGBA{top[6], black, top[4], black, top[2], black, top[0]}
	for i, c := range middle {
		bar := image.Rect(i*Width/len(middle), 240, (i+1)*Width/len(middle), 270)
		draw.Draw(frame, bar, image.NewUniform(c), image.Point{}, draw.Src)
	}

	// Reserve white and near-black reference patches in the lower band.
	cuts := [...]int{0, 114, 229, 343, 457, 488, 518, Width}
	lower := [...]color.Color{
		color.RGBA{G: 33, B: 76, A: 255},
		color.White,
		color.RGBA{R: 50, B: 106, A: 255},
		black,
		color.Gray{Y: 8},
		color.Gray{Y: 16},
		black,
	}
	for i, c := range lower {
		bar := image.Rect(cuts[i], 270, cuts[i+1], Height)
		draw.Draw(frame, bar, image.NewUniform(c), image.Point{}, draw.Src)
	}

	return frame
}
