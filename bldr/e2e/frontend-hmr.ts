import assert from 'node:assert/strict'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { closeSync, openSync, readFileSync } from 'node:fs'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium, webkit } from 'playwright'

declare global {
  var __frontendDocument: number | undefined
}

// This smoke runs the ordinary CLI with isolated project state and source files.
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
await mkdir(join(root, '.tmp'), { recursive: true })
const directory = await mkdtemp(join(root, '.tmp/frontend-hmr-'))
const source = relative(root, directory).replaceAll('\\', '/')
const appPath = join(directory, 'App.tsx')
const cssPath = join(directory, 'app.css')
const sharedPath = join(directory, 'Shared.tsx')
const configPath = join(directory, 'vite.config.ts')
const logPath = join(directory, 'native.log')
const binary = process.env.BLDR_FRONTEND_TEST_BINARY ?? join(directory, 'bldr')
const appSource = `import React, { useState } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import Shared from '@s4wave/web/hmr-fixture'
import './app.css'
export default function App() {
  const [count, setCount] = useState(0)
  const context = useBldrContext()
  return <main><h1>HMR before</h1><button id="hmr-count" onClick={() => setCount(count + 1)}>Count {count}</button><div id="hmr-context">{context?.webView ? 'context ready' : 'context missing'}</div><Shared /></main>
}
`
const cssSource = '#hmr-count { color: rgb(255, 0, 0); }\n'
const sharedSource =
  'export default function Shared() { return <p>Shared before</p> }\n'
const configSource = `import config from '../../vite.config.ts'
import { resolve } from 'node:path'
export default {
  ...config,
  resolve: { ...config.resolve, alias: [
    { find: '@s4wave/web/hmr-fixture', replacement: resolve(${JSON.stringify(directory)}, 'Shared.tsx') },
    ...config.resolve.alias,
  ] },
  server: { watch: { ignored: ['**/state/**', '**/.bldr*/**', '**/vendor/**', '**/*.log'] } },
}
`
await writeFile(configPath, configSource)
await writeFile(
  join(directory, 'bldr.star'),
  `
project(id="frontend-hmr-check", start=start_config(plugins=["web", "hmr-app"], loadWebStartup="bldr/e2e/downstreamapp/testdata/app/web/startup.tsx"))
manifest("web", builder="bldr/web/plugin/compiler", config=web_plugin_compiler_config())
manifest("hmr-app", builder="bldr/plugin/compiler/js", config=js_plugin_config(webPluginId="web", viteConfigPaths=[${JSON.stringify(source + '/vite.config.ts')}], viteDisableProjectConfig=True, modules=[js_module("JS_MODULE_KIND_FRONTEND", ${JSON.stringify(source + '/App.tsx')}, entrypoint=True, webViewParentId={"empty": True})]))
build("web", manifests=["web", "hmr-app"], targets=["browser"])
`,
)

// Reserve a local address before starting the CLI's listener.
const reservation = createServer()
await new Promise<void>((ready) => reservation.listen(0, '127.0.0.1', ready))
const address = reservation.address()
assert(address && typeof address !== 'string')
const origin = `http://127.0.0.1:${address.port}`
await new Promise<void>((closed) => reservation.close(() => closed()))
let native: ChildProcess | undefined
let exited: Promise<void> | undefined

async function startNative(): Promise<void> {
  const output = openSync(logPath, 'a')
  native = spawn(
    binary,
    [
      `--bldr-src-path=${root}`,
      `--state-path=${join(directory, 'state')}`,
      `--config=${join(directory, 'bldr.yaml')}`,
      '--log-level=debug',
      '--no-tui',
      '--minify-entrypoint=false',
      'start',
      'web',
      `--listen=127.0.0.1:${address.port}`,
    ],
    { cwd: root, stdio: ['ignore', output, output] },
  )
  closeSync(output)
  exited = new Promise((done, reject) => {
    native!.once('error', reject)
    native!.once('exit', () => done())
  })
  const deadline = Date.now() + 90000
  while (Date.now() < deadline) {
    if (native.exitCode !== null || native.signalCode !== null) {
      throw new Error('Bldr exited during startup; see ' + logPath)
    }
    try {
      if ((await fetch(origin)).ok) return
    } catch {
      // The CLI has not opened its listener yet.
    }
    await new Promise((done) => setTimeout(done, 100))
  }
  throw new Error('Bldr startup did not become ready; see ' + logPath)
}

async function stopNative(): Promise<void> {
  if (native && native.exitCode === null && native.signalCode === null)
    native.kill('SIGINT')
  await exited
  native = undefined
}

function publications(): number {
  return (
    readFileSync(logPath, 'utf8').split('committing manifest to world').length -
    1
  )
}

