import { describe, it, expect } from 'vitest'
import {
  AsyncOpfsDb,
  COMMS_BROADCAST_CHANNEL,
  type CommsNotification,
} from './async-opfs.js'

describe('AsyncOpfsDb constants', () => {
  it('exports COMMS_BROADCAST_CHANNEL', () => {
    expect(COMMS_BROADCAST_CHANNEL).toBe('bldr-comms-sqlite')
  })

  it('CommsNotification type is usable', () => {
    const n: CommsNotification = { table: 'messages', seq: 1 }
    expect(n.table).toBe('messages')
    expect(n.seq).toBe(1)
  })
})

// Full integration tests for AsyncOpfsDb require a browser environment with
// OPFS and sqlite.wasm loaded. These would run in Playwright browser tests.
// The unit tests above validate the module can be imported and types work.
