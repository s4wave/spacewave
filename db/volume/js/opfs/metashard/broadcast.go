//go:build js

package metashard

import (
	"encoding/binary"
	"syscall/js"
)

// BroadcastChannelName is the channel name for meta shard generation invalidation.
const BroadcastChannelName = "hydra-metashard-gen"

// MetaBroadcaster sends meta shard generation invalidation messages.
type MetaBroadcaster struct {
	channel js.Value
}

// NewMetaBroadcaster creates a BroadcastChannel for meta shard invalidation.
func NewMetaBroadcaster() *MetaBroadcaster {
	ch := js.Global().Get("BroadcastChannel").New(BroadcastChannelName)
	return &MetaBroadcaster{channel: ch}
}

// Send broadcasts a meta shard generation.
func (b *MetaBroadcaster) Send(generation uint64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, generation)
	arr := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(arr, buf)
	b.channel.Call("postMessage", arr.Get("buffer"))
}

// Close closes the BroadcastChannel.
func (b *MetaBroadcaster) Close() {
	b.channel.Call("close")
}
