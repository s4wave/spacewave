import type { ParityManifest } from './capture.js'

// routesManifest covers public page families and signed-out account screens.
const routesManifest: ParityManifest = {
  states: [
    ...[
      '/',
      '/landing',
      '/landing/drive',
      '/landing/chat',
      '/landing/devices',
      '/landing/plugins',
      '/landing/notes',
      '/landing/cli',
      '/landing/hydra',
      '/landing/bifrost',
      '/landing/controllerbus',
      '/community',
      '/tos',
      '/privacy',
      '/pricing',
      '/dmca',
      '/licenses',
      '/download',
      '/download/cli',
      '/changelog',
      '/docs',
      '/blog',
    ].map((path) => ({
      id: path === '/' ? 'home' : path.slice(1).replaceAll('/', '-'),
      path: path === '/docs' || path === '/changelog' ? `/#${path}` : path,
      ready: 'h1:not(.sw-initial-title):not(.swl-title)',
    })),
    ...['login', 'signup'].map((path) => ({
      id: path,
      path: `/#/${path}`,
      ready: 'form',
    })),
    {
      id: 'recover',
      path: '/#/recover',
      ready: async (page) => {
        await page
          .getByRole('heading', {
            level: 1,
            name: 'Recover Account',
            exact: true,
          })
          .waitFor()
      },
    },
    {
      id: 'sessions',
      path: '/#/sessions',
      ready: async (page) => {
        await page
          .getByRole('heading', { level: 1, name: 'Welcome back', exact: true })
          .waitFor()
      },
    },
  ],
}

export default routesManifest
