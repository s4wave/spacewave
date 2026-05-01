// Package s4wave_vm implements the VM block types and world operations.
package s4wave_vm

import (
	"github.com/aperturerobotics/cayley/quad"
)

// VmV86TypeID is the type identifier for VmV86 objects.
const VmV86TypeID = "spacewave/vm/v86"

// PredV86Image is the graph predicate linking a VmV86 to its V86Image. The
// V86Image supplies the default WASM/BIOS/kernel/rootfs UnixFS edges via its
// own v86image/* predicates.
var PredV86Image = quad.IRI("v86/image")

// PredV86KernelOverride optionally overrides the kernel UnixFS resolved from
// the linked V86Image. Takes precedence over the V86Image's v86image/kernel edge.
var PredV86KernelOverride = quad.IRI("v86/kernel-override")

// PredV86RootfsOverride optionally overrides the rootfs UnixFS resolved from
// the linked V86Image. Takes precedence over the V86Image's v86image/rootfs edge.
var PredV86RootfsOverride = quad.IRI("v86/rootfs-override")

// PredV86BiosOverride optionally overrides the BIOS UnixFS resolved from the
// linked V86Image. Takes precedence over the V86Image's
// v86image/bios/{seabios,vgabios} edges.
var PredV86BiosOverride = quad.IRI("v86/bios-override")

// PredV86WasmOverride optionally overrides the emulator WASM UnixFS resolved
// from the linked V86Image. Takes precedence over v86image/wasm.
var PredV86WasmOverride = quad.IRI("v86/wasm-override")

// GetBlockTypeId returns the block type identifier.
func (v *VmV86) GetBlockTypeId() string {
	return VmV86TypeID
}

// MarshalBlock marshals the block to binary.
func (v *VmV86) MarshalBlock() ([]byte, error) {
	return v.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (v *VmV86) UnmarshalBlock(data []byte) error {
	return v.UnmarshalVT(data)
}

// Validate performs cursory checks on the VmV86.
func (v *VmV86) Validate() error {
	return nil
}
