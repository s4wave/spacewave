import { describe, expect, it } from 'vitest'

import {
  buildSessionStorageStatsView,
  setupBannerStorageThresholdBytes,
} from './SessionStorageStatsContext.js'

describe('buildSessionStorageStatsView', () => {
  it('stays ineligible while the first storage snapshot is loading', () => {
    const view = buildSessionStorageStatsView(null, true)

    expect(view.loading).toBe(true)
    expect(view.setupBannerEligible).toBe(false)
  })

  it('stays ineligible when the provider does not support storage stats', () => {
    const view = buildSessionStorageStatsView(
      { supported: false, totalBytes: 50n * 1024n * 1024n, blockCount: 9n },
      false,
    )

    expect(view.supported).toBe(false)
    expect(view.setupBannerEligible).toBe(false)
  })

  it('requires more than ten MiB before the setup banner can render', () => {
    const threshold = buildSessionStorageStatsView(
      {
        supported: true,
        totalBytes: setupBannerStorageThresholdBytes,
        blockCount: 1n,
      },
      false,
    )
    const overThreshold = buildSessionStorageStatsView(
      {
        supported: true,
        totalBytes: setupBannerStorageThresholdBytes + 1n,
        blockCount: 2n,
      },
      false,
    )

    expect(threshold.setupBannerEligible).toBe(false)
    expect(overThreshold.setupBannerEligible).toBe(true)
  })
})
