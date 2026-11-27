package s4wave_vm

import (
	"github.com/aperturerobotics/cayley/quad"
)

// PredWasiVmRootfs is the graph predicate linking a WasiVm to the UnixFS
// object holding its root filesystem.
var PredWasiVmRootfs = quad.IRI("wasivm/rootfs")

// PredWasiVmImage is the graph predicate linking a WasiVm to the WasiVmImage
// supplying its root filesystem when the WasiVm has no wasivm/rootfs edge.
var PredWasiVmImage = quad.IRI("wasivm/image")

// DefaultWasiVmInitPath is the init program run when the config selects no
// init program.
const DefaultWasiVmInitPath = "/bin/sh"

// Validate performs cursory checks on the WasiVm.
func (v *WasiVm) Validate() error {
	return nil
}
