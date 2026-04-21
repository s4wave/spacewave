import { test, expect } from '@playwright/test'

test.describe('SAB Cross-Tab via window.postMessage', () => {
  test('transfers SharedArrayBuffer between parent and child window', async ({
    page,
    context,
  }) => {
    // Grant popup permissions.
    await context.grantPermissions([])

    // Navigate to sender page. It will window.open() a child.
    await page.goto('/sab-window-post.html?role=sender')

    // Wait for the child window to be opened.
    const childPromise = context.waitForEvent('page')

    const logEl = page.locator('#log')

    // The sender either completes (popup works) or reports popup blocked.
    // Wait for child page or timeout.
    let child: any = null
    try {
      child = await Promise.race([
        childPromise,
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error('no child')), 3000),
        ),
      ])
    } catch {
      // Popup was blocked.
    }

    // Wait for sender to reach terminal state.
    await expect(logEl).toContainText('DONE', { timeout: 10000 })

    const results = await page.evaluate(() => (window as any).__results)
    console.log('Results:', JSON.stringify(results, null, 2))
    console.log('Sender log:\n' + (await logEl.textContent()))

    if (results.error === 'popup blocked') {
      console.log('FINDING: window.open() blocked by browser/Playwright')
      return
    }

    if (results.windowPostSAB) {
      if (results.sabShared) {
        console.log('FINDING: window.postMessage SUPPORTS SharedArrayBuffer transfer!')
      } else {
        console.log('FINDING: window.postMessage sent SAB but sharing failed')
        console.log(`  Response: ${results.responseReceived}`)
      }
    } else if (results.sabDowngraded) {
      console.log(
        `FINDING: SAB downgraded to ${results.receivedType} during window.postMessage`,
      )
    } else if (results.timeout) {
      console.log('FINDING: window.postMessage SAB transfer timed out (likely dropped)')
    } else {
      console.log('FINDING: window.postMessage SAB transfer failed:', results.error)
    }

    if (child) {
      const childLog = child.locator('#log')
      try {
        const childText = await childLog.textContent({ timeout: 2000 })
        console.log('Child log:\n' + childText)
      } catch {}
    }
  })
})
