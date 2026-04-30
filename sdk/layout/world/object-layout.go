package s4wave_layout_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_web_object "github.com/s4wave/spacewave/web/object"
)

const (
	// ObjectLayoutTypeID is the type identifier for a ObjectLayout.
	ObjectLayoutTypeID = "alpha/object-layout"
	// ObjectLayoutComponentID is the component identifier for displaying an ObjectLayout.
	ObjectLayoutComponentID = "alpha/object-layout"
)

// NewObjectLayout constructs a new empty object layout.
func NewObjectLayout() *ObjectLayout {
	return &ObjectLayout{}
}

// NewObjectLayoutBlock constructs a new ObjectLayout block.
func NewObjectLayoutBlock() block.Block {
	return &ObjectLayout{}
}

// UnmarshalObjectLayout unmarshals a object layout from a cursor.
// If empty, returns nil, nil.
func UnmarshalObjectLayout(ctx context.Context, bcs *block.Cursor) (*ObjectLayout, error) {
	return block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
}

// LookupObjectLayout looks up the object layout in the world.
func LookupObjectLayout(ctx context.Context, ws world.WorldState, objKey string) (*ObjectLayout, world.ObjectState, error) {
	return world.LookupObject[*ObjectLayout](ctx, ws, objKey, NewObjectLayoutBlock)
}

// AccessObjectLayout accesses the object layout object at a specific ref.
func AccessObjectLayout(ctx context.Context, access world.AccessWorldStateFunc, objRef *bucket.ObjectRef) (*ObjectLayout, error) {
	return world.LookupObjectRef[*ObjectLayout](ctx, access, objRef, NewObjectLayoutBlock)
}

// Clone clones the layout object.
func (s *ObjectLayout) Clone() *ObjectLayout {
	if s == nil {
		return nil
	}
	return s.CloneVT()
}

// NewObjectLayoutTab constructs a new ObjectLayoutTab.
//
// ComponentId is the identifier of the selected component to load.
// May be empty if not selected yet.
//
// ObjectInfo is the information about the object to render info about.
//
// Path is the path to navigate to within the ObjectContainer.
func NewObjectLayoutTab(componentID string, objectInfo *s4wave_web_object.ObjectInfo, path string) *ObjectLayoutTab {
	return &ObjectLayoutTab{
		ComponentId: componentID,
		ObjectInfo:  objectInfo,
		Path:        path,
	}
}

// ValidateObjectLayoutTabDef checks an ObjectLayout TabDef.
func ValidateObjectLayoutTabDef(m *s4wave_layout.TabDef) error {
	tab := &ObjectLayoutTab{}
	if err := tab.UnmarshalVT(m.GetData()); err != nil {
		return err
	}
	return nil
}

// Validate performs cursory checks on the ObjectLayout block.
func (s *ObjectLayout) Validate() error {
	if err := s.GetLayoutModel().Validate(ValidateObjectLayoutTabDef); err != nil {
		return errors.Wrap(err, "layout_model")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *ObjectLayout) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *ObjectLayout) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Marshal marshals the ObjectLayoutTab assuming no errors will be returned.
func (s *ObjectLayoutTab) Marshal() []byte {
	data, _ := s.MarshalVT()
	return data
}

// _ is a type assertion
var _ block.Block = (*ObjectLayout)(nil)
