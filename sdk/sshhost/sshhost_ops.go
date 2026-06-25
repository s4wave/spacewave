package s4wave_sshhost

import (
	"context"
	"slices"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CreateSshHostOpId is the operation id for CreateSshHostOp.
var CreateSshHostOpId = "spacewave/ssh-host/create"

// NewCreateSshHostOp constructs a new CreateSshHostOp.
func NewCreateSshHostOp(
	objKey string,
	label string,
	endpoint *SshHostEndpoint,
	credentials *SshHostCredentialRefs,
	hostKeyPins []*SshHostKeyPin,
	ts time.Time,
) *CreateSshHostOp {
	return &CreateSshHostOp{
		ObjectKey:   objKey,
		Label:       label,
		Endpoint:    endpoint,
		Credentials: credentials,
		HostKeyPins: hostKeyPins,
		Timestamp:   timestamppb.New(ts),
	}
}

// NewCreateSshHostOpBlock constructs a CreateSshHostOp block.
func NewCreateSshHostOpBlock() block.Block {
	return &CreateSshHostOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateSshHostOp) GetOperationTypeId() string {
	return CreateSshHostOpId
}

// Validate performs cursory checks on the op.
func (o *CreateSshHostOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if strings.TrimSpace(o.GetLabel()) == "" {
		return world.ErrEmptyOp
	}
	host := o.buildSshHost()
	if err := host.Validate(); err != nil {
		return err
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateSshHostOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	host := o.buildSshHost()
	_, _, err = world.CreateWorldObject(ctx, ws, o.GetObjectKey(), func(bcs *block.Cursor) error {
		bcs.SetBlock(host, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, o.GetObjectKey(), SshHostTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *CreateSshHostOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *CreateSshHostOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *CreateSshHostOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupCreateSshHostOp looks up a CreateSshHostOp operation type.
func LookupCreateSshHostOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateSshHostOpId {
		return &CreateSshHostOp{}, nil
	}
	return nil, nil
}

func (o *CreateSshHostOp) buildSshHost() *SshHost {
	ts := o.GetTimestamp()
	return &SshHost{
		Label:       strings.TrimSpace(o.GetLabel()),
		Endpoint:    NormalizeSshHostEndpoint(o.GetEndpoint()),
		Credentials: o.GetCredentials().CloneVT(),
		HostKeyPins: cloneSshHostKeyPins(o.GetHostKeyPins()),
		LastStatus: &SshHostStatus{
			State:      SshHostProbeState_SSH_HOST_PROBE_STATE_UNKNOWN,
			Message:    "not probed",
			ObservedAt: ts,
		},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func cloneSshHostKeyPins(pins []*SshHostKeyPin) []*SshHostKeyPin {
	if len(pins) == 0 {
		return nil
	}
	out := make([]*SshHostKeyPin, 0, len(pins))
	for _, pin := range pins {
		if pin == nil {
			continue
		}
		out = append(out, pin.CloneVT())
	}
	return slices.Clip(out)
}

// _ is a type assertion.
var _ world.Operation = ((*CreateSshHostOp)(nil))
