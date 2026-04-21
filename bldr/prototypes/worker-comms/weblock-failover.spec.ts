import { test, expect } from '@playwright/test'

test.describe('WebLock Failover', () => {
  test('waiter acquires lock after holder tab closes', async ({ context }) => {
    // Open holder tab.
    const holder = await context.newPage()
    await holder.goto('/weblock-failover.html?role=holder')
    const holderLog = holder.locator('#log')
    await expect(holderLog).toContainText('Lock acquired', { timeout: 5000 })

    // Give holder time to broadcast SAB and start writing.
    await holder.waitForTimeout(200)

    // Open waiter tab.
    const waiter = await context.newPage()
    await waiter.goto('/weblock-failover.html?role=waiter')
    const waiterLog = waiter.locator('#log')
    await expect(waiterLog).toContainText('Requesting lock', { timeout: 5000 })

    // Give waiter time to register the lock request.
    await waiter.waitForTimeout(200)

    // Close holder tab, releasing the lock.
    const closeStart = Date.now()
    await holder.close()
    const closeMs = Date.now() - closeStart
    console.log(`Holder tab closed in ${closeMs} ms`)

    // Waiter should acquire the lock.
    await expect(waiterLog).toContainText('DONE', { timeout: 10000 })

    const results = await waiter.evaluate(() => (window as any).__results)
    console.log('Waiter results:', JSON.stringify(results, null, 2))

    expect(results.lockAcquired).toBe(true)
    console.log(`Lock acquisition latency: ${results.waitMs.toFixed(1)} ms`)

    if (results.sabShared) {
      console.log(
        `SAB cross-tab via BroadcastChannel: YES (last counter: ${results.lastHolderCounter})`,
      )
    } else {
      console.log('SAB cross-tab via BroadcastChannel: NO')
    }

    console.log('Waiter log:\n' + (await waiterLog.textContent()))
  })
})
