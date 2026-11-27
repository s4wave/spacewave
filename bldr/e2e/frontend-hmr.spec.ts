import assert from 'node:assert/strict'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { dirname, join, relative, resolve } from 'node:path'
import { createInterface } from 'node:readline'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page } from '@playwright/test'

// Keep ordinary E2E collection safe until the isolated CLI is explicitly supplied.
test.skip(
  !process.env.BLDR_FRONTEND_TEST_BINARY,
  'Opt-in: build the Bldr CLI, set BLDR_FRONTEND_TEST_BINARY, and run bun run playwright test --config bldr/e2e/frontend-hmr.config.ts',
)

/** Canonical contains the singleton imports provided by the document. */
type Canonical = {
  React: typeof import('react')
  jsx: typeof import('react/jsx-runtime').jsx
  WebDocument: typeof import('@aptre/bldr').WebDocument
  BldrContext: typeof import('@aptre/bldr-react').BldrContext
}

/** Mounted adds the runtime document and view received through Bldr context. */
type Mounted = Canonical & {
  webDocument: import('@aptre/bldr').WebDocument
  webView: import('@aptre/bldr').WebView
  sharedIdentity?: boolean
}

declare global {
  var __hmrCanonical: Canonical
  var __hmrMounted: Mounted
  var __hmrShared: Mounted
  var __hmrInitial: Mounted | undefined
  var __hmrObservation: Promise<void>
  var __frontendDocument: number | undefined
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

/** identities records imports and runtime objects used by the mounted fixture. */
const identities = `import React from 'react'
import { jsx } from 'react/jsx-runtime'
import { WebDocument } from '@aptre/bldr'
import { BldrContext, useBldrContext } from '@aptre/bldr-react'
`
const appSource = `${identities}
import Shared from '@s4wave/web/hmr-fixture'
import SameShared from './Shared.tsx'
import './app.css'
export default function App() {
  // Retain the application's counter through compatible updates.
  const [count, setCount] = React.useState(0)

  // Retain the draft independently of the counter.
  const [text, setText] = React.useState('')

  // Read the mounted document and view from their owning context.
  const context = useBldrContext()

  // Record the imports and runtime objects for identity comparisons.
  globalThis.__hmrMounted = { React, jsx, WebDocument, BldrContext, ...context, sharedIdentity: Shared === SameShared }

  // Render editable content and the shared component in the mounted view.
  return <main>
    <h1>HMR before</h1>
    <button id="hmr-count" onClick={() => setCount(count + 1)}>Count {count}</button>
    <input aria-label="Draft" value={text} onChange={event => setText(event.target.value)} />
    <Shared />
  </main>
}
`
const sharedSource = `${identities}
export default function Shared() {
  const [count, setCount] = React.useState(0)
  const context = useBldrContext()
  globalThis.__hmrShared = { React, jsx, WebDocument, BldrContext, ...context }
  return <section><p>Shared before</p><button id="hmr-shared-count" onClick={() => setCount(count + 1)}>Shared count {count}</button></section>
}
`
const cssSource = '#hmr-count { color: rgb(255, 0, 0); }\n'
const startupSource = `${identities}
import { WebView } from '@aptre/bldr-react'
export default function Startup() {
  globalThis.__hmrCanonical = { React, jsx, WebDocument, BldrContext }
  return <WebView />
}
`

/** FrontendFixture owns one ordinary Bldr CLI and disposable project source. */
class FrontendFixture {
  readonly lines: string[] = []
  private native?: ChildProcess
  private closed?: Promise<void>
  origin = ''

  constructor(readonly directory: string) {}

  /** start waits for the CLI's listener announcement and collects its output. */
  async start(): Promise<void> {
    // Create the same manifest-loaded application used by the existing smoke.
    const source = relative(root, this.directory).replaceAll('\\', '/')
    const config = `import config from '../../vite.config.ts'
import { resolve } from 'node:path'
export default {
  ...config,
  resolve: { ...config.resolve, alias: [
    { find: '@s4wave/web/hmr-fixture', replacement: resolve(${JSON.stringify(this.directory)}, 'Shared.tsx') },
    ...config.resolve.alias,
  ] },
  server: { watch: { ignored: ['**/state/**', '**/.bldr*/**', '**/vendor/**', '**/*.log'] } },
}
`
    await Promise.all([
      writeFile(join(this.directory, 'App.tsx'), appSource),
      writeFile(join(this.directory, 'Shared.tsx'), sharedSource),
      writeFile(join(this.directory, 'Startup.tsx'), startupSource),
      writeFile(join(this.directory, 'app.css'), cssSource),
      writeFile(join(this.directory, 'vite.config.ts'), config),
      writeFile(
        join(this.directory, 'bldr.star'),
        `
project(id="frontend-hmr-check", start=start_config(plugins=["web", "hmr-app"], loadWebStartup=${JSON.stringify(source + '/Startup.tsx')}))
manifest("web", builder="bldr/web/plugin/compiler", config=web_plugin_compiler_config())
manifest("hmr-app", builder="bldr/plugin/compiler/js", config=js_plugin_config(webPluginId="web", viteConfigPaths=[${JSON.stringify(source + '/vite.config.ts')}], viteDisableProjectConfig=True, modules=[js_module("JS_MODULE_KIND_FRONTEND", ${JSON.stringify(source + '/App.tsx')}, entrypoint=True, webViewParentId={"empty": True})]))
build("web", manifests=["web", "hmr-app"], targets=["browser"])
`,
      ),
    ])

    // Reserve an isolated listener address before spawning the task's CLI.
    const reservation = createServer()
    await new Promise<void>((ready) =>
      reservation.listen(0, '127.0.0.1', ready),
    )
    const address = reservation.address()
    assert(address && typeof address !== 'string')
    this.origin = `http://127.0.0.1:${address.port}`
    await new Promise<void>((closed) => reservation.close(() => closed()))
    const binary = process.env.BLDR_FRONTEND_TEST_BINARY
    assert(
      binary,
      'Build the CLI and set BLDR_FRONTEND_TEST_BINARY before running this test',
    )

    // Observe native output, including real bundle builds and publications.
    const native = (this.native = spawn(
      resolve(binary),
      [
        `--bldr-src-path=${root}`,
        `--state-path=${join(this.directory, 'state')}`,
        `--config=${join(this.directory, 'bldr.yaml')}`,
        '--log-level=debug',
        '--no-tui',
        '--minify-entrypoint=false',
        'start',
        'web',
        `--listen=127.0.0.1:${address.port}`,
      ],
      { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] },
    ))
    this.closed = new Promise((done) => native.once('close', () => done()))
    const output = createInterface({ input: native.stdout! })
    const errors = createInterface({ input: native.stderr! })
    const ready = new Promise<void>((done, reject) => {
      // Fail readiness if the fixture process cannot start or exits early.
      native.once('error', reject)
      native.once('exit', (code, signal) =>
        reject(new Error(`Bldr exited before readiness: ${code}/${signal}`)),
      )
      const receive = (line: string) => {
        this.lines.push(line)
        if (line.includes('listening on:')) done()
      }
      output.on('line', receive)
      errors.on('line', receive)
    })
    await ready
  }

  /** operations reads actual builder invocations and manifest commits. */
  operations() {
    return {
      viteBundles: this.lines.filter((line) =>
        /building vite bundle\b/.test(line),
      ).length,
      esbuildBundles: this.lines.filter((line) =>
        /building esbuild bundle\b/.test(line),
      ).length,
      webPkgBundles: this.lines.filter((line) =>
        line.includes('building web pkg bundle with vite'),
      ).length,
      rolldownBundles: this.lines.filter(
        (line) =>
          line.includes('running command:') && line.includes('/run-build.mjs'),
      ).length,
      publications: this.lines.filter((line) =>
        line.includes('committing manifest to world'),
      ).length,
    }
  }

  /** close joins the fixture CLI and persists diagnostics before deleting data. */
  async close(reportPath: string): Promise<void> {
    // End only the process started by this fixture.
    // Its pipe owns its compilers.
    if (this.native?.exitCode === null && this.native.signalCode === null) {
      this.native.kill('SIGINT')
    }

    // Join closed stdio even when the process already exited or failed to spawn.
    await this.closed

    // Keep native evidence outside the disposable project and remove its state.
    await writeFile(reportPath, this.lines.join('\n') + '\n')
    await rm(this.directory, { recursive: true, force: true })
  }
}

