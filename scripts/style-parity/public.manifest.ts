import type { ParityManifest } from './capture.js'

// publicManifest starts parity on static routes before adding account fixtures.
const publicManifest: ParityManifest = {
  states: [
    {
      id: 'landing',
      path: '/',
      ready: 'h1:not(.sw-initial-title):not(.swl-title)',
    },
    {
      id: 'pricing',
      path: '/pricing',
      ready: async (page) => {
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
        await page
          .getByRole('heading', {
            level: 1,
            name: 'Download Spacewave',
            exact: true,
          })
          .waitFor()
      },
    },
  ],
}

export default publicManifest
