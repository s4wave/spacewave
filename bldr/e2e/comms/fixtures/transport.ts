// transport.ts - Transport factory test fixture.
//
// Detects worker comms config, creates a transport factory with detected
// config, verifies correct transport availability per browser.

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import {
  createTransportFactory,
  type PluginTransportFactory,
} from '../../../web/bldr/plugin-transport.js'
import { createSabPair } from '../../../web/bldr/sab-ring-stream.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      config: string
      hasPairStream: boolean
      factoryCreated: boolean
    }
  }
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    // Detect config.
    const detect = await detectWorkerCommsConfig()
    const config = detect.config

    // Noop stream functions for the factory.
    const noopOpen = async () => {
      throw new Error('not implemented')
    }
    const noopHandle = async () => {}

    // Create the factory based on config.
    let factory: PluginTransportFactory
    let hasPairStream = false

    if (config === 'B' || config === 'C') {
      const { aSab, bSab } = createSabPair()
      factory = createTransportFactory(detect, {
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
    } else {
      factory = createTransportFactory(detect, {
        openStream: noopOpen,
        handleIncomingStream: noopHandle,
      })

      hasPairStream = factory.openPairStream != null
    }

    const factoryCreated = factory.config === config

    // Validate expectations per config.
    if (config === 'B' || config === 'C') {
      if (!hasPairStream) {
        errors.push('expected openPairStream on config ' + config)
      }
    } else {
      if (hasPairStream) {
        errors.push('unexpected openPairStream on config ' + config)
      }
    }

    if (!factoryCreated) {
      errors.push(`factory config mismatch: ${factory.config} vs ${config}`)
    }

    const pass = errors.length === 0 && factoryCreated
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      config,
      hasPairStream,
      factoryCreated,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      config: '',
      hasPairStream: false,
      factoryCreated: false,
    }
  }

  log.textContent = 'DONE'
}

run()
