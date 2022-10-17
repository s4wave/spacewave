package plugin_host

import (
	"context"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
)

const (
	// PluginHostTypeID is the type identifier for a PluginHost.
	PluginHostTypeID = "bldr/plugin-host"
	// PluginManifestTypeID is the type identifier for a PluginManifest.
	PluginManifestTypeID = "bldr/plugin-manifest"

	// PredPluginHostToPluginManifest is the predicate linking a host to a manifest.
	PredPluginHostToPluginManifest = quad.IRI(PluginManifestTypeID)
)

// NewPluginHostToPluginManifestQuad links PluginHost to PluginManifest.
func NewPluginHostToPluginManifestQuad(pluginHostKey, pluginManifestKey, pluginID string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		pluginHostKey,
		PredPluginHostToPluginManifest.String(),
		pluginManifestKey,
		quad.IRI(pluginID).String(),
	)
}

// NewPluginHostPluginManifestKey builds the object key for a plugin manifest attached to a PluginHost.
func NewPluginHostPluginManifestKey(pluginHostKey, pluginID string) string {
	return strings.Join([]string{pluginHostKey, "p", pluginID}, "/")
}

// CreatePluginHost creates a PluginHost object if it doesn't exist.
func CreatePluginHost(ctx context.Context, ws world.WorldState, objKey string) (created bool, err error) {
	_, hostExists, err := ws.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if hostExists {
		return false, nil
	}

	// TODO: plugin host object contents ?
	_, err = ws.CreateObject(objKey, nil)
	if err != nil {
		return false, err
	}

	types := world_types.NewTypesState(ctx, ws)
	err = types.SetObjectType(objKey, PluginHostTypeID)
	return true, err
}

// SetPluginManifest creates a PluginManifest object in the world.
//
// Checks if the object exists already, and updates it if so.
func SetPluginManifest(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, error) {
	obj, objOk, err := ws.GetObject(objKey)
	if err != nil {
		return nil, err
	}
	if objOk {
		_, err = obj.SetRootRef(rootRef)
	} else {
		_, err = ws.CreateObject(objKey, rootRef)
		if err == nil {
			// create the <type> ref
			typesState := world_types.NewTypesState(ctx, ws)
			err = typesState.SetObjectType(objKey, PluginManifestTypeID)
		}
	}
	return nil, err
}

// CheckPluginHostType checks the type graph quad for a PluginHost.
func CheckPluginHostType(typesState *world_types.TypesState, objKey string) error {
	pluginHostType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if pluginHostType != PluginHostTypeID {
		return errors.Errorf("expected plugin host type %s but got %q", PluginHostTypeID, pluginHostType)
	}
	return err
}

// LookupPluginManifest looks up a PluginManifest in the world.
func LookupPluginManifest(ctx context.Context, ws world.WorldState, objKey string) (*bldr_plugin.PluginManifest, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, err
	}
	var manifest *bldr_plugin.PluginManifest
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_plugin.UnmarshalPluginManifest(bcs)
		return err
	})
	return manifest, err
}

// ListPluginHostPluginManifests lists all plugin manifests assigned to the PluginHost.
func ListPluginHostPluginManifests(ctx context.Context, w world.WorldState, pluginHostKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		pluginHostKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredPluginHostToPluginManifest), nil
		},
	)
}

// CollectPluginHostPluginManifests collects all PluginManifest linked to by the PluginHost.
func CollectPluginHostPluginManifests(
	ctx context.Context,
	ws world.WorldState,
	pluginHostKeys ...string,
) ([]*bldr_plugin.PluginManifest, []string, []error, error) {
	pluginManifestObjKeys, err := ListPluginHostPluginManifests(ctx, ws, pluginHostKeys...)
	if err != nil {
		return nil, nil, nil, err
	}

	var manifestErrors []error
	manifests := make([]*bldr_plugin.PluginManifest, 0, len(pluginManifestObjKeys))
	for _, objKey := range pluginManifestObjKeys {
		manifest, err := LookupPluginManifest(ctx, ws, objKey)
		if err != nil {
			manifestErrors = append(manifestErrors, errors.Wrapf(err, "plugin_manifests[%s]", objKey))
		} else {
			manifests = append(manifests, manifest)
		}
	}

	return manifests, pluginManifestObjKeys, manifestErrors, nil
}

// LookupPluginHostManifest looks up the PluginManifest with the given plugin ID.
// If not found, returns nil, "", nil
func LookupPluginHostManifest(
	ctx context.Context,
	ws world.WorldState,
	pluginHostKey string,
	pluginID string,
) (*bldr_plugin.PluginManifest, string, error) {
	gqs, err := ws.LookupGraphQuads(NewPluginHostToPluginManifestQuad(pluginHostKey, "", pluginID), 1)
	if err != nil {
		return nil, "", err
	}

	if len(gqs) == 0 {
		return nil, "", nil
	}

	gq := gqs[0]
	pluginManifestKey, err := world.GraphValueToKey(gq.GetObj())
	if err != nil {
		return nil, "", err
	}

	manifest, err := LookupPluginManifest(ctx, ws, pluginManifestKey)
	if err != nil {
		return nil, pluginManifestKey, err
	}
	return manifest, pluginManifestKey, nil
}

// CheckPluginHostHasPluginManifest checks if the PluginHost is linked to a PluginManifest.
func CheckPluginHostHasPluginManifest(ctx context.Context, w world.WorldState, pluginHostKey, pluginManifestKey string) (bool, error) {
	gq, err := w.LookupGraphQuads(world.NewGraphQuad(
		world.KeyToGraphValue(pluginHostKey).String(),
		PredPluginHostToPluginManifest.String(),
		world.KeyToGraphValue(pluginManifestKey).String(),
		"",
	), 1)
	if err != nil {
		return false, err
	}
	return len(gq) != 0, nil
}

// EnsurePluginHostHasPluginManifest checks if the PluginHost has the PluginManifest and returns an error otherwise.
func EnsurePluginHostHasPluginManifest(ctx context.Context, w world.WorldState, pluginHostKey, pluginManifestKey string) error {
	hasPass, err := CheckPluginHostHasPluginManifest(ctx, w, pluginHostKey, pluginManifestKey)
	if err == nil && !hasPass {
		err = errors.Errorf("plugin host %s does not have plugin manifest %s", pluginHostKey, pluginManifestKey)
	}
	return err
}
