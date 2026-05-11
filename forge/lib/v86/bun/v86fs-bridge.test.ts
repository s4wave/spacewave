import { describe, expect, it } from 'vitest'
import { pushable } from 'it-pushable'
import type { V86fsMessage as V86fsMessageType } from '@go/github.com/s4wave/spacewave/db/unixfs/v86fs/v86fs.pb.js'

import { createV86fsSrpcAdapterForClient } from './v86fs-bridge.js'

describe('createV86fsSrpcAdapter', () => {
  it('replies to pending callbacks when the response stream closes', async () => {
    const responses = pushable<V86fsMessageType>({ objectMode: true })
    const sent: V86fsMessageType[] = []
    const client = {
      RelayV86fs(outgoing: AsyncIterable<V86fsMessageType>) {
        void (async () => {
          for await (const msg of outgoing) {
            sent.push(msg)
          }
        })().catch(() => {})
        return responses
      },
    }
    const { adapter, close } = createV86fsSrpcAdapterForClient(client)
    const v86fsAdapter = adapter as {
      onLookup(
        parent_id: number,
        name: string,
        reply: (
          status: number,
          inode_id: number,
          mode: number,
          size: number,
        ) => void,
      ): void
    }

    const reply = new Promise<number[]>((resolve) => {
      v86fsAdapter.onLookup(1, 'missing', (...args) => resolve(args))
    })
    responses.end()

    await expect(reply).resolves.toEqual([5, 0, 0, 0])
    expect(sent).toHaveLength(1)
    close()
  })
})