/** observeEdit measures a source write through a DOM mutation, without polling. */
async function observeEdit(
  page: Page,
  path: string,
  source: string,
  expected: { selector: string; text?: string; color?: string },
): Promise<number> {
  // Subscribe before the write, so even a fast compiler update is observed.
  await page.evaluate((expected) => {
    const matches = () => {
      const element = document.querySelector(expected.selector)
      return (
        !!element &&
        (expected.text === undefined ||
          element.textContent === expected.text) &&
        (expected.color === undefined ||
          getComputedStyle(element).color === expected.color)
      )
    }
    globalThis.__hmrObservation = new Promise<void>((done) => {
      const observer = new MutationObserver(() => {
        if (!matches()) return
        observer.disconnect()
        done()
      })
      observer.observe(document, {
        subtree: true,
        childList: true,
        characterData: true,
        attributes: true,
      })
    })
  }, expected)

  // Report edit-to-observed-update. Startup and compositor paint are separate.
  const started = performance.now()
  await writeFile(path, source)
  await page.evaluate(() => globalThis.__hmrObservation)
  return Math.round(performance.now() - started)
}

/** snapshot verifies import identities and reads Bldr's real backend generations. */
async function snapshot(page: Page) {
  return page.evaluate(async () => {
    // Compare editable consumers with the canonical imports used by startup.
    const mounted = globalThis.__hmrMounted
    const shared = globalThis.__hmrShared
    const canonical = globalThis.__hmrCanonical
    const keys = ['React', 'jsx', 'WebDocument', 'BldrContext'] as const
    const identities =
      keys.every(
        (key) =>
          mounted[key] === canonical[key] && shared[key] === canonical[key],
      ) &&
      mounted.sharedIdentity &&
      shared.webDocument === mounted.webDocument &&
      shared.webView === mounted.webView &&
      mounted.webDocument instanceof canonical.WebDocument

    // Save runtime objects once, then compare after each compatible update.
    const initial = (globalThis.__hmrInitial ??= mounted)
    const status = await mounted.webDocument.buildWebDocumentStatusSnapshot()
    return {
      identities,
      canonicalRetained: keys.every((key) => initial[key] === mounted[key]),
      documentRetained: initial.webDocument === mounted.webDocument,
      viewRetained: initial.webView === mounted.webView,
      backends: status
        .webWorkers!.map((worker) => ({
          id: worker.id,
          generation: worker.generation,
          ready: worker.ready,
        }))
        .sort((a, b) => (a.id ?? '').localeCompare(b.id ?? '')),
      documentID: globalThis.__frontendDocument,
    }
  })
}

