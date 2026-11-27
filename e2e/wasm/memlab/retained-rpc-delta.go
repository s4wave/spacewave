//go:build !js

package memlab

// RetainedRpcDelta is a per-pair retained count delta between first and last snapshots.
type RetainedRpcDelta struct {
	// Service is the RPC service name.
	Service string `json:"service"`
	// Method is the RPC method name.
	Method string `json:"method"`
	// BaselineCount is the retained count in the first snapshot.
	BaselineCount int `json:"baselineCount"`
	// FinalCount is the retained count in the last snapshot.
	FinalCount int `json:"finalCount"`
	// Delta is FinalCount minus BaselineCount.
	Delta int `json:"delta"`
}
