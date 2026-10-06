import { resolve } from 'node:path'
import { createServer } from 'vite'

/** viteConfig is the website's Vite configuration, which fixtures share. */
const viteConfig = 'vite.config.ts'

/**
 * serveFixture serves one fixture module through the website's own Vite
 * configuration, so the page gets the production stylesheet, fonts and
 * module resolution. The page is at the returned url; close the server when
 * done.
 */
export async function serveFixture(entry: string) {
  // Resolve a virtual entry to the fixture.
  const source = resolve(entry)
  const server = await createServer({
    configFile: viteConfig,
    server: { host: '127.0.0.1', port: 0, open: false },
    logLevel: 'error',
    plugins: [
      {
        name: 'ui-fixture',
        resolveId(id) {
          if (id.endsWith('/__fixture.tsx')) return source
        },
        configureServer(dev) {
          // Mount the fixture on a bare page under the configuration's base.
          const page = `${dev.config.base}fixture`
          dev.middlewares.use(page, (_req, res, next) => {
            res.setHeader('Content-Type', 'text/html')
            dev
              .transformIndexHtml(
                page,
                `<html><body><div id="root"></div><script type="module" src="${dev.config.base}__fixture.tsx"></script></body></html>`,
              )
              .then((html) => res.end(html), next)
          })
        },
      },
    ],
  })
  await server.listen()
  return { server, url: `${server.resolvedUrls!.local[0]}fixture` }
}
