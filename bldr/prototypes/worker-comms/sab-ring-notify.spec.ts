import { test, expect } from '@playwright/test'

test.describe('SAB Ring Buffer Notify Mode', () => {
  test('compares timeout vs notify-only Atomics.wait strategies', async ({
    page,
  }) => {
    await page.goto('/sab-ring-notify.html')
    const logEl = page.locator('#log')
    await expect(logEl).toContainText('DONE', { timeout: 120000 })

    const results = await page.evaluate(() => (window as any).__results)
    console.log('Results:', JSON.stringify(results, null, 2))

    if (results.error) {
      console.log(`Skipped: ${results.error}`)
      return
    }

    for (const [label, data] of Object.entries(results) as any) {
      console.log(`\n${label}:`)
      console.log(`  Timeout mode:  ${(data.timeout.throughput / 1e6).toFixed(1)} MB/s`)
      console.log(`  Notify mode:   ${(data.notify.throughput / 1e6).toFixed(1)} MB/s`)
      console.log(`  Notify/Timeout: ${data.speedup.toFixed(1)}x`)
    }

    console.log('\nFull output:\n' + (await logEl.textContent()))
  })
})
