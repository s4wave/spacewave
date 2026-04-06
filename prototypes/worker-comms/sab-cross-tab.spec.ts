import { test, expect } from '@playwright/test'

test.describe('SAB Cross-Tab Transfer', () => {
  test('shares SharedArrayBuffer between two tabs via BroadcastChannel', async ({
    context,
  }) => {
    // Open sender tab.
    const sender = await context.newPage()
    await sender.goto('/sab-cross-tab.html?role=sender')
    const senderLog = sender.locator('#log')
    await expect(senderLog).toContainText('Waiting for receiver tab', {
      timeout: 5000,
    })

    // Open receiver tab.
    const receiver = await context.newPage()
    await receiver.goto('/sab-cross-tab.html?role=receiver')

    // Wait for both to reach a terminal state (DONE).
    // Sender may finish first (if SAB send fails) or receiver first.
    await expect(senderLog).toContainText('DONE', { timeout: 15000 })

    // Check sender results.
    const senderResults = await sender.evaluate(
      () => (window as any).__results,
    )
    console.log('Sender results:', JSON.stringify(senderResults, null, 2))
    console.log('Sender log:\n' + (await senderLog.textContent()))

    if (!senderResults.broadcastChannelSAB) {
      // BroadcastChannel does not support SAB in this browser.
      // This is a valid experimental finding.
      console.log(
        'FINDING: BroadcastChannel does NOT support SharedArrayBuffer transfer',
      )
      console.log(`Error: ${senderResults.error}`)
      return
    }

    // If send succeeded, receiver should also be done.
    const receiverLog = receiver.locator('#log')
    await expect(receiverLog).toContainText('DONE', { timeout: 10000 })
    const receiverResults = await receiver.evaluate(
      () => (window as any).__results,
    )
    console.log('Receiver results:', JSON.stringify(receiverResults, null, 2))
    console.log('Receiver log:\n' + (await receiverLog.textContent()))

    expect(receiverResults.sabReceived).toBe(true)
    expect(receiverResults.magicCorrect).toBe(true)
    expect(senderResults.sabShared).toBe(true)
    console.log('FINDING: BroadcastChannel SUPPORTS SharedArrayBuffer transfer')
  })
})
