package s4wave_device

import "github.com/pkg/errors"

// Valid reports whether the vendor is a defined value.
func (v GraphicsCardVendor) Valid() bool {
	_, ok := GraphicsCardVendor_name[int32(v)]
	return ok
}

// Validate validates a declared card: it needs an id, a known vendor and the
// device nodes a container opens to use it.
func (c *GraphicsCard) Validate() error {
	switch {
	case c.GetId() == "":
		return errors.New("id must be set")
	case c.GetVendor() == GraphicsCardVendor_GRAPHICS_CARD_VENDOR_UNKNOWN || !c.GetVendor().Valid():
		return errors.New("invalid vendor")
	case len(c.GetDevicePaths()) == 0:
		return errors.New("device_paths must be set")
	}
	return nil
}

// Validate validates a card request: its vendor, when set, is a known vendor.
// A nil request asks for no cards.
func (r *GraphicsCardRequest) Validate() error {
	if !r.GetVendor().Valid() {
		return errors.New("invalid vendor")
	}
	return nil
}
