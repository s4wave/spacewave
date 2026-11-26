package s4wave_device

import (
	"os"
	"slices"
	"testing"
	"testing/fstest"
)

// TestDetectGraphicsCards detects an AMD card from a copy of a ROCm host's
// sysfs, skips a card of another vendor, and finds none without /dev/kfd.
func TestDetectGraphicsCards(t *testing.T) {
	sys := fstest.MapFS{
		"class/drm/card0/device/vendor":                   {Data: []byte("0x1002\n")},
		"class/drm/renderD128/device/vendor":              {Data: []byte("0x1002\n")},
		"class/drm/renderD128/device/mem_info_vram_total": {Data: []byte("12868124672\n")},
		"class/drm/renderD128/device/uevent":              {Data: []byte("DRIVER=amdgpu\nPCI_ID=1002:73DF\nPCI_SLOT_NAME=0000:03:00.0\n")},
		"class/drm/renderD129/device/vendor":              {Data: []byte("0x10de\n")},
		"class/kfd/kfd/dev":                               {Data: []byte("234:0\n")},
	}
	cards, err := DetectGraphicsCards(sys)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("detected %d cards, want 1", len(cards))
	}
	card := cards[0]
	if card.GetId() != "0000:03:00.0" || card.GetVendor() != GraphicsCardVendor_GRAPHICS_CARD_VENDOR_AMD || card.GetMemoryBytes() != 12868124672 {
		t.Fatalf("detected %v", card)
	}
	if !slices.Equal(card.GetDevicePaths(), []string{"/dev/kfd", "/dev/dri/renderD128"}) {
		t.Fatalf("device paths %v", card.GetDevicePaths())
	}
	if err := card.Validate(); err != nil {
		t.Fatal(err)
	}

	// Without the compute interface the card cannot run ROCm work.
	delete(sys, "class/kfd/kfd/dev")
	if cards, err := DetectGraphicsCards(sys); err != nil || len(cards) != 0 {
		t.Fatalf("without kfd: %v, %v", cards, err)
	}

	// A host without sysfs has no cards.
	if cards, err := DetectGraphicsCards(fstest.MapFS{}); err != nil || len(cards) != 0 {
		t.Fatalf("without sysfs: %v, %v", cards, err)
	}
}

// TestDetectHostGraphicsCards logs the cards this host's sysfs reports, so a
// run on a card host shows detection through real sysfs symlinks.
func TestDetectHostGraphicsCards(t *testing.T) {
	cards, err := DetectGraphicsCards(os.DirFS("/sys"))
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		t.Logf("host card %v", card)
	}
}
