import { test, expect } from '@playwright/test'

test.describe('SAB SharedWorker Relay', () => {
  test('relays SharedArrayBuffer between tabs through SharedWorker', async ({
    context,
  }) => {
    // Open sender tab.
    const sender = await context.newPage()
    await sender.goto('/sab-sharedworker-relay.html?role=sender')
    const senderLog = sender.locator('#log')
    await expect(senderLog).toContainText('stored', { timeout: 5000 })

    // Give SharedWorker time to store the SAB.
    await sender.waitForTimeout(200)

    // Open receiver tab.
    const receiver = await context.newPage()
    await receiver.goto('/sab-sharedworker-relay.html?role=receiver')
    const receiverLog = receiver.locator('#log')

    // Wait for receiver to complete.
    await expect(receiverLog).toContainText('DONE', { timeout: 10000 })

    const receiverResults = await receiver.evaluate(
      () => (window as any).__results,
    )
    console.log('Receiver results:', JSON.stringify(receiverResults, null, 2))
    console.log('Receiver log:\n' + (await receiverLog.textContent()))

    // Check if sender got the confirmation.
    await expect(senderLog).toContainText('DONE', { timeout: 10000 })
    const senderResults = await sender.evaluate(
      () => (window as any).__results,
    )
    console.log('Sender results:', JSON.stringify(senderResults, null, 2))
    console.log('Sender log:\n' + (await senderLog.textContent()))

    if (senderResults.sharedWorkerRelay && senderResults.sabShared) {
      console.log('FINDING: SharedWorker CAN relay SAB between tabs!')
    } else if (senderResults.sabDowngraded || receiverResults.receivedType) {
      const t = senderResults.receivedType || receiverResults.receivedType
      console.log(`FINDING: SAB downgraded to ${t} through SharedWorker relay`)
    } else if (receiverResults.noSab) {
      console.log('FINDING: SharedWorker lost the SAB reference')
    } else {
      console.log('FINDING: SharedWorker SAB relay did not work')
    }
  })
})
