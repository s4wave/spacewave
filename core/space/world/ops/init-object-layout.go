package space_world_ops

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_web_object "github.com/s4wave/spacewave/web/object"
	"github.com/sirupsen/logrus"
)

// InitObjectLayout initializes an ObjectLayout with starter content in a world.
// Returns any error.
func InitObjectLayout(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewInitObjectLayoutOp(objKey, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// InitObjectLayoutOpId is the operation id for InitObjectLayoutOp.
var InitObjectLayoutOpId = "space/world/init-object-layout"

// NewInitObjectLayoutOp constructs a new InitObjectLayoutOp block.
func NewInitObjectLayoutOp(
	objKey string,
	ts time.Time,
) *InitObjectLayoutOp {
	return &InitObjectLayoutOp{
		ObjectKey: objKey,
		Timestamp: timestamp.New(ts),
	}
}

// NewInitObjectLayoutOpBlock constructs a new InitObjectLayoutOp block.
func NewInitObjectLayoutOpBlock() block.Block {
	return &InitObjectLayoutOp{}
}

// Validate performs cursory checks on the op.
func (o *InitObjectLayoutOp) Validate() error {
	objKey := o.GetObjectKey()
	if len(objKey) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return s4wave_layout_world.CheckObjectLayoutKey(objKey)
}

// GetOperationTypeId returns the operation type identifier.
func (o *InitObjectLayoutOp) GetOperationTypeId() string {
	return InitObjectLayoutOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitObjectLayoutOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Create an ObjectLayout with an initial files tab.
	layout := &s4wave_layout_world.ObjectLayout{
		LayoutModel: &s4wave_layout.LayoutModel{
			Layout: &s4wave_layout.RowDef{
				Id: "root",
				Children: []*s4wave_layout.RowOrTabSetDef{
					{
						Node: &s4wave_layout.RowOrTabSetDef_TabSet{
							TabSet: &s4wave_layout.TabSetDef{
								Id:     "main-tabset",
								Weight: 100,
								Children: []*s4wave_layout.TabDef{
									{
										Id:   "files",
										Name: "Files",
										Data: s4wave_layout_world.NewObjectLayoutTab(
											"",
											&s4wave_web_object.ObjectInfo{
												Info: &s4wave_web_object.ObjectInfo_WorldObjectInfo{
													WorldObjectInfo: &s4wave_web_object.WorldObjectInfo{
														ObjectKey: "files",
													},
												},
											},
											"",
										).Marshal(),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create the object with the layout body
	objState, _, err := world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(layout, true)
		return nil
	})
	if err != nil {
		return false, err
	}
	_ = objState

	// Set the object type
	if err := world_types.SetObjectType(ctx, worldHandle, objKey, s4wave_layout_world.ObjectLayoutTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *InitObjectLayoutOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *InitObjectLayoutOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *InitObjectLayoutOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupInitObjectLayoutOp looks up a InitObjectLayoutOp operation type.
func LookupInitObjectLayoutOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == InitObjectLayoutOpId {
		return &InitObjectLayoutOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*InitObjectLayoutOp)(nil))
