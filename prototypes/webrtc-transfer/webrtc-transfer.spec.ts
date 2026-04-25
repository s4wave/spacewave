import { test, expect } from '@playwright/test'

// Wait for the page to finish all tests and collect results.
async function runAndCollect(page: any) {
  await page.goto('/webrtc-transfer.html')
  const logEl = page.locator('#log')
  await expect(logEl).toContainText('DONE', { timeout: 30000 })
  const results = await page.evaluate(() => (window as any).__results)
  const logText = await logEl.textContent()
  return { results, logText }
}

// These tests record findings per browser. DC transfer is expected to
// work in Chromium + WebKit but not Firefox. MessagePort pipe must work
// in all browsers (it's the universal fallback).

test.describe('RTCDataChannel early transfer (before open/send)', () => {
  test('early offerer DC (transferred before signaling)', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-early-offerer']
    console.log(`[${browserName}] dc-transfer-early-offerer: ${r?.pass ? 'PASS' : 'FAIL'} ${r?.error || ''}`)
    if (r?.pass) {
      console.log(`  readyState=${r.received?.readyState} opened=${r.opened} neutered=${r.neutered}`)
    }

    // Early transfer should work in all browsers
    expect(r?.pass).toBe(true)
    // Firefox does not neuter the original DC on transfer
    if (browserName !== 'firefox') {
      expect(r?.neutered).toBe(true)
    }
  })

  test('early answerer DC (transferred from ondatachannel before open)', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-early-answerer']
    console.log(`[${browserName}] dc-transfer-early-answerer: ${r?.pass ? 'PASS' : 'FAIL'} ${r?.error || ''}`)
    if (r?.pass) {
      console.log(`  readyState=${r.received?.readyState} opened=${r.opened}`)
    }

    // Early transfer should work in all browsers
    expect(r?.pass).toBe(true)
  })

  test('negotiated DC (bifrost pattern, transferred before signaling)', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-negotiated']
    console.log(`[${browserName}] dc-transfer-negotiated: ${r?.pass ? 'PASS' : 'FAIL'} ${r?.error || ''}`)
    if (r?.pass) {
      console.log(`  readyState=${r.received?.readyState} opened=${r.opened} neutered=${r.neutered}`)
    }

    // Negotiated DC transferred early should work in all browsers
    expect(r?.pass).toBe(true)
    // Firefox does not neuter the original DC on transfer
    if (browserName !== 'firefox') {
      expect(r?.neutered).toBe(true)
    }
  })
})

test.describe('RTCDataChannel direct transfer (after open)', () => {
  test('answerer DC with legacy [dc] syntax', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-legacy']
    console.log(`[${browserName}] dc-transfer-legacy: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)
    if (r.pass) {
      console.log(`  readyState=${r.received?.readyState} neutered=${r.neutered}`)
    }

    if (browserName === 'chromium' || browserName === 'webkit') {
      expect(r.pass).toBe(true)
      expect(['open', 'connecting']).toContain(r.received?.readyState)
      expect(r.neutered).toBe(true)
    }
    // Firefox: expected to fail, just log
  })

  test('answerer DC with {transfer: [dc]} syntax', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-options']
    console.log(`[${browserName}] dc-transfer-options: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)

    if (browserName === 'chromium' || browserName === 'webkit') {
      expect(r.pass).toBe(true)
      expect(['open', 'connecting']).toContain(r.received?.readyState)
    }
  })

  test('answerer DC as top-level message property', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-top-level']
    console.log(`[${browserName}] dc-transfer-top-level: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)

    if (browserName === 'chromium' || browserName === 'webkit') {
      expect(r.pass).toBe(true)
    }
  })

  test('offerer DC (created via createDataChannel + used)', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['dc-transfer-offerer']
    console.log(`[${browserName}] dc-transfer-offerer: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)
    // Expected to fail in all browsers: offerer DC is no longer
    // transferable after establishment. Chrome says explicitly:
    // "Transfers must occur on creation, and before any calls to send()."
  })

  test('RTCPeerConnection is NOT transferable', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['pc-transfer']
    console.log(`[${browserName}] pc-transfer: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)
    // RTCPeerConnection is expected to be non-transferable in all browsers.
    expect(r.pass).toBe(false)
  })
})

test.describe('MessagePort pipe (universal fallback)', () => {
  test('text + binary full duplex roundtrip', async ({
    page,
    browserName,
  }) => {
    const { results } = await runAndCollect(page)
    const r = results['msgport-pipe']
    console.log(`[${browserName}] msgport-pipe: ${r.pass ? 'PASS' : 'FAIL'} ${r.error || ''}`)

    // MessagePort pipe MUST work in all browsers.
    expect(r.pass).toBe(true)
    expect(r.textRoundTrip).toBe(true)
    expect(r.binaryRoundTrip).toBe(true)
  })
})

test.describe('API availability', () => {
  test('main thread has required APIs', async ({ page, browserName }) => {
    const { results } = await runAndCollect(page)
    const apis = results['api-availability']
    console.log(`[${browserName}] APIs:`, JSON.stringify(apis))

    expect(apis.RTCPeerConnection).toBe(true)
    expect(apis.RTCDataChannel).toBe(true)
    expect(apis.MessageChannel).toBe(true)
    expect(apis.Worker).toBe(true)
  })
})

test.describe('Cross-browser summary', () => {
  test('full results log', async ({ page, browserName }) => {
    const { results, logText } = await runAndCollect(page)

    const tests = [
      'dc-transfer-early-offerer',
      'dc-transfer-early-answerer',
      'dc-transfer-negotiated',
      'dc-transfer-legacy',
      'dc-transfer-options',
      'dc-transfer-top-level',
      'dc-transfer-offerer',
      'pc-transfer',
      'msgport-pipe',
    ]

    console.log(`\n=== ${browserName.toUpperCase()} ===`)
    for (const t of tests) {
      const r = results[t]
      const status = r?.pass ? 'PASS' : 'FAIL'
      console.log(`  ${t.padEnd(25)} ${status} ${r?.error ? '(' + r.error + ')' : ''}`)
    }
  })
})
