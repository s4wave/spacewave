import { test, expect } from '@playwright/test'

test.describe('Beforeunload Snapshot Race', () => {
  test('snapshots 16MB WASM memory to OPFS within timing budget', async ({
    page,
  }) => {
    await page.goto('/beforeunload-snapshot.html')
    const logEl = page.locator('#log')
    await expect(logEl).toContainText('DONE', { timeout: 30000 })

    const results = await page.evaluate(() => (window as any).__results)
    console.log('Results:', JSON.stringify(results, null, 2))

    expect(results.memMB).toBe(16)

    if (results.opfsAvailable === false) {
      // OPFS not available (e.g. WebKit in Playwright).
      console.log('OPFS not available, IDB fallback only')
      console.log(`IDB write: ${results.idbWriteMs.toFixed(2)} ms`)
    } else {
      console.log(`Worker snapshot: ${results.snapshotMs.toFixed(2)} ms`)
      console.log(`Total round-trip: ${results.totalRoundTripMs.toFixed(2)} ms`)
      console.log(`OPFS verified: ${results.opfsVerified}`)
      console.log(`IDB write: ${results.idbWriteMs.toFixed(2)} ms`)
      console.log(`Would need alert(): ${results.wouldNeedAlert}`)
    }

    console.log('\nFull output:\n' + (await logEl.textContent()))
  })
})
