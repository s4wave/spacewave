// transport.ts - Transport factory test fixture.
//
// Detects worker comms config, creates a transport factory with detected
// config, verifies correct transport availability per browser.

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import {
  createTransportFactory,
  type PluginTransportFactory,
} from '../../../web/bldr/plugin-transport.js'
import {
  SabBusEndpoint,
  createBusSab,
} from '../../../web/bldr/sab-bus.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      config: string
      hasBusStream: boolean
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
    let hasBusStream = false

    if (config === 'B' || config === 'C') {
      // SAB configs: provide a bus endpoint.
      const busOpts = { slotSize: 256, numSlots: 16 }
      const busSab = createBusSab(busOpts)
      const endpoint = new SabBusEndpoint(busSab, 0, busOpts)
      endpoint.register()

      factory = createTransportFactory(detect, {
        openStream: noopOpen,
        handleIncomingStream: noopHandle,
        busEndpoint: endpoint,
        pluginId: 0,
      })

      hasBusStream = factory.openBusStream != null
      endpoint.close()
    } else {
      // Config A/F: no bus, no cross-tab.
      factory = createTransportFactory(detect, {
        openStream: noopOpen,
        handleIncomingStream: noopHandle,
      })

      hasBusStream = factory.openBusStream != null
    }

    const factoryCreated = factory.config === config

    // Validate expectations per config.
    if (config === 'B' || config === 'C') {
      if (!hasBusStream) {
        errors.push('expected openBusStream on config ' + config)
      }
    } else {
      // Config A/F: no bus transport.
      if (hasBusStream) {
        errors.push('unexpected openBusStream on config ' + config)
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
      hasBusStream,
      factoryCreated,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      config: '',
      hasBusStream: false,
      factoryCreated: false,
    }
  }

  log.textContent = 'DONE'
}

run()
