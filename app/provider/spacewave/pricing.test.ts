import { describe, expect, it } from 'vitest'

import { CLOUD_FEATURES, FREE_FEATURES } from './pricing.js'

describe('spacewave pricing copy', () => {
  it('frames the free tier as the full local-first app', () => {
    expect(FREE_FEATURES[0]).toBe('Full local-first app on your devices')
  })

  it('frames the paid tier as added cloud services', () => {
    expect(CLOUD_FEATURES[0]).toBe(
      'Adds cloud sync, storage, backup, and relay services:',
    )
  })
})
