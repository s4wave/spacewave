package wayland

import (
	"bytes"
	"image"
	"image/draw"
	"math/rand/v2"
	"testing"
)

// TestTexture checks a viewer's copy of a texture matches the image after
// each update, through random paints with updates taken at random.
func TestTexture(t *testing.T) {
	// Start the image and the viewer's copy alike.
	bounds := image.Rect(0, 0, 24, 24)
	truth, copied := image.NewRGBA(bounds), image.NewRGBA(bounds)
	tex := &texture{size: bounds.Size()}
	rng := rand.New(rand.NewPCG(1, 2))
	color := byte(0)

	// Paint and take updates at random.
	for step := range 20000 {
		if rng.IntN(3) != 0 {
			// Paint an area.
			r := image.Rect(rng.IntN(24), rng.IntN(24), rng.IntN(24), rng.IntN(24))
			color++
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					truth.Pix[truth.PixOffset(x, y)] = color
				}
			}
			tex.add(r)
			continue
		}

		// Take an update and apply it as a viewer does.
		for _, r := range tex.damage {
			draw.Draw(copied, r, truth, r.Min, draw.Src)
		}
		tex.damage = tex.damage[:0]
		if !bytes.Equal(truth.Pix, copied.Pix) {
			t.Fatalf("step %d: the viewer's copy differs from the image", step)
		}
	}
}

// TestTextureEdges checks that, for thin areas such as a window's edges,
// a long list merges into areas near their own size rather than their
// bounding box.
func TestTextureEdges(t *testing.T) {
	// Mark the left and right edges of ten windows stacked down the screen.
	tex := &texture{}
	marked := 0
	for i := range 10 {
		y := 100 * i
		for _, r := range []image.Rectangle{image.Rect(0, y, 4, y+90), image.Rect(996, y, 1000, y+90)} {
			tex.add(r)
			marked += areaOf(r)
		}
	}

	// The list stays short and draws at most twice what was marked.
	if len(tex.damage) > maxDamage {
		t.Fatalf("%d areas, want at most %d", len(tex.damage), maxDamage)
	}
	drawn := 0
	for _, d := range tex.damage {
		drawn += areaOf(d)
	}
	if drawn > 2*marked {
		t.Fatalf("draws %d pixels for %d marked", drawn, marked)
	}
}

// TestTextureShifted checks an edge marked again a few pixels over at each
// step merges into one area over the path.
func TestTextureShifted(t *testing.T) {
	tex := &texture{}
	for i := range 8 {
		tex.add(image.Rect(0, 0, 900, 25).Add(image.Pt(6*i, 4*i)))
	}
	if want := image.Rect(0, 0, 942, 53); len(tex.damage) != 1 || tex.damage[0] != want {
		t.Fatalf("marked %v, want [%v]", tex.damage, want)
	}
}
