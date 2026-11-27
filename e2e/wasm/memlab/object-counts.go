//go:build !js

package memlab

// ObjectCounts holds counts of leak-target objects.
type ObjectCounts struct {
	// ClientRpc counts retained client RPC objects.
	ClientRpc int `json:"clientRpc"`
	// ChannelStream counts retained channel streams.
	ChannelStream int `json:"channelStream"`
	// Promise counts retained promises.
	Promise int `json:"promise"`
	// Generator counts retained generators.
	Generator int `json:"generator"`
	// OnNext counts retained onNext callbacks.
	OnNext int `json:"onNext"`
}
