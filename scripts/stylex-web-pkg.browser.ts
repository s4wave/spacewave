import assert from 'node:assert/strict'
import { chromium, webkit } from 'playwright'

// Browser-imported web packages must wait for their native CSS dependency before publishing their exports.
for (const [engine, launcher] of Object.entries({ chromium, webkit })) {
  const browser = await launcher.launch()
  try {
    const page = await browser.newPage()
    page.setDefaultTimeout(15_000)
    page.on('pageerror', (error) =>
      console.error(`${engine}: ${error.message}`),
    )
    page.on('response', (response) => {
      if (response.status() >= 400)
        console.error(`${engine}: ${response.status()} ${response.url()}`)
    })
    let release!: () => void
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    const requested = page.waitForRequest((request) =>
      new URL(request.url()).pathname.endsWith('.css'),
    )
    await page.route('**/*.css', async (route) => {
      await held
      await route.continue()
    })

    // Holding the real stylesheet proves that module completion depends on its load, not on a timer.
    await page.goto(process.argv[2]!, { waitUntil: 'commit' })
    await requested
    assert.equal(
      await page.locator('#target').getAttribute('data-state'),
      'loading',
    )
    release()
    await page.locator('#target[data-state=ready]').waitFor()
    if (process.argv[3] === 'buttons') {
      const icon = page.locator('#native-button svg')
      await icon.waitFor()
      const actual = await icon.evaluate((element) => {
        const style = getComputedStyle(element)
        return {
          width: style.width,
          height: style.height,
          pointerEvents: style.pointerEvents,
          color: style.color,
        }
      })
      assert.deepEqual(actual, {
        width: '16px',
        height: '16px',
        pointerEvents: 'none',
        color: 'rgb(17, 34, 51)',
      })
      console.log(
        `${engine}: shared native button styles its caller's icon after CSS delivery`,
      )
      continue
    }
    const style = await page.locator('#target').evaluate((element) => {
      const style = getComputedStyle(element)
      return { color: style.color, padding: style.padding }
    })
    assert.deepEqual(style, { color: 'rgb(255, 0, 0)', padding: '4px' })
    const other = await page.locator('#other').evaluate((element) => {
      const style = getComputedStyle(element)
      return {
        backgroundColor: style.backgroundColor,
        marginTop: style.marginTop,
      }
    })
    assert.deepEqual(other, {
      backgroundColor: 'rgb(0, 0, 255)',
      marginTop: '7px',
    })
    console.log(`${engine}: native package CSS loaded before module readiness`)
  } finally {
    await browser.close()
  }
}
