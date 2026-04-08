//go:build js

package blockshard

import (
	"encoding/binary"
	"syscall/js"
)

// BroadcastChannelName is the channel name for shard generation invalidation.
const BroadcastChannelName = "hydra-blockshard-gen"

// InvalidationMsg is a shard generation invalidation message.
// Wire format: [shard_id: u16] [generation: u64] = 10 bytes.
type InvalidationMsg struct {
	ShardID    uint16
	Generation uint64
}

// Encode serializes the invalidation message to 10 bytes.
func (m *InvalidationMsg) Encode() []byte {
	buf := make([]byte, 10)
	binary.BigEndian.PutUint16(buf[0:2], m.ShardID)
	binary.BigEndian.PutUint64(buf[2:10], m.Generation)
	return buf
}

// DecodeInvalidationMsg parses a 10-byte invalidation message.
func DecodeInvalidationMsg(buf []byte) *InvalidationMsg {
	if len(buf) < 10 {
		return nil
	}
	return &InvalidationMsg{
		ShardID:    binary.BigEndian.Uint16(buf[0:2]),
		Generation: binary.BigEndian.Uint64(buf[2:10]),
	}
}

// Broadcaster sends shard generation invalidation messages over BroadcastChannel.
type Broadcaster struct {
	channel js.Value
}

// NewBroadcaster creates a BroadcastChannel for sending invalidation messages.
func NewBroadcaster() *Broadcaster {
	ch := js.Global().Get("BroadcastChannel").New(BroadcastChannelName)
	return &Broadcaster{channel: ch}
}

// Send broadcasts a shard generation invalidation.
func (b *Broadcaster) Send(shardID int, generation uint64) {
	msg := InvalidationMsg{
		ShardID:    uint16(shardID),
		Generation: generation,
	}
	data := msg.Encode()
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	b.channel.Call("postMessage", arr.Get("buffer"))
}

// Close closes the BroadcastChannel.
func (b *Broadcaster) Close() {
	b.channel.Call("close")
}

// Listener receives shard generation invalidation messages.
type Listener struct {
	channel js.Value
	msgs    chan InvalidationMsg
	cleanup js.Func
}

// NewListener creates a BroadcastChannel listener for invalidation messages.
func NewListener() *Listener {
	ch := js.Global().Get("BroadcastChannel").New(BroadcastChannelName)
	l := &Listener{
		channel: ch,
		msgs:    make(chan InvalidationMsg, 32),
	}
	l.cleanup = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		data := args[0].Get("data")
		if data.IsUndefined() || data.IsNull() {
			return nil
		}
		arr := js.Global().Get("Uint8Array").New(data)
		buf := make([]byte, arr.Get("length").Int())
		js.CopyBytesToGo(buf, arr)
		msg := DecodeInvalidationMsg(buf)
		if msg != nil {
			select {
			case l.msgs <- *msg:
			default:
			}
		}
		return nil
	})
	ch.Set("onmessage", l.cleanup)
	return l
}

// Messages returns the channel for receiving invalidation messages.
func (l *Listener) Messages() <-chan InvalidationMsg {
	return l.msgs
}

// Close closes the BroadcastChannel listener.
func (l *Listener) Close() {
	l.channel.Call("close")
	l.cleanup.Release()
}
