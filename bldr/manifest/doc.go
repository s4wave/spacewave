// Package bldr_manifest defines the Manifest, one revision of one plugin built
// for one platform, and the references used to select and fetch it.
//
// A Manifest holds its ManifestMeta, the entrypoint path inside the
// distribution filesystem, references to the distribution and asset UnixFS
// roots, and the sorted plugin IDs it depends on, which the host loads and holds
// while it runs. ManifestMeta carries the selection fields: manifest ID, build
// type, platform ID and revision, where a higher revision wins. It also carries
// the hints a host acts on before the plugin runs: whether it may run in the
// background once its user confirms, the Flowgraph node types it registers, and
// the World operation types it handles, so a replica that meets such an
// operation can start the plugin and wait for its handler.
//
// The Manifest declares no services, routes or object types. A plugin registers
// those after it starts, so the same Manifest shape serves a native binary, a
// WebAssembly worker, a GoScript module and a TypeScript module. A ManifestRef
// pairs ManifestMeta with the ObjectRef of the Manifest block, and a
// ManifestBundle groups references under one timestamp.
package bldr_manifest
