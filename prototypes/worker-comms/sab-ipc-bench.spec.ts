import { test, expect } from '@playwright/test'

test.describe('SAB IPC Benchmark', () => {
  test('runs throughput and latency benchmarks', async ({ page }) => {
    // Atomics.wait is not available on the main thread in some browsers.
    // The benchmark uses it for the SAB ping-pong test. Firefox and Safari
    // may not support Atomics.wait on the main thread, so we check.
    await page.goto('/sab-ipc-bench.html')

    // Wait for the benchmarks to complete (look for "DONE" in the log).
    const logEl = page.locator('#log')
    await expect(logEl).toContainText('DONE', { timeout: 60000 })

    // Extract results.
    const results = await page.evaluate(() => (window as any).__results)
    console.log('Results:', JSON.stringify(results, null, 2))

    if (results.error) {
      // Not cross-origin isolated (some browser configs).
      test.skip(true, `Skipped: ${results.error}`)
      return
    }

    // Verify structure.
    expect(results.roundTrip).toBeDefined()
    expect(results.throughput1k).toBeDefined()
    expect(results.throughput64k).toBeDefined()

    // Report speedups (these vary wildly by browser).
    const rtSpeedup =
      results.roundTrip.messagePort.latencyUs /
      results.roundTrip.sab.latencyUs
    console.log(`Round-trip speedup: ${rtSpeedup.toFixed(1)}x`)

    const tp1kSpeedup =
      results.throughput1k.sab.throughput /
      results.throughput1k.messagePort.throughput
    console.log(`1KB throughput speedup: ${tp1kSpeedup.toFixed(1)}x`)

    const tp64kSpeedup =
      results.throughput64k.sab.throughput /
      results.throughput64k.messagePort.throughput
    console.log(`64KB throughput speedup: ${tp64kSpeedup.toFixed(1)}x`)

    // Log the full output for analysis.
    const logText = await logEl.textContent()
    console.log('Benchmark output:\n' + logText)
  })
})
