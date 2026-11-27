package s4wave_device

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// amdPCIVendor is the PCI vendor id sysfs reports for an AMD card.
const amdPCIVendor = "0x1002"

// DetectGraphicsCards lists the host's graphics cards that a container can use,
// reading sysfs only, with no vendor tools. sys is the sysfs root, such as
// os.DirFS("/sys"). A host without sysfs, such as macOS, has no cards.
//
// Each DRM render node is one card, identified by its PCI address. An AMD card
// is listed when the kernel's ROCm compute interface /dev/kfd is present, with
// its dedicated memory and the two device nodes ROCm opens. An NVIDIA card is
// not listed: a container reaches it only through the NVIDIA container
// toolkit, which no Worker host runs yet.
func DetectGraphicsCards(sys fs.FS) ([]*GraphicsCard, error) {
	// List the render nodes. No DRM class means no cards.
	entries, err := fs.ReadDir(sys, "class/drm")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Without the compute interface no AMD card can run ROCm work.
	_, err = fs.Stat(sys, "class/kfd/kfd")
	kfd := err == nil

	// Describe each AMD render node's card.
	var cards []*GraphicsCard
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "renderD") {
			continue
		}
		dev := path.Join("class/drm", name, "device")
		vendor, err := fs.ReadFile(sys, path.Join(dev, "vendor"))
		if err != nil {
			return nil, err
		}
		if string(bytes.TrimSpace(vendor)) != amdPCIVendor || !kfd {
			continue
		}
		card, err := detectAMDCard(sys, dev, name)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// detectAMDCard describes the AMD card whose sysfs device directory is dev and
// whose render node is renderNode.
func detectAMDCard(sys fs.FS, dev, renderNode string) (*GraphicsCard, error) {
	// Identify the card by the PCI address its uevent records.
	uevent, err := fs.ReadFile(sys, path.Join(dev, "uevent"))
	if err != nil {
		return nil, err
	}
	var id string
	for line := range strings.SplitSeq(string(uevent), "\n") {
		if v, ok := strings.CutPrefix(line, "PCI_SLOT_NAME="); ok {
			id = v
		}
	}
	if id == "" {
		return nil, errors.New(renderNode + ": uevent names no PCI_SLOT_NAME")
	}

	// Read the dedicated memory amdgpu reports.
	vram, err := fs.ReadFile(sys, path.Join(dev, "mem_info_vram_total"))
	if err != nil {
		return nil, err
	}
	memoryBytes, err := strconv.ParseUint(string(bytes.TrimSpace(vram)), 10, 64)
	if err != nil {
		return nil, errors.New(renderNode + ": mem_info_vram_total: " + err.Error())
	}
	return &GraphicsCard{
		Id:          id,
		Vendor:      GraphicsCardVendor_GRAPHICS_CARD_VENDOR_AMD,
		MemoryBytes: memoryBytes,
		DevicePaths: []string{"/dev/kfd", "/dev/dri/" + renderNode},
	}, nil
}
