// @vitest-environment node
import { expect, it } from 'vitest'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { Agent, get } from 'node:http'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

import { DevelopmentEnvironment } from './development.js'
import {
  adaptDevelopmentClient,
  bindDevelopmentImports,
} from './development-client.js'
import { DevelopmentConfig } from './vite.pb.js'
import { SendRequest } from '../../../frontend/frontend.pb.js'

const require = createRequire(import.meta.url)
const repoRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../../../../',
)

it('guards the installed Vite client and rewrites module specifiers only', async () => {
  const clientPath = join(
    dirname(require.resolve('vite/package.json')),
    'dist/client/client.mjs',
  )
  const code = await readFile(clientPath, 'utf8')
  expect(adaptDevelopmentClient(code)).toContain(
    'return frontend.connect(handlers)',
  )
  expect(() =>
    adaptDevelopmentClient(
      code.replace('const transport =', 'const changedTransport ='),
    ),
  ).toThrow('unsupported Vite client')
  expect(
    bindDevelopmentImports(
      'import React from "/b/fe/test/@id/react"; export const literal = "/b/fe/test/@id/react"',
      '/b/fe/test/',
      ['react'],
      '/refresh.mjs',
    ),
  ).toBe(
    'import React from "react"; export const literal = "/b/fe/test/@id/react"',
  )
})

it('serves a real graph, emits CSS and custom updates, and closes its listener', async () => {
  const temporaryRoot = join(repoRoot, '.tmp')
  await mkdir(temporaryRoot, { recursive: true })
  const root = await mkdtemp(join(temporaryRoot, 'frontend-environment-'))
  const appPath = join(root, 'App.tsx')
  const cssPath = join(root, 'app.css')
  await writeFile(
    appPath,
    'import React from "react"; export { useResource } from "@aptre/bldr-sdk/hooks/useResource.js"; import "./app.css"; export default function App() { return <button>before</button> }',
  )
  await writeFile(cssPath, 'button { color: red }')
  await writeFile(
    join(root, 'vite.config.ts'),
    `
import react from ${JSON.stringify(require.resolve('@vitejs/plugin-react'))}
export default { server: { watch: { ignored: [] } }, plugins: [react(), { name: 'echo', configureServer(server) {
  server.middlewares.use((request, response, next) => {
    if (!request.url.endsWith('/large.js')) return next()
    const body = Buffer.alloc(16 * 1024 * 1024, 120)
    response.writeHead(200, { 'Content-Type': 'text/javascript', 'Content-Length': body.length })
    response.end(body)
  })
  server.environments.client.hot.on('bldr:echo', (data, client) => client.send('bldr:reply', data))
} }] }
`,
  )
  const environment = new DevelopmentEnvironment(
    DevelopmentConfig.create({
      rootDir: root,
      distDir: join(repoRoot, 'bldr'),
      cacheDir: join(root, 'cache'),
      entrypoints: ['App.tsx'],
      externalPkgs: ['react', 'react-dom'],
      sessionId: 'test',
    }),
  )
  const abort = new AbortController()
  let privateURL: string | undefined
  try {
    const result = await environment.start()
    privateURL = result.privateUrl
    expect(result.refreshRuntime).toContain('injectIntoGlobalHook')
    const updates = environment.watch(abort.signal)
    expect((await updates.next()).value?.session?.routePrefix).toBe(
      '/b/fe/test/',
    )
    const module = await fetch(privateURL + '/b/fe/test/App.tsx?t=1')
    expect(module.status).toBe(200)
    const source = await module.text()
    expect(source).toContain('from "react"')
    expect(source).toContain('/sdk/hooks/useResource.tsx')
    expect(source).toContain('/bldr-dev/frontend-refresh/test.mjs')
    const css = await fetch(privateURL + '/b/fe/test/app.css')
    expect(await css.text()).toContain('__vite__updateStyle')

    const reply = updates.next()
    environment.send(
      SendRequest.create({
        sessionId: 'test',
        payload: JSON.stringify({
          type: 'custom',
          event: 'bldr:echo',
          data: 7,
        }),
      }),
    )
    expect(JSON.parse((await reply).value!.payload!)).toEqual({
      type: 'custom',
      event: 'bldr:reply',
      data: 7,
    })
    const update = updates.next()
    await writeFile(cssPath, 'button { color: blue }')
    const event = (await update).value!
    expect(JSON.parse(event.payload!)).toMatchObject({
      type: 'update',
      updates: [{ path: '/app.css' }],
    })
    expect(event.sequence).toBe(2n)
    const refreshed = await fetch(privateURL + '/b/fe/test/app.css?t=2')
    expect(await refreshed.text()).toContain('color: blue')

    // Backpressure must not let the default five-second keep-alive deadline
    // truncate a module that the private listener has already queued.
    const agent = new Agent({ keepAlive: true })
    try {
      const response = await new Promise<import('node:http').IncomingMessage>(
        (resolve, reject) => {
          get(privateURL + '/b/fe/test/large.js', { agent }, resolve).on(
            'error',
            reject,
          )
        },
      )
      await delay(6500)
      let received = 0
      for await (const chunk of response) received += chunk.length
      expect(received).toBe(16 * 1024 * 1024)
      expect(response.complete).toBe(true)
    } finally {
      agent.destroy()
    }
  } finally {
    abort.abort()
    await environment.close()
    await rm(root, { recursive: true, force: true })
  }
  await expect(fetch(privateURL + '/b/fe/test/App.tsx')).rejects.toThrow()
}, 30000)
