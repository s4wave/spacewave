package v86_wazero

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
)

const (
	linuxBootHdrSetupSects       = 0x1f1
	linuxBootHdrBootFlag         = 0x1fe
	linuxBootHdrVidMode          = 0x1fa
	linuxBootHdrHeader           = 0x202
	linuxBootHdrVersion          = 0x206
	linuxBootHdrTypeOfLoader     = 0x210
	linuxBootHdrLoadflags        = 0x211
	linuxBootHdrRamdiskImage     = 0x218
	linuxBootHdrRamdiskSize      = 0x21c
	linuxBootHdrHeapEndPtr       = 0x224
	linuxBootHdrCmdLinePtr       = 0x228
	linuxBootHdrCmdlineSize      = 0x238
	linuxBootHdrChecksum1        = 0xaa55
	linuxBootHdrChecksum2        = 0x53726448
	linuxBootHdrLoaderUnassigned = 0xff
	linuxBootHdrLoadedHigh       = 1 << 0
	linuxBootHdrQuietFlag        = 1 << 5
	linuxBootHdrKeepSegments     = 1 << 6
	linuxBootHdrCanUseHeap       = 1 << 7
	kernelHighAddress            = 0x100000
	initrdAddress                = 64 << 20
)

// optionROM is one option-rom image exposed through fw_cfg.
type optionROM struct {
	name string
	data []byte
}

// loadLinuxKernel validates a bzImage and installs it with its initrd and
func (h *HostRuntime) loadLinuxKernel(ctx context.Context, kernel []byte, initrd []byte, cmdline string) error {
	if len(kernel) < linuxBootHdrCmdlineSize+4 {
		return errors.Errorf("kernel image too small: %d bytes", len(kernel))
	}
	bzimage := append([]byte(nil), kernel...)
	if binary.LittleEndian.Uint16(bzimage[linuxBootHdrBootFlag:]) != linuxBootHdrChecksum1 {
		return errors.New("kernel image has invalid boot flag")
	}
	header := uint32(bzimage[linuxBootHdrHeader]) |
		uint32(bzimage[linuxBootHdrHeader+1])<<8 |
		uint32(bzimage[linuxBootHdrHeader+2])<<16 |
		uint32(bzimage[linuxBootHdrHeader+3])<<24
	if header != linuxBootHdrChecksum2 {
		return errors.New("kernel image has invalid header magic")
	}
	protocol := binary.LittleEndian.Uint16(bzimage[linuxBootHdrVersion:])
	if protocol < 0x202 {
		return errors.Errorf("kernel boot protocol %#x is not supported", protocol)
	}
	flags := bzimage[linuxBootHdrLoadflags]
	if flags&linuxBootHdrLoadedHigh == 0 {
		return errors.New("kernel image is not loaded-high capable")
	}

	setupSects := uint32(bzimage[linuxBootHdrSetupSects])
	if setupSects == 0 {
		setupSects = 4
	}
	cmdlineSize := uint32(255)
	if protocol >= 0x206 {
		cmdlineSize = binary.LittleEndian.Uint32(bzimage[linuxBootHdrCmdlineSize:])
	}
	cmdlineBytes := append([]byte(cmdline), 0)
	if uint32(len(cmdlineBytes)) >= cmdlineSize {
		return errors.Errorf("kernel cmdline length %d exceeds limit %d", len(cmdlineBytes), cmdlineSize)
	}

	const realModeSegment = 0x8000
	const heapEnd = 0xe000
	basePtr := uint32(realModeSegment << 4)
	cmdLinePtr := basePtr + heapEnd

	bzimage[linuxBootHdrTypeOfLoader] = linuxBootHdrLoaderUnassigned
	bzimage[linuxBootHdrLoadflags] = (flags &^ (linuxBootHdrQuietFlag | linuxBootHdrKeepSegments)) | linuxBootHdrCanUseHeap
	binary.LittleEndian.PutUint16(bzimage[linuxBootHdrVidMode:], 0xffff)
	binary.LittleEndian.PutUint16(bzimage[linuxBootHdrHeapEndPtr:], heapEnd-0x200)
	binary.LittleEndian.PutUint32(bzimage[linuxBootHdrCmdLinePtr:], cmdLinePtr)
	if err := h.writeGuestBlob(ctx, cmdLinePtr, cmdlineBytes); err != nil {
		return errors.Wrap(err, "write kernel cmdline")
	}

	protectedModeStart := (setupSects + 1) * 512
	if protectedModeStart >= uint32(len(bzimage)) {
		return errors.Errorf("kernel protected-mode offset %#x exceeds image size %#x", protectedModeStart, len(bzimage))
	}
	if len(initrd) != 0 {
		if uint64(kernelHighAddress)+uint64(len(bzimage))-uint64(protectedModeStart) >= initrdAddress {
			return errors.New("kernel image overlaps fixed initrd address")
		}
		if err := h.writeGuestBlob(ctx, initrdAddress, initrd); err != nil {
			return errors.Wrap(err, "write initrd")
		}
		binary.LittleEndian.PutUint32(bzimage[linuxBootHdrRamdiskImage:], initrdAddress)
		binary.LittleEndian.PutUint32(bzimage[linuxBootHdrRamdiskSize:], uint32(len(initrd)))
	}
	if basePtr+protectedModeStart >= 0xa0000 {
		return errors.New("kernel real-mode setup exceeds low memory")
	}
	if err := h.writeGuestBlob(ctx, basePtr, bzimage[:protectedModeStart]); err != nil {
		return errors.Wrap(err, "write kernel real-mode setup")
	}
	if err := h.writeGuestBlob(ctx, kernelHighAddress, bzimage[protectedModeStart:]); err != nil {
		return errors.Wrap(err, "write kernel protected-mode image")
	}

	h.optionROMs = append(h.optionROMs, optionROM{
		name: "genroms/kernel.bin",
		data: makeLinuxBootROM(realModeSegment, heapEnd),
	})
	return nil
}

// makeLinuxBootROM builds the 16-bit boot stub that jumps into the loaded
func makeLinuxBootROM(realModeSegment, heapEnd uint16) []byte {
	const size = 0x200
	data := make([]byte, size)
	binary.LittleEndian.PutUint16(data[0:], 0xaa55)
	data[2] = size / 0x200

	i := 3
	data[i] = 0xfa
	i++
	data[i] = 0xb8
	i++
	binary.LittleEndian.PutUint16(data[i:], realModeSegment)
	i += 2
	data[i] = 0x8e
	data[i+1] = 0xc0
	i += 2
	data[i] = 0x8e
	data[i+1] = 0xd8
	i += 2
	data[i] = 0x8e
	data[i+1] = 0xe0
	i += 2
	data[i] = 0x8e
	data[i+1] = 0xe8
	i += 2
	data[i] = 0x8e
	data[i+1] = 0xd0
	i += 2
	data[i] = 0xbc
	i++
	binary.LittleEndian.PutUint16(data[i:], heapEnd)
	i += 2
	data[i] = 0xea
	i++
	binary.LittleEndian.PutUint16(data[i:], 0)
	i += 2
	binary.LittleEndian.PutUint16(data[i:], realModeSegment+0x20)
	i += 2

	checksumIndex := i
	var checksum byte
	for _, b := range data {
		checksum += b
	}
	data[checksumIndex] = -checksum
	return data
}
