package resource_world

import (
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
)

// BuildTree builds a new UnixFS tree in the stage from an UploadTree entry
// stream and returns its root, which the stage holds until it is released or
// drops the root with ReleaseRoots.
func (r *WorldStageResource) BuildTree(
	strm s4wave_unixfs_world.SRPCUnixFSStageResourceService_BuildTreeStream,
) (*s4wave_unixfs_world.BuildTreeResponse, error) {
	rootRef, err := resource_unixfs.BuildStagedTree(strm, r.stage)
	if err != nil {
		return nil, err
	}
	return &s4wave_unixfs_world.BuildTreeResponse{RootRef: rootRef}, nil
}

// _ is a type assertion
var _ s4wave_unixfs_world.SRPCUnixFSStageResourceServiceServer = (*WorldStageResource)(nil)
