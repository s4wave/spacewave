//go:build js

package opfs

import (
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

const (
	bldrOPFSBroadcastChannelNew = "BLDR_OPFS_BROADCAST_CHANNEL_NEW"
	bldrOPFSBroadcastSend       = "BLDR_OPFS_BROADCAST_SEND"
	bldrOPFSBroadcastClose      = "BLDR_OPFS_BROADCAST_CLOSE"
)

// BroadcastMessage is the typed shard-generation invalidation payload.
type BroadcastMessage struct {
	ShardID    uint16
	Generation uint64
}

// NewBroadcastChannel creates a browser BroadcastChannel.
func NewBroadcastChannel(name string) (js.Value, error) {
	return DefaultDriver.NewBroadcastChannel(name)
}

// NewBroadcastChannel creates a browser BroadcastChannel.
func (BrowserDriver) NewBroadcastChannel(name string) (js.Value, error) {
	if name == "" {
		return js.Undefined(), errors.New("broadcast channel name required")
	}
	if useTinyGoBroadcastHelpers() {
		return newTinyGoBroadcastChannel(name), nil
	}

	newChannel := js.Global().Get(bldrOPFSBroadcastChannelNew)
	if jsFuncAvailable(newChannel) {
		return newChannel.Invoke(name), nil
	}
	broadcastChannel := js.Global().Get("BroadcastChannel")
	if broadcastChannel.IsUndefined() || broadcastChannel.IsNull() || broadcastChannel.Type() != js.TypeFunction {
		return js.Undefined(), errors.New("BroadcastChannel unavailable")
	}
	return broadcastChannel.New(name), nil
}

// SendBroadcastChannel posts a typed shard-generation invalidation payload.
func SendBroadcastChannel(channel js.Value, msg BroadcastMessage) error {
	return DefaultDriver.SendBroadcastChannel(channel, msg)
}

// SendBroadcastChannel posts a typed shard-generation invalidation payload.
func (BrowserDriver) SendBroadcastChannel(channel js.Value, msg BroadcastMessage) error {
	if channel.IsUndefined() || channel.IsNull() {
		return errors.New("broadcast channel unavailable")
	}
	if useTinyGoBroadcastHelpers() {
		sendTinyGoBroadcast(channel, int(msg.ShardID), msg.Generation)
		return nil
	}

	send := js.Global().Get(bldrOPFSBroadcastSend)
	if jsFuncAvailable(send) {
		send.Invoke(
			channel,
			int(msg.ShardID),
			int(uint32(msg.Generation>>32)),
			int(uint32(msg.Generation)),
		)
		return nil
	}

	arr := jsutil.NewUint8Array(10)
	arr.SetIndex(0, int(msg.ShardID>>8))
	arr.SetIndex(1, int(msg.ShardID))
	for i := 0; i < 8; i++ {
		shift := uint((7 - i) * 8)
		arr.SetIndex(2+i, int(byte(msg.Generation>>shift)))
	}
	jsutil.Call(channel, "postMessage", arr)
	return nil
}

// CloseBroadcastChannel closes a browser BroadcastChannel.
func CloseBroadcastChannel(channel js.Value) error {
	return DefaultDriver.CloseBroadcastChannel(channel)
}

// CloseBroadcastChannel closes a browser BroadcastChannel.
func (BrowserDriver) CloseBroadcastChannel(channel js.Value) error {
	if channel.IsUndefined() || channel.IsNull() {
		return nil
	}
	if useTinyGoBroadcastHelpers() {
		closeTinyGoBroadcastChannel(channel)
		return nil
	}

	closeChannel := js.Global().Get(bldrOPFSBroadcastClose)
	if jsFuncAvailable(closeChannel) {
		closeChannel.Invoke(channel)
		return nil
	}
	jsutil.Call(channel, "close")
	return nil
}

func jsFuncAvailable(fn js.Value) bool {
	return !fn.IsUndefined() && !fn.IsNull() && fn.Type() == js.TypeFunction
}