try {
  if (!process.env.BLDR_FRONTEND_TEST_BINARY) {
    const built = spawnSync(
      'go',
      ['build', '-mod=readonly', '-o', binary, './bldr/cmd/bldr'],
      { cwd: root, stdio: 'inherit' },
    )
    assert.equal(built.status, 0, 'build the Bldr CLI')
  }
  await writeFile(appPath, appSource)
  await writeFile(cssPath, cssSource)
  await writeFile(sharedPath, sharedSource)
  await startNative()
  for (const [name, engine] of Object.entries({ chromium, webkit })) {
    if (
      process.env.BLDR_FRONTEND_TEST_BROWSER &&
      process.env.BLDR_FRONTEND_TEST_BROWSER !== name
    )
      continue
    const browser = await engine.launch({ headless: true })
    const deadline = setTimeout(() => void browser.close(), 110000)
    const page = await browser.newPage()
    page.setDefaultTimeout(15000)
    let workers = 0
    const pageErrors: string[] = []
    const diagnostics: string[] = []
    page.on('console', (message) => {
      if (/frontend|devtool|dial|WebSocket/i.test(message.text())) {
        diagnostics.push(message.text())
        if (diagnostics.length > 30) diagnostics.shift()
      }
    })
    page.on('worker', () => workers++)
    page.on('pageerror', (error) => pageErrors.push(error.message))
    await page.addInitScript(() => {
      globalThis.__frontendDocument = Math.random()
    })
    try {
      await page.goto(origin, { waitUntil: 'domcontentloaded' })
      await page.locator('#hmr-count').waitFor({ timeout: 60000 })
      await page.locator('#hmr-count').click({ clickCount: 2 })
      const documentID = await page.evaluate(
        () => globalThis.__frontendDocument,
      )
      const initialWorkers = workers
      const initialPublications = publications()
      const changed = Date.now()
      await writeFile(cssPath, '#hmr-count { color: rgb(0, 0, 255); }\n')
      await page.waitForFunction(
        () =>
          getComputedStyle(document.querySelector('#hmr-count')!).color ===
          'rgb(0, 0, 255)',
      )
      const cssMs = Date.now() - changed
      const reactChanged = Date.now()
      await writeFile(appPath, appSource.replace('HMR before', 'HMR after'))
      await page
        .getByRole('heading', { name: 'HMR after', exact: true })
        .waitFor()
      const reactMs = Date.now() - reactChanged
      await writeFile(
        sharedPath,
        sharedSource.replace('Shared before', 'Shared after'),
      )
      await page.getByText('Shared after', { exact: true }).waitFor()
      assert.equal(
        await page.locator('#hmr-context').innerText(),
        'context ready',
      )
      assert.equal(await page.locator('#hmr-count').innerText(), 'Count 2')
      assert.equal(
        await page.evaluate(() => globalThis.__frontendDocument),
        documentID,
      )
      assert.equal(workers, initialWorkers)
      assert.equal(publications(), initialPublications)
      console.log(name, 'warm updates', {
        cssMs,
        reactMs,
        frontendPublications: 0,
        backendReplacements: 0,
      })

      await writeFile(appPath, 'export default function broken( {')
      await page.locator('vite-error-overlay').waitFor()
      await writeFile(appPath, appSource.replace('HMR before', 'HMR recovered'))
      await page
        .getByRole('heading', { name: 'HMR recovered', exact: true })
        .waitFor()
      await page.locator('vite-error-overlay').waitFor({ state: 'detached' })
      for (let i = 1; i <= 3; i++)
        await writeFile(
          appPath,
          appSource.replace('HMR before', `HMR rapid ${i}`),
        )
      await page
        .getByRole('heading', { name: 'HMR rapid 3', exact: true })
        .waitFor()
      assert.equal(await page.locator('#hmr-count').innerText(), 'Count 2')

      // Exceed the snapshot compiler's former 30-second idle release delay.
      await new Promise((done) => setTimeout(done, 31000))
      await writeFile(
        appPath,
        appSource.replace('HMR before', 'HMR after idle'),
      )
      await page
        .getByRole('heading', { name: 'HMR after idle', exact: true })
        .waitFor()
      assert.equal(await page.locator('#hmr-count').innerText(), 'Count 2')
      assert.equal(publications(), initialPublications)

      const routes = await page.evaluate(async (entrypoint) => {
        const current = await fetch(
          '/b/fe/entrypoint/' + entrypoint + '?t=4242',
        )
        const expired = await fetch('/b/fe/expired/' + entrypoint)
        return {
          current: current.status,
          target: current.url,
          expired: expired.status,
        }
      }, source + '/App.tsx')
      assert.equal(routes.current, 200)
      assert.equal(routes.expired, 410)
      assert(routes.target.endsWith('?t=4242'))

      const configNavigation = page.waitForEvent('domcontentloaded')
      await writeFile(configPath, configSource + `\n// ${name} replacement\n`)
      await configNavigation
      await page.locator('#hmr-count').waitFor({ timeout: 60000 })
      assert.notEqual(
        await page.evaluate(() => globalThis.__frontendDocument),
        documentID,
      )
      assert.equal(await page.locator('#hmr-count').innerText(), 'Count 0')
      console.log(name, 'configuration replacement passed')

      // Interrupt the real devtool transport and edit while its owner is absent.
      await stopNative()
      await writeFile(
        appPath,
        appSource.replace('HMR before', 'HMR reconnected'),
      )
      const reconnectNavigation = page.waitForEvent('domcontentloaded', {
        timeout: 30000,
      })
      await startNative()
      await reconnectNavigation
      await page
        .getByRole('heading', { name: 'HMR reconnected', exact: true })
        .waitFor({ timeout: 60000 })
      assert.equal(pageErrors.length, 0, pageErrors.join('\n'))
      console.log(
        name,
        'syntax, rapid saves, idle, config replacement, and devtool restart passed',
      )
    } catch (error) {
      console.error(name, 'browser transport diagnostics', diagnostics)
      throw error
    } finally {
      clearTimeout(deadline)
      await browser.close()
      await writeFile(appPath, appSource)
      await writeFile(cssPath, cssSource)
      await writeFile(sharedPath, sharedSource)
    }
  }
} catch (error) {
  console.error('Frontend smoke failed; native log:', logPath)
  console.error(
    (await readFile(logPath, 'utf8')).split('\n').slice(-30).join('\n'),
  )
  throw error
} finally {
  await stopNative()
  const processes = spawnSync('ps', ['-Ao', 'command'], {
    encoding: 'utf8',
  }).stdout
  assert(
    !processes.split('\n').some((line) => line.includes(directory)),
    'Bldr left an owned compiler process running',
  )
  await rm(directory, { recursive: true, force: true })
}