// TIER: opt-in. Run with frontend-hmr.config.ts and a freshly built CLI.
test('manifest frontend preserves state and identities across compatible edits', async ({
  page,
}, testInfo) => {
  // Allocate isolated project data and collect real runtime transport evidence.
  await mkdir(join(root, '.tmp'), { recursive: true })
  const fixture = new FrontendFixture(
    await mkdtemp(join(root, '.tmp/frontend-hmr-')),
  )

  // Capture browser errors, backend creation, and frontend transport traffic.
  const pageErrors: string[] = []
  const modules: string[] = []
  const responses: {
    url: string
    fromServiceWorker: boolean
    status: number
  }[] = []
  const sockets: string[] = []
  let workersCreated = 0
  page.on('pageerror', (error) => pageErrors.push(error.message))
  page.on('worker', () => workersCreated++)

  // Record browser transport endpoints without accessing private compiler URLs.
  page.on('websocket', (socket) => sockets.push(socket.url()))
  page.on('request', (request) => {
    if (request.url().includes('/b/fe/')) modules.push(request.url())
  })
  page.on('response', (response) => {
    if (response.url().includes('/b/fe/')) {
      responses.push({
        url: response.url(),
        fromServiceWorker: response.fromServiceWorker(),
        status: response.status(),
      })
    }
  })

  // Keep the document marker independent of application module replacement.
  await page.addInitScript(() => {
    globalThis.__frontendDocument = Math.random()
  })

  // Measure startup independently of the warm source edits.
  const startup = performance.now()
  const edits: {
    file: string
    before: string
    after: string
    editToObservedUpdateMs: number
    operations: ReturnType<FrontendFixture['operations']>
  }[] = []
  let initialOperations: ReturnType<FrontendFixture['operations']>
  try {
    await fixture.start()
    await page.goto(fixture.origin, { waitUntil: 'domcontentloaded' })
    await page.locator('#hmr-count').waitFor({ timeout: 60_000 })
    const startupMs = Math.round(performance.now() - startup)
    await page.locator('#hmr-count').click({ clickCount: 2 })
    await page.locator('#hmr-shared-count').click({ clickCount: 3 })
    await page.getByRole('textbox', { name: 'Draft' }).fill('Retained draft')
    const initial = await snapshot(page)
    const initialWorkers = workersCreated
    initialOperations = fixture.operations()
    expect(initial.identities).toBe(true)
    expect(initial.backends.length).toBeGreaterThan(0)
    expect(initial.backends.every((backend) => backend.ready)).toBe(true)
    expect(initialOperations.publications).toBeGreaterThan(0)
    expect(initialOperations.webPkgBundles).toBeGreaterThan(0)
    expect(initialOperations.rolldownBundles).toBeGreaterThan(0)
    expect(
      await page
        .locator('#hmr-count')
        .evaluate((element) => getComputedStyle(element).color),
    ).toBe('rgb(255, 0, 0)')

    // Edit CSS, the mounted application, and its shared UI through the same graph.
    const cases = [
      {
        file: 'app.css',
        before: cssSource,
        after: '#hmr-count { color: rgb(0, 0, 255); }\n',
        expected: { selector: '#hmr-count', color: 'rgb(0, 0, 255)' },
      },
      {
        file: 'App.tsx',
        before: appSource,
        after: appSource.replace('HMR before', 'HMR after'),
        expected: { selector: 'h1', text: 'HMR after' },
      },
      {
        file: 'Shared.tsx',
        before: sharedSource,
        after: sharedSource.replace('Shared before', 'Shared after'),
        expected: { selector: 'section p', text: 'Shared after' },
      },
    ]
    for (const edit of cases) {
      // Observe the accepted render and compare the real operation counters.
      const elapsed = await observeEdit(
        page,
        join(fixture.directory, edit.file),
        edit.after,
        edit.expected,
      )
      const operations = fixture.operations()
      edits.push({
        file: edit.file,
        before: edit.before,
        after: edit.after,
        editToObservedUpdateMs: elapsed,
        operations: {
          viteBundles: operations.viteBundles - initialOperations.viteBundles,
          esbuildBundles:
            operations.esbuildBundles - initialOperations.esbuildBundles,
          webPkgBundles:
            operations.webPkgBundles - initialOperations.webPkgBundles,
          rolldownBundles:
            operations.rolldownBundles - initialOperations.rolldownBundles,
          publications:
            operations.publications - initialOperations.publications,
        },
      })
      expect(operations).toEqual(initialOperations)
      expect(await snapshot(page)).toEqual(initial)
      expect(workersCreated).toBe(initialWorkers)
      expect(await page.locator('#hmr-count').textContent()).toBe('Count 2')
      expect(await page.locator('#hmr-shared-count').textContent()).toBe(
        'Shared count 3',
      )
      expect(
        await page.getByRole('textbox', { name: 'Draft' }).inputValue(),
      ).toBe('Retained draft')
    }

    // Prove that browser modules use Bldr Fetch and the devtool socket.
    expect(
      await page.evaluate(() => !!navigator.serviceWorker.controller),
    ).toBe(true)
    expect(modules.some((url) => /App\.tsx\?t=/.test(url))).toBe(true)
    expect(modules.some((url) => /Shared\.tsx\?t=/.test(url))).toBe(true)
    expect(modules.every((url) => new URL(url).origin === fixture.origin)).toBe(
      true,
    )
    expect(responses.length).toBeGreaterThan(0)
    expect(
      responses.every(
        (response) => response.fromServiceWorker && response.status === 200,
      ),
    ).toBe(true)
    expect(sockets.length).toBeGreaterThan(0)
    expect(
      sockets.every(
        (url) => new URL(url).host === new URL(fixture.origin).host,
      ),
    ).toBe(true)
    expect(pageErrors).toEqual([])

    // Retain source identities, startup, updates, and calibrated operation counts.
    const evidence = {
      browser: testInfo.project.name,
      startupMs,
      initial,
      initialWorkers,
      initialOperations,
      edits,
      modules,
      responses,
      sockets,
    }
    console.log(testInfo.project.name, {
      startupMs,
      edits: edits.map((edit) => ({
        file: edit.file,
        editToObservedUpdateMs: edit.editToObservedUpdateMs,
        operations: edit.operations,
      })),
    })
    await writeFile(
      testInfo.outputPath('observations.json'),
      JSON.stringify(evidence, null, 2) + '\n',
    )
  } finally {
    await page.close()
    await fixture.close(testInfo.outputPath('native.log'))
  }

  // Include drained output in the assertion against late builds or publications.
  expect(fixture.operations()).toEqual(initialOperations)
})
