//go:build !js

package memlab

// SnapshotAnalysis is per-snapshot analysis output.
type SnapshotAnalysis struct {
	// Label is the snapshot label given on the command line.
	Label string `json:"label"`
	// Path is the heap snapshot file.
	Path string `json:"path"`
	// Counts holds the leak-target object counts in this snapshot.
	Counts ObjectCounts `json:"counts"`
	// TopRetained lists the most retained service and method pairs in this snapshot.
	TopRetained []RetainedRpcEntry `json:"topRetained"`
}
