import { describe, expect, it, vi } from 'vitest'

import type { WorkerCommsDetectResult } from './worker-comms-detect.js'
import { createTransportFactory } from './plugin-transport.js'
import { SabRingStream, createSabPair } from './sab-ring-stream.js'

const caps = {
  crossOriginIsolated: true,
  sabAvailable: true,
  opfsAvailable: true,
  webLocksAvailable: true,
  broadcastChannelAvailable: true,
}

function detect(
  config: WorkerCommsDetectResult['config'],
): WorkerCommsDetectResult {
  return { config, caps }
}

describe('createTransportFactory pair streams', () => {
  it('exposes brokered pair streams only for SAB configs', async () => {
    const { aSab, bSab } = createSabPair()
    const closePairEndpoint = vi.fn()
    const factory = createTransportFactory(detect('C'), {
      openStream: async () => {
        throw new Error('not used')
      },
      handleIncomingStream: async () => {},
      openPairEndpoint: async () => ({
        pairId: 'sab-pair-1',
        localWorkerId: 'worker-a',
        remoteWorkerId: 'worker-b',
        txSab: aSab,
        rxSab: bSab,
        mtuBytes: 32 * 1024,
      }),
      closePairEndpoint,
    })

    expect(factory.openPairStream).toBeTypeOf('function')
    const localStream = await factory.openPairStream!('worker-b')
    const remoteStream = new SabRingStream(bSab, aSab)
    const payload = new TextEncoder().encode('pair stream')
    const writeDone = localStream.sink(
      (async function* () {
        yield payload
      })(),
    )

    const received = await remoteStream.source.next()
    expect(received.value).toEqual(payload)
    await writeDone

    const closeable = localStream as unknown as SabRingStream
    closeable.close()
    expect(closePairEndpoint).toHaveBeenCalledWith('sab-pair-1')
    remoteStream.close()
  })

  it('does not expose pair streams for MessagePort-only configs', () => {
    const factory = createTransportFactory(detect('A'), {
      openStream: async () => {
        throw new Error('not used')
      },
      handleIncomingStream: async () => {},
      openPairEndpoint: async () => {
        throw new Error('should not be called')
      },
    })

    expect(factory.openPairStream).toBeUndefined()
  })
})
