// Package bldr_project defines a Bldr project: the manifests it can build, the
// builds that group them, and what starts first.
//
// A ProjectConfig maps manifest IDs to their compiler configurations
// (ManifestConfig) and build names to manifests, platforms and targets
// (BuildConfig). StartConfig names the plugins loaded at startup, and
// RemoteConfig names a World to deploy Manifests into. Projects are written in
// Starlark (bldr.star, read by the starlark subpackage); the controller
// subpackage builds a project's manifests in dependency order, building the
// provider of a shared web package before the plugins that exclude it.
package bldr_project
