package s4wave_device

import (
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldTitle returns the operator-visible label, which object browsers show
// for the Device.
func (d *Device) WorldTitle() string {
	return d.GetLabel()
}

// WorldTime returns when the Device record was made.
func (d *Device) WorldTime() *timestamppb.Timestamp {
	return d.GetCreatedAt()
}

// _ is a type assertion
var (
	_ world.TitledBlock = (*Device)(nil)
	_ world.DatedBlock  = (*Device)(nil)
)
