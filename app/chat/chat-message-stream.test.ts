import { describe, expect, it } from 'vitest'

import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'

import { accumulateChatMessages } from './chat-message-stream.js'

async function* batches(...values: ChatMessageInfo[][]) {
  for (const value of values) yield value
}

describe('accumulateChatMessages', () => {
  it('preserves adjacent batches and removes replayed messages', async () => {
    const snapshots: ChatMessageInfo[][] = []
    for await (const snapshot of accumulateChatMessages(
      batches(
        [{ objectKey: 'message/1' }],
        [{ objectKey: 'message/2' }],
        [{ objectKey: 'message/1' }, { objectKey: 'message/3' }],
      ),
    )) {
      snapshots.push(snapshot)
    }
    expect(snapshots.at(-1)?.map((message) => message.objectKey)).toEqual([
      'message/1',
      'message/2',
      'message/3',
    ])
  })
})
