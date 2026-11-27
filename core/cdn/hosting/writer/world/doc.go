// Package cdn_writer_world keeps CDN Space root pointers in a World: one
// CdnSpace object per Space, written by two typed operations, import and
// commit. The pointer state then survives restarts and replicates with the
// rest of the World. A server whose World runs these operations adds
// SpaceTypes to its schema.
package cdn_writer_world
