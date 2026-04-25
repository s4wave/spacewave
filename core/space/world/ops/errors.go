package space_world_ops

import "errors"

// ErrNodeRequired is returned when a node is required but not provided.
var ErrNodeRequired = errors.New("canvas node is required")

// ErrNodeIdRequired is returned when a node ID is required but empty.
var ErrNodeIdRequired = errors.New("canvas node id is required")

// ErrNodeIdsRequired is returned when at least one node ID is required.
var ErrNodeIdsRequired = errors.New("at least one node id is required")

// ErrNodeNotFound is returned when a canvas node is not found.
var ErrNodeNotFound = errors.New("canvas node not found")

// ErrEdgeNil is returned if the edge is nil.
var ErrEdgeNil = errors.New("edge cannot be nil")

// ErrEdgeEmptyId is returned if the edge has an empty id.
var ErrEdgeEmptyId = errors.New("edge id cannot be empty")

// ErrEdgeEmptySourceNodeId is returned if the edge has an empty source node id.
var ErrEdgeEmptySourceNodeId = errors.New("edge source_node_id cannot be empty")

// ErrEdgeEmptyTargetNodeId is returned if the edge has an empty target node id.
var ErrEdgeEmptyTargetNodeId = errors.New("edge target_node_id cannot be empty")

// ErrEdgeNodeNotFound is returned if an edge references a non-existent node.
var ErrEdgeNodeNotFound = errors.New("edge references non-existent node")

// ErrNoEdgeIds is returned if no edge ids are provided.
var ErrNoEdgeIds = errors.New("at least one edge id is required")
