//go:build !js

package memlab

// RetainedRpcEntry is a service/method pair with a count.
type RetainedRpcEntry struct {
	// Service is the RPC service name.
	Service string `json:"service"`
	// Method is the RPC method name.
	Method string `json:"method"`
	// Count is the number of retained calls.
	Count int `json:"count"`
}
