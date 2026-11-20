// Package s4wave_space is the client contract for a mounted Space: a
// SharedObject whose body is a World, presented as a workspace.
//
// SpaceResourceService watches the Space's state (readiness, settings and typed
// World contents) and its sharing state (participants and the viewer's role),
// gives access to its World engine as a child resource for the s4wave_world
// services, and manages what the Space carries besides objects: secrets, its
// plugins and their manifests, and its sequencer, control, backfill and roster
// settings. MountSpaceContents returns a SpaceContentsResourceService, which
// runs the Space's plugins and their process bindings.
package s4wave_space
