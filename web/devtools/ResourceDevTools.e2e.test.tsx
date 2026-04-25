/**
 * E2E tests for Resource DevTools.
 * Simplified to debug why tests aren't running.
 */
import { describe, test, expect, beforeAll } from 'vitest'
import {
  createE2EClient,
  getTestServerPort,
  type E2ETestClient,
} from '@s4wave/web/test/e2e-client.js'

describe('Resource DevTools E2E', () => {
  let client: E2ETestClient | undefined

  beforeAll(async () => {
    let port: number
    try {
      port = getTestServerPort()
    } catch {
      return
    }

    try {
      client = await createE2EClient(port)
    } catch (err) {
      console.warn('Failed to connect:', err)
    }
  })

  test('simple test runs', ({ skip }) => {
    skip(!client, 'No backend')
    expect(true).toBe(true)
  })
})
