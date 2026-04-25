package space

import (
	"context"
	"strings"
	"unicode"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// SpaceBodyType is the space shared object body type id.
const SpaceBodyType = "space"

// SpaceSharedObjectBody is the interface for the space shared object body.
// It provides access to the world engine and metadata about the space.
type SpaceSharedObjectBody interface {
	// GetWorldEngine returns the world engine for this space.
	GetWorldEngine() world.Engine
	// GetWorldEngineID returns the world engine identifier for this space.
	GetWorldEngineID() string
	// GetWorldEngineBucketID returns the bucket ID for the world engine.
	GetWorldEngineBucketID() string
	// GetSharedObjectRef returns the shared object reference for this space.
	GetSharedObjectRef() *sobject.SharedObjectRef
	// GetSharedObject returns the shared object handle.
	GetSharedObject() sobject.SharedObject
}

// MountSharedObjectBodyValue is the value of the MountSharedObjectBody directive for a space.
type MountSharedObjectBodyValue = sobject.MountSharedObjectBodyValue[SpaceSharedObjectBody]

// SpaceEngineId returns the space world engine id.
func SpaceEngineId(ref *sobject.SharedObjectRef) string {
	resourceRef := ref.GetProviderResourceRef()
	return strings.Join([]string{
		SpaceBodyType,
		resourceRef.GetProviderId(),
		resourceRef.GetProviderAccountId(),
		resourceRef.GetId(),
	}, "/")
}

// NewSpaceSoMeta constructs a new SpaceSoMeta.
func NewSpaceSoMeta(name string) *SpaceSoMeta {
	return &SpaceSoMeta{Name: name}
}

// NewSharedObjectMeta constructs a new SharedObjectMeta for a space.
func NewSharedObjectMeta(spaceName string) (*sobject.SharedObjectMeta, error) {
	meta := NewSpaceSoMeta(spaceName)
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	metaDat, err := meta.MarshalVT()
	if err != nil {
		return nil, err
	}
	return &sobject.SharedObjectMeta{
		BodyType: SpaceBodyType,
		BodyMeta: metaDat,
	}, nil
}

// FixupSpaceName fixes up a space name by trimming spaces and
// collapsing consecutive spaces into single spaces.
func FixupSpaceName(name string) string {
	// Trim spaces from start and end
	name = strings.TrimSpace(name)

	// Replace consecutive spaces with single space
	wasSpace := false
	var b strings.Builder
	for _, r := range name {
		if r == ' ' {
			if !wasSpace {
				b.WriteRune(r)
			}
			wasSpace = true
		} else {
			b.WriteRune(r)
			wasSpace = false
		}
	}

	return b.String()
}

// ValidateSpaceName validates a space name.
//
// - Must be 1-64 characters
// - Must start with a letter
// - Can only contain letters, numbers, dash, underscore, and spaces
// - Must not end with dash, underscore, or space
// - No consecutive dashes, underscores, or spaces
// - No special characters
// - Spaces allowed only in the middle
func ValidateSpaceName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return errors.Errorf("space name: must be between 1 and 64 characters")
	}

	if !unicode.IsLetter(rune(name[0])) {
		return errors.Errorf("space name: must start with a letter")
	}

	if name[len(name)-1] == '-' || name[len(name)-1] == '_' || name[len(name)-1] == ' ' {
		return errors.Errorf("space name: must not end with dash, underscore, or space")
	}

	prev := rune(0)
	for _, r := range name {
		if (r == '-' || r == '_' || r == ' ') && (prev == '-' || prev == '_' || prev == ' ') {
			return errors.Errorf("space name: must not contain consecutive dashes, underscores, or spaces")
		}

		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '_' && r != ' ' {
			return errors.Errorf("space name: can only contain letters, numbers, dash, underscore, and spaces")
		}

		prev = r
	}

	return nil
}

// Validate validates the SpaceSoMeta.
func (m *SpaceSoMeta) Validate() error {
	if err := ValidateSpaceName(m.GetName()); err != nil {
		return err
	}
	return nil
}

// FilterSharedObjectList filters a SharedObjectList to a list of SpaceSoListEntry.
// invalidMeta is optional and is called if an invalid metadata is found.
// if invalidMeta is nil we will return the error directly when encountering an invalid metadata.
func FilterSharedObjectList(
	list []*sobject.SharedObjectListEntry,
	invalidMeta func(ent *sobject.SharedObjectListEntry, err error) error,
) ([]*SpaceSoListEntry, error) {
	// match spaces
	out := make([]*SpaceSoListEntry, 0, len(list))
	for _, entry := range list {
		entryMeta := entry.GetMeta()
		if entryMeta.GetBodyType() == SpaceBodyType {
			meta := &SpaceSoMeta{}
			if err := meta.UnmarshalVT(entryMeta.GetBodyMeta()); err != nil {
				// if invalidMeta callback is provided call it
				if invalidMeta != nil {
					err = invalidMeta(entry, err)
					if err == nil {
						continue
					}
				}
				return nil, err
			}
			out = append(out, &SpaceSoListEntry{
				Entry:     entry,
				SpaceMeta: meta,
			})
		}
	}
	return out, nil
}

// ExMountSpaceSoBody executes a mount directive for a space shared object body.
// Returns the mounted space engine, a directive reference, and any error.
func ExMountSpaceSoBody(
	ctx context.Context,
	b bus.Bus,
	ref *sobject.SharedObjectRef,
	returnIfIdle bool,
	valDisposeCb func(),
) (MountSharedObjectBodyValue, directive.Reference, error) {
	return sobject.ExMountSharedObjectBody[SpaceSharedObjectBody](
		ctx,
		b,
		ref,
		SpaceBodyType,
		returnIfIdle,
		valDisposeCb,
	)
}
