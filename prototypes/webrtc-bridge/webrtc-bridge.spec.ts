import { test, expect } from '@playwright/test'

async function runAndCollect(page: any) {
  await page.goto('/')
  const logEl = page.locator('#log')
  await expect(logEl).toContainText('DONE', { timeout: 15000 })
  const results = await page.evaluate(() => (window as any).__results)
  return results
}

test('seed: createPC command/response over MessagePort', async ({ page }) => {
  const results = await runAndCollect(page)
  const r = results['seed-rpc']
  expect(r.pass).toBe(true)
  expect(r.pcId).toBeTruthy()
  expect(r.snapshot.connectionState).toBe('new')
  expect(r.snapshot.signalingState).toBe('stable')
})

test('signaling: full offer/answer exchange through RPC', async ({ page }) => {
  const results = await runAndCollect(page)
  const r = results['signaling']
  expect(r.pass).toBe(true)
})

test('ice: candidate forwarding and connectivity', async ({ page }) => {
  const results = await runAndCollect(page)
  const r = results['ice-connectivity']
  expect(r.pass).toBe(true)
  expect(r.connectionState).toBe('connected')
})

test('dc-transfer: create, transfer, open, roundtrip data', async ({
  page,
}) => {
  const results = await runAndCollect(page)
  const r = results['dc-transfer']
  expect(r.pass).toBe(true)
  expect(r.offererLabel).toBe('xfer-test')
  expect(r.answererLabel).toBe('xfer-test')
  expect(r.textForward).toBe('hello from offerer')
  expect(r.textReverse).toBe('hello from answerer')
  expect(r.binarySize).toBe(5)
})

test('snapshot-cache: tracks PC state through lifecycle', async ({ page }) => {
  const results = await runAndCollect(page)
  const r = results['snapshot-cache']
  expect(r.pass).toBe(true)
  expect(r.initialOk).toBe(true)
  expect(r.connectedOk).toBe(true)
  expect(r.allFields).toBe(true)
  expect(r.hasDescs).toBe(true)
  expect(r.finalSnapshot.connectionState).toBe('connected')
})
