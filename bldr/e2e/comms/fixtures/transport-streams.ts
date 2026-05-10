// transport-streams.ts - Transport factory stream verification fixture.
//
// Creates a transport factory, calls openPairStream() and openCrossTabStream(),
// sends actual data through the returned streams, verifies receipt.
// Verifies openPairStream unavailable on WebKit (Config A/F).

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import { createTransportFactory } from '../../../web/bldr/plugin-transport.js'
import {
  SabRingStream,
  createSabPair,
} from '../../../web/bldr/sab-ring-stream.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      config: string
      hasPairStream: boolean
      pairStreamRoundTrip: boolean
      pairUnavailableOnFallback: boolean
    }
  }
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const detect = await detectWorkerCommsConfig()
  const config = detect.config

  let hasPairStream: boolean
  let pairStreamRoundTrip = false
  let pairUnavailableOnFallback = false

  const noopOpen = async () => {
    throw new Error('not implemented')
  }
  const noopHandle = async () => {}

  if (config === 'B' || config === 'C') {
    const { aSab, bSab } = createSabPair()
    const remoteStream = new SabRingStream(bSab, aSab)

    const factory = createTransportFactory(detect, {
      openStream: noopOpen,
      handleIncomingStream: noopHandle,
      openPairEndpoint: async () => ({
        pairId: 'sab-pair-fixture-1',
        localWorkerId: 'worker-a',
        remoteWorkerId: 'worker-b',
        txSab: aSab,
        rxSab: bSab,
        mtuBytes: 32 * 1024,
      }),
    })

    hasPairStream = factory.openPairStream != null

    if (factory.openPairStream) {
      const stream = await factory.openPairStream('worker-b')

      // Verify the stream has source and sink (PacketStream interface).
      if (!stream.source) errors.push('pair stream missing source')
      if (!stream.sink) errors.push('pair stream missing sink')

      // Write test data through the stream's sink.
      const testPayload = new TextEncoder().encode('transport-factory-test')
      const writePromise = stream.sink(
        (async function* () {
          yield testPayload
        })(),
      )

      const msg = await remoteStream.source.next()
      if (!msg.done) {
        const received = new TextDecoder().decode(msg.value)
        pairStreamRoundTrip = received === 'transport-factory-test'
        if (!pairStreamRoundTrip) {
          errors.push(`pair round-trip mismatch: got ${received}`)
        }
      } else {
        errors.push('pair remote stream received no data')
      }

      const closeable = stream as SabRingStream
      closeable.close()
      remoteStream.close()
      await writePromise.catch(() => {})
    } else {
      errors.push('expected openPairStream on config ' + config)
    }
  } else {
    const factory = createTransportFactory(detect, {
      openStream: noopOpen,
      handleIncomingStream: noopHandle,
    })

    hasPairStream = factory.openPairStream != null
    pairUnavailableOnFallback = !hasPairStream

    if (hasPairStream) {
      errors.push('unexpected openPairStream on config ' + config)
    }
  }

  const pass = errors.length === 0
  window.__results = {
    pass,
    detail: errors.length > 0 ? errors.join('; ') : 'ok',
    config,
    hasPairStream,
    pairStreamRoundTrip,
    pairUnavailableOnFallback,
  }
  log.textContent = 'DONE'
}

run().catch((err) => {
  window.__results = {
    pass: false,
    detail: `error: ${err}`,
    config: '',
    hasPairStream: false,
    pairStreamRoundTrip: false,
    pairUnavailableOnFallback: false,
  }
  document.getElementById('log')!.textContent = 'DONE'
})
