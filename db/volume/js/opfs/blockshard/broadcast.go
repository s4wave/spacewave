//go:build js

package blockshard

import (
	"encoding/binary"
	"sync"
	"syscall/js"
)

// BroadcastChannelName is the base channel name for shard generation invalidation.
const BroadcastChannelName = "hydra-blockshard-gen"

const (
	bldrOPFSBroadcastChannelNew = "BLDR_OPFS_BROADCAST_CHANNEL_NEW"
	bldrOPFSBroadcastSend       = "BLDR_OPFS_BROADCAST_SEND"
	bldrOPFSBroadcastClose      = "BLDR_OPFS_BROADCAST_CLOSE"
)

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
func NewBroadcaster(scope string) *Broadcaster {
	ch := newBroadcastChannel(scope)
	return &Broadcaster{channel: ch}
}

// Send broadcasts a shard generation invalidation.
func (b *Broadcaster) Send(shardID int, generation uint64) {
	msg := InvalidationMsg{
		ShardID:    uint16(shardID),
		Generation: generation,
	}
	send := js.Global().Get(bldrOPFSBroadcastSend)
	if jsFuncAvailable(send) {
		send.Invoke(
			b.channel,
			int(msg.ShardID),
			int(uint32(msg.Generation>>32)),
			int(uint32(msg.Generation)),
		)
		return
	}

	arr := js.Global().Get("Uint8Array").New(10)
	arr.SetIndex(0, int(msg.ShardID>>8))
	arr.SetIndex(1, int(msg.ShardID))
	for i := 0; i < 8; i++ {
		shift := uint((7 - i) * 8)
		arr.SetIndex(2+i, int(byte(msg.Generation>>shift)))
	}
	b.channel.Call("postMessage", arr)
}

// Close closes the BroadcastChannel.
func (b *Broadcaster) Close() {
	closeBroadcastChannel(b.channel)
}

// Listener receives shard generation invalidation messages.
type Listener struct {
	channel js.Value
	mu      sync.Mutex
	pending map[uint16]uint64
	notify  chan struct{}
	cleanup js.Func
}

// NewListener creates a BroadcastChannel listener for invalidation messages.
func NewListener(scope string) *Listener {
	ch := newBroadcastChannel(scope)
	l := &Listener{
		channel: ch,
		pending: make(map[uint16]uint64),
		notify:  make(chan struct{}, 1),
	}
	l.cleanup = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		data := args[0].Get("data")
		if data.IsUndefined() || data.IsNull() {
			return nil
		}
		payload, length, ok := invalidationPayloadBytes(data)
		if !ok {
			return nil
		}
		if length < 10 {
			return nil
		}
		msg := &InvalidationMsg{
			ShardID: uint16(byte(payload.Index(0).Int()))<<8 |
				uint16(byte(payload.Index(1).Int())),
			Generation: uint64(byte(payload.Index(2).Int()))<<56 |
				uint64(byte(payload.Index(3).Int()))<<48 |
				uint64(byte(payload.Index(4).Int()))<<40 |
				uint64(byte(payload.Index(5).Int()))<<32 |
				uint64(byte(payload.Index(6).Int()))<<24 |
				uint64(byte(payload.Index(7).Int()))<<16 |
				uint64(byte(payload.Index(8).Int()))<<8 |
				uint64(byte(payload.Index(9).Int())),
		}
		l.mu.Lock()
		if msg.Generation > l.pending[msg.ShardID] {
			l.pending[msg.ShardID] = msg.Generation
		}
		l.mu.Unlock()
		select {
		case l.notify <- struct{}{}:
		default:
		}
		return nil
	})
	ch.Set("onmessage", l.cleanup)
	return l
}

func invalidationPayloadBytes(data js.Value) (js.Value, int, bool) {
	length := data.Get("length")
	if !length.IsUndefined() && !length.IsNull() {
		return data, length.Int(), true
	}
	byteLength := data.Get("byteLength")
	if byteLength.IsUndefined() || byteLength.IsNull() {
		return js.Value{}, 0, false
	}
	// Browser/runtime versions have posted both Uint8Array and raw ArrayBuffer
	// payloads. Normalize to byte indexing so old tabs keep receiving invalidations.
	return js.Global().Get("Uint8Array").New(data), byteLength.Int(), true
}

// Notify returns the wakeup channel for invalidation processing.
func (l *Listener) Notify() <-chan struct{} {
	return l.notify
}

// DrainPending returns the latest pending generation per shard and clears it.
func (l *Listener) DrainPending() []InvalidationMsg {
	l.mu.Lock()
	defer l.mu.Unlock()

	msgs := make([]InvalidationMsg, 0, len(l.pending))
	for shardID, generation := range l.pending {
		msgs = append(msgs, InvalidationMsg{
			ShardID:    shardID,
			Generation: generation,
		})
	}
	clear(l.pending)
	return msgs
}

// Close closes the BroadcastChannel listener.
func (l *Listener) Close() {
	closeBroadcastChannel(l.channel)
	l.cleanup.Release()
}

func newBroadcastChannel(scope string) js.Value {
	newChannel := js.Global().Get(bldrOPFSBroadcastChannelNew)
	name := scopedBroadcastChannelName(scope)
	if jsFuncAvailable(newChannel) {
		return newChannel.Invoke(name)
	}
	return js.Global().Get("BroadcastChannel").New(name)
}

func scopedBroadcastChannelName(scope string) string {
	if scope == "" {
		return BroadcastChannelName
	}
	// Lock prefixes identify the OPFS blockshard storage owner. Broadcasts stay
	// within that owner so another engine on the same origin cannot advance this
	// engine's shard generations and force refreshes on every read.
	return BroadcastChannelName + ":" + scope
}

func closeBroadcastChannel(channel js.Value) {
	closeChannel := js.Global().Get(bldrOPFSBroadcastClose)
	if jsFuncAvailable(closeChannel) {
		closeChannel.Invoke(channel)
		return
	}
	channel.Call("close")
}

func jsFuncAvailable(fn js.Value) bool {
	return !fn.IsUndefined() && !fn.IsNull()
}
