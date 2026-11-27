package wayland

import (
	"image"
	"slices"

	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// maxDamage bounds a texture's damage list. Past it the texture merges the
// two areas whose bounding box adds the least area.
const maxDamage = 16

// view is what one viewer has of a space's scene: the screen, layers, roots,
// grab and frames last sent, and the texture of each layer. The
// compositor's lock guards it.
type view struct {
	// screen is the screen last sent, or nil.
	screen *s4wave_vm.WasiVmSceneScreen
	// layers is the layers last sent, or nil.
	layers *s4wave_vm.WasiVmSceneLayers
	// roots is the roots last sent, or nil.
	roots *s4wave_vm.WasiVmSceneRoots
	// grab is the grab last sent, or nil.
	grab *s4wave_vm.WasiVmSceneGrab
	// frames is the frames last sent, or nil.
	frames *s4wave_vm.WasiVmSceneFrames
	// textures is the texture the viewer holds for each surface shown.
	textures map[*surface]*texture
}

// newView returns a view holding nothing.
func newView() *view {
	return &view{textures: make(map[*surface]*texture)}
}

// texture is a viewer's copy of a surface's image: its size in pixels and
// the areas of it to send again.
type texture struct {
	// size is the texture's size in pixels.
	size image.Point
	// damage is the areas to send again, in texture pixels.
	damage []image.Rectangle
}

// add marks the area r, inside the texture, to send again.
func (t *texture) add(r image.Rectangle) {
	// Skip an area already marked and drop those it covers.
	if r.Empty() {
		return
	}
	if slices.ContainsFunc(t.damage, r.In) {
		return
	}

	// Grow r over each marked area whose box with r sends no more than
	// both areas, such as a line redrawn a few pixels over, then drop the areas
	// r covers.
	for {
		i := slices.IndexFunc(t.damage, func(d image.Rectangle) bool { return waste(d, r) <= areaOf(d.Intersect(r)) })
		if i < 0 {
			break
		}
		r = r.Union(t.damage[i])
		t.damage = slices.Delete(t.damage, i, i+1)
	}
	t.damage = slices.DeleteFunc(t.damage, func(d image.Rectangle) bool { return d.In(r) })
	t.damage = append(t.damage, r)

	// Shorten a long list by merging the closest two areas. Merging keeps
	// thin edges thin rather than sending all of the image.
	for len(t.damage) > maxDamage {
		i, j := closest(t.damage)
		u := t.damage[i].Union(t.damage[j])
		t.damage = slices.DeleteFunc(t.damage, func(d image.Rectangle) bool { return d.In(u) })
		t.damage = append(t.damage, u)
	}
}

// closest returns the indexes of the two areas whose bounding box adds the
// least area beyond theirs.
func closest(areas []image.Rectangle) (int, int) {
	bi, bj, best := 0, 1, -1
	for i, a := range areas {
		for j := i + 1; j < len(areas); j++ {
			if w := waste(a, areas[j]); best < 0 || w < best {
				bi, bj, best = i, j, w
			}
		}
	}
	return bi, bj
}

// waste returns the area of the bounding box of a and b outside both.
func waste(a, b image.Rectangle) int {
	return areaOf(a.Union(b)) - areaOf(a) - areaOf(b) + areaOf(a.Intersect(b))
}

// areaOf returns the area of r.
func areaOf(r image.Rectangle) int {
	return r.Dx() * r.Dy()
}
