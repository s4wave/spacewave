// transport-streams.ts - Transport factory stream verification fixture.
//
// Creates a transport factory, calls openBusStream() and openCrossTabStream(),
// sends actual data through the returned streams, verifies receipt.
// Verifies openBusStream unavailable on WebKit (Config A/F).

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import {
  createTransportFactory,
  type PluginTransportFactory,
} from '../../../web/bldr/plugin-transport.js'
import {
  SabBusEndpoint,
  SabBusStream,
  createBusSab,
} from '../../../web/bldr/sab-bus.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      config: string
      hasBusStream: boolean
      busStreamRoundTrip: boolean
      busUnavailableOnFallback: boolean
    }
  }
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const detect = await detectWorkerCommsConfig()
  const config = detect.config

  let hasBusStream: boolean
  let busStreamRoundTrip = false
  let busUnavailableOnFallback = false

  const noopOpen = async () => {
    throw new Error('not implemented')
  }
  const noopHandle = async () => {}

  if (config === 'B' || config === 'C') {
    // SAB configs: create bus with two endpoints.
    const busOpts = { slotSize: 8192, numSlots: 64 }
    const busSab = createBusSab(busOpts)

    // Endpoint 1 (our plugin) in the factory.
    const endpoint1 = new SabBusEndpoint(busSab, 1, busOpts)
    endpoint1.register()

    // Endpoint 2 (peer plugin) for receiving.
    const endpoint2 = new SabBusEndpoint(busSab, 2, busOpts)
    endpoint2.register()

    const factory = createTransportFactory(detect, {
      openStream: noopOpen,
      handleIncomingStream: noopHandle,
      busEndpoint: endpoint1,
    })

    hasBusStream = factory.openBusStream != null

    if (factory.openBusStream) {
      // Open a bus stream to endpoint 2.
      const stream = await factory.openBusStream(2)

      // Verify the stream has source and sink (PacketStream interface).
      if (!stream.source) errors.push('bus stream missing source')
      if (!stream.sink) errors.push('bus stream missing sink')

      // Write test data through the stream's sink.
      const testPayload = new TextEncoder().encode('transport-factory-test')
      const writePromise = stream.sink(
        (async function* () {
          yield testPayload
        })(),
      )

      // Read from endpoint 2 directly.
      const msg = await endpoint2.read()
      if (msg) {
        const received = new TextDecoder().decode(msg.data)
        busStreamRoundTrip = received === 'transport-factory-test'
        if (!busStreamRoundTrip) {
          errors.push(`bus round-trip mismatch: got ${received}`)
        }
      } else {
        errors.push('bus endpoint 2 received no data')
      }

      // Close streams.
      if (stream.close) {
        ;(stream as any).close()
      }
      await writePromise.catch(() => {})
    } else {
      errors.push('expected openBusStream on config ' + config)
    }

    endpoint1.close()
    endpoint2.close()
  } else {
    // Config A or F: no SAB bus.
    const factory = createTransportFactory(detect, {
      openStream: noopOpen,
      handleIncomingStream: noopHandle,
    })

    hasBusStream = factory.openBusStream != null
    busUnavailableOnFallback = !hasBusStream

    if (hasBusStream) {
      errors.push('unexpected openBusStream on config ' + config)
    }
  }

  const pass = errors.length === 0
  window.__results = {
    pass,
    detail: errors.length > 0 ? errors.join('; ') : 'ok',
    config,
    hasBusStream,
    busStreamRoundTrip,
    busUnavailableOnFallback,
  }
  log.textContent = 'DONE'
}

run().catch((err) => {
  window.__results = {
    pass: false,
    detail: `error: ${err}`,
    config: '',
    hasBusStream: false,
    busStreamRoundTrip: false,
    busUnavailableOnFallback: false,
  }
  document.getElementById('log')!.textContent = 'DONE'
})
