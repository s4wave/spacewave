import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'

// accumulateChatMessages projects ordered WatchMessages deltas into complete snapshots.
export async function* accumulateChatMessages(
  batches: AsyncIterable<ChatMessageInfo[]>,
): AsyncGenerator<ChatMessageInfo[]> {
  const messages: ChatMessageInfo[] = []
  const objectKeys = new Set<string>()
  for await (const batch of batches) {
    for (const message of batch) {
      const objectKey = message.objectKey ?? ''
      if (objectKey && objectKeys.has(objectKey)) continue
      if (objectKey) objectKeys.add(objectKey)
      messages.push(message)
    }
    yield [...messages]
  }
}
