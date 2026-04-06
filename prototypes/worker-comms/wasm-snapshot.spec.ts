import { test, expect } from '@playwright/test'

test.describe('WASM Memory Snapshot/Restore', () => {
  test('snapshots and restores WASM memory with checksum verification', async ({
    page,
  }) => {
    await page.goto('/wasm-snapshot.html')
    const logEl = page.locator('#log')
    await expect(logEl).toContainText('DONE', { timeout: 30000 })

    const results = await page.evaluate(() => (window as any).__results)
    console.log('Results:', JSON.stringify(results, null, 2))

    // Checksum must match after restore.
    expect(results.checksumMatch).toBe(true)

    // Snapshot and restore should be fast.
    console.log(`Snapshot: ${results.snapshotMs.toFixed(2)} ms for ${results.memoryKB} KB`)
    console.log(`Restore: ${results.restoreMs.toFixed(2)} ms`)

    if (results.opfsAvailable) {
      console.log(`OPFS write: ${results.opfsWriteMs.toFixed(2)} ms`)
      console.log(`OPFS read: ${results.opfsReadMs.toFixed(2)} ms`)
    } else {
      console.log('OPFS: not available')
    }

    console.log(`IndexedDB write: ${results.idbWriteMs.toFixed(2)} ms`)

    // Memory size scaling.
    for (const s of results.scaling) {
      console.log(
        `${s.mb} MB: snapshot ${s.snapshotMs.toFixed(1)} ms, ` +
          `restore ${s.restoreMs.toFixed(1)} ms`,
      )
    }

    console.log('\nFull output:\n' + (await logEl.textContent()))
  })
})
