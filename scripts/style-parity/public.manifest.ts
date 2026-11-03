import type { ParityManifest } from './capture.js'

// publicManifest starts parity on static routes before adding account fixtures.
const publicManifest: ParityManifest = {
  states: [
    {
      id: 'landing',
      path: '/',
      ready: async (page) => {
        // Require the actual page heading before waiting on its hydration effects.
        await page.waitForFunction(() => '__swPrerenderRoot' in globalThis)
        await page
          .locator('h1:not(.sw-initial-title):not(.swl-title)')
          .first()
          .waitFor()

        // The heading is already in server HTML; wait for the logo's visibility and pointer effects too.
        await page.locator('.animated-logo-blur').first().waitFor()
        await page.mouse.move(1, 1)
        await page.waitForFunction(() => {
          if (!matchMedia('(hover: hover) and (pointer: fine)').matches)
            return true
          const transform = document
            .querySelector<HTMLElement>('[style*="--animated-logo-transform"]')
            ?.style.getPropertyValue('--animated-logo-transform')
          return (
            !!transform && transform !== 'rotateX(0deg) rotateY(0deg) scale(1)'
          )
        })
      },
    },
    {
      id: 'pricing',
      path: '/pricing',
      ready: async (page) => {
        await page.waitForFunction(() => '__swPrerenderRoot' in globalThis)
        await page
          .getByRole('heading', {
            level: 1,
            name: 'Spacewave Pricing',
            exact: true,
          })
          .waitFor()
      },
    },
    {
      id: 'download',
      path: '/download',
      ready: async (page) => {
        await page.waitForFunction(() => '__swPrerenderRoot' in globalThis)
        await page
          .getByRole('heading', {
            level: 1,
            name: 'Download Spacewave',
            exact: true,
          })
          .waitFor()
        await page
          .getByRole('link', { name: /^Download.* for (macOS|Windows|Linux)/ })
          .first()
          .waitFor()
      },
    },
  ],
}

export default publicManifest
