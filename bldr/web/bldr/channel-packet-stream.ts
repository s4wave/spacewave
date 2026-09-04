import { type ChannelStream, type PacketStream } from 'starpc'

// channelPacketStream supplies the PacketStream lifecycle contract while the
// ChannelStream remains responsible for transport teardown.
export function channelPacketStream(channel: ChannelStream): PacketStream {
  return {
    source: channel.source,
    sink: channel.sink,
    close: async () => channel.close(),
    abort: (error) => channel.close(error),
  }
}
