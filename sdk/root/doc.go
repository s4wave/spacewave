// Package s4wave_root is the client contract for the root of a Spacewave
// Resource SDK tree, the resource every other resource is reached from.
//
// RootResourceService lists and looks up providers, mounts Sessions by ID or
// index, lists and watches Sessions and their metadata, unlocks, moves, resets
// and deletes them, and serves runtime-wide state: state atoms, Space root
// aliases, hashing helpers, metrics, the changelog and web listeners. Mounting a
// Session returns a child resource for the s4wave_session services. NewRoot
// wraps the root resource of a resource_client.Client.
package s4wave_root
