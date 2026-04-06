import { describe, it, expect } from 'vitest'
import {
  COMMS_DB_FILENAME,
  type CommsMessage,
} from './comms-table.js'

describe('comms-table constants', () => {
  it('exports COMMS_DB_FILENAME', () => {
    expect(COMMS_DB_FILENAME).toBe('comms.db')
  })

  it('CommsMessage type is usable', () => {
    const msg: CommsMessage = {
      id: 1,
      sourcePluginId: 10,
      targetPluginId: 20,
      payload: new Uint8Array([1, 2, 3]),
      createdAt: 1712345678,
    }
    expect(msg.id).toBe(1)
    expect(msg.sourcePluginId).toBe(10)
    expect(msg.targetPluginId).toBe(20)
    expect(msg.payload).toEqual(new Uint8Array([1, 2, 3]))
  })
})

// Full round-trip tests (CommsWriter -> BroadcastChannel -> CommsReader)
// require sqlite.wasm in a browser environment. See Playwright tests.
