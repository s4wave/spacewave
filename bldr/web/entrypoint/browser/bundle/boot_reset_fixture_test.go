//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableBootResetExecutableFixture(t *testing.T) {
	runStableBootFixture(t, stableBootResetFixtureScript)
}

func TestStableBootGateRunsOnceBeforeEntrypointImport(t *testing.T) {
	runStableBootFixture(t, stableBootImportOrderFixtureScript)
}

func TestStableBootRecoversSignedOutSession(t *testing.T) {
	runStableBootFixture(t, stableBootSignedOutRecoveryFixtureScript)
}

func TestStableBootEntrypointStreamProgress(t *testing.T) {
	runStableBootFixture(t, stableBootEntrypointStreamProgressFixtureScript)
}

func TestStableBootDownloadRegistry(t *testing.T) {
	runStableBootFixture(t, stableBootDownloadRegistryFixtureScript)
}

func runStableBootFixture(t *testing.T, fixtureScript string) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not available for executable boot reset fixture")
	}

	dir := t.TempDir()
	if err := WriteStableBootAsset(dir); err != nil {
		t.Fatal(err)
	}

	harnessPath := filepath.Join(dir, "boot-reset-fixture.mjs")
	if err := os.WriteFile(harnessPath, []byte(fixtureScript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bun", "run", harnessPath, filepath.Join(dir, stableBootFilename), dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("boot reset fixture failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "=passed") {
		t.Fatalf("boot reset fixture did not report pass:\n%s", out)
	}
}

const stableBootResetFixtureScript = `
const bootPath = process.argv[2]
if (!bootPath) throw new Error('missing boot asset path')

const script = await Bun.file(bootPath).text()

class StorageFixture {
  constructor(entries) {
    this.map = new Map(Object.entries(entries))
  }
  get length() {
    return this.map.size
  }
  getItem(key) {
    return this.map.has(key) ? this.map.get(key) : null
  }
  setItem(key, value) {
    this.map.set(key, String(value))
  }
  removeItem(key) {
    this.map.delete(key)
  }
  key(index) {
    return Array.from(this.map.keys())[index] ?? null
  }
  has(key) {
    return this.map.has(key)
  }
}

const localStorage = new StorageFixture({
  'spacewave-has-session': '1',
  'spacewave-has-interacted': '1',
  'spacewave-state-devtools': '{"open":true}',
  'app-persistent': '{"json":{"draft":"keep"}}',
  'tab-state-home': '{"selected":"old"}',
})
const sessionStorage = new StorageFixture({
  'shell-tabs-state': '{"tabs":[]}',
  'shell-tabs-layout': '{}',
  'spacewave-sso-start-provider': 'provider',
  'spacewave-sso-return-to': '#/return',
  'spacewave-pending-join': 'invite',
  'spacewave-auth-handoff-payload': 'payload',
})

let fetchCalled = false
let indexedDBDeleteCalled = false
let opfsDirectoryRequested = false
const unregistered = []
const deletedCaches = []
let reloadCalled = false
let reloadResolve
const reloadPromise = new Promise((resolve) => {
  reloadResolve = resolve
})

globalThis.localStorage = localStorage
globalThis.sessionStorage = sessionStorage
globalThis.CustomEvent = class CustomEvent {
  constructor(type, init) {
    this.type = type
    this.detail = init?.detail
  }
}
globalThis.performance = { mark() {} }
globalThis.window = {
  location: {
    href: 'https://spacewave.test/',
    reload() {
      reloadCalled = true
      reloadResolve()
    },
  },
  dispatchEvent() {},
  addEventListener() {},
  removeEventListener() {},
}
globalThis.document = {
  documentElement: { setAttribute() {} },
  querySelector() { return null },
  getElementById() { return null },
  addEventListener() {},
  removeEventListener() {},
}
globalThis.navigator = {
  serviceWorker: {
    async getRegistrations() {
      return [
        { async unregister() { unregistered.push('sw-a') } },
        { async unregister() { unregistered.push('sw-b') } },
      ]
    },
  },
  storage: {
    async getDirectory() {
      opfsDirectoryRequested = true
      return {}
    },
  },
}
globalThis.indexedDB = {
  deleteDatabase() {
    indexedDBDeleteCalled = true
  },
}
globalThis.caches = {
  async keys() {
    return ['bldr-control', 'bldr-generation-old']
  },
  async delete(name) {
    deletedCaches.push(name)
    return true
  },
}
globalThis.fetch = async () => {
  fetchCalled = true
  throw new Error('release fetch must not run before reset reload')
}

new Function(script)()
await Promise.race([
  reloadPromise,
  new Promise((_, reject) => setTimeout(() => reject(new Error('timeout waiting for reload')), 2000)),
])

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

assert(reloadCalled, 'boot reset did not reload')
assert(!fetchCalled, 'boot fetched release before reset reload')
assert(unregistered.length === 2, 'boot did not unregister ServiceWorkers')
assert(deletedCaches.join(',') === 'bldr-control,bldr-generation-old', 'boot did not delete expected caches')
assert(localStorage.getItem('spacewave-browser-app-state-version') === '1000000', 'boot did not store durable version after cleanup')
assert(sessionStorage.getItem('spacewave-browser-tab-state-version') === '1000000', 'boot did not store tab version')
assert(sessionStorage.getItem('spacewave-browser-app-state-reset-attempted') === '1000000', 'boot did not set reset attempt guard')
assert(globalThis.__swBootRecoveryStatus.compatibilityVersion === '1000000', 'boot recovery status missing compatibility version')
assert(globalThis.__swBootRecoveryStatus.lastResetDecision === 'reset-complete', 'boot recovery status missing reset-complete decision')
assert(globalThis.__swBootStatus.compatibilityVersion === '1000000', 'boot status missing compatibility version')
assert(globalThis.__swBootStatus.lastResetDecision === 'reset-complete', 'boot status missing latest reset decision')
assert(!localStorage.has('spacewave-has-session'), 'boot did not clear shell localStorage key')
assert(!localStorage.has('spacewave-has-interacted'), 'boot did not clear interaction hint')
assert(localStorage.getItem('app-persistent') === '{"json":{"draft":"keep"}}', 'boot must preserve generic app-persistent state')
assert(localStorage.getItem('tab-state-home') === '{"selected":"old"}', 'boot must preserve generic tab-state prefix')
assert(!sessionStorage.has('shell-tabs-state'), 'boot did not clear shell sessionStorage key')
assert(!sessionStorage.has('shell-tabs-layout'), 'boot did not clear shell layout key')
assert(sessionStorage.getItem('spacewave-sso-start-provider') === 'provider', 'boot must preserve SSO start provider')
assert(sessionStorage.getItem('spacewave-sso-return-to') === '#/return', 'boot must preserve SSO return path')
assert(sessionStorage.getItem('spacewave-pending-join') === 'invite', 'boot must preserve pending join payload')
assert(sessionStorage.getItem('spacewave-auth-handoff-payload') === 'payload', 'boot must preserve auth handoff payload')
assert(!indexedDBDeleteCalled, 'boot must not delete IndexedDB')
assert(!opfsDirectoryRequested, 'boot must not request OPFS root')

console.log('boot-reset-fixture=passed')
`

const stableBootSignedOutRecoveryFixtureScript = `
const bootPath = process.argv[2]
if (!bootPath) throw new Error('missing boot asset path')
const script = await Bun.file(bootPath).text()

class StorageFixture {
  constructor(entries) { this.map = new Map(Object.entries(entries)) }
  get length() { return this.map.size }
  getItem(key) { return this.map.has(key) ? this.map.get(key) : null }
  setItem(key, value) { this.map.set(key, String(value)) }
  removeItem(key) { this.map.delete(key) }
  key(index) { return Array.from(this.map.keys())[index] ?? null }
}

async function runScenario(gatedPath) {
  const localStorage = new StorageFixture({
    'spacewave-browser-app-state-version': '1000000',
    'presentation-preference': 'compact',
    'selected-map': 'moon',
    'map-draft-session': '{"moves":[1,2]}',
  })
  const sessionStorage = new StorageFixture({
    'spacewave-browser-tab-state-version': '1000000',
  })
  const events = []
  let redirectedTo
  let redirectResolve
  const redirectPromise = new Promise((resolve) => { redirectResolve = resolve })
  const opfsEntries = new Map([['runtime-cache', {}], ['session-shell', {}]])

  globalThis.localStorage = localStorage
  globalThis.sessionStorage = sessionStorage
  globalThis.CustomEvent = class CustomEvent {
    constructor(type, init) { this.type = type; this.detail = init?.detail }
  }
  globalThis.performance = { mark() {} }
  globalThis.window = {
    location: {
      href: 'https://spacewave.test/',
      origin: 'https://spacewave.test',
      hash: '',
      replace(path) {
        events.push('redirect')
        redirectedTo = path
        redirectResolve()
      },
    },
    history: { state: null, replaceState() {} },
    dispatchEvent() {},
    addEventListener() {},
    removeEventListener() {},
    setTimeout,
    clearTimeout,
  }
  globalThis.document = {
    readyState: 'complete',
    documentElement: { setAttribute() {} },
    querySelector() { return null },
    getElementById() { return null },
    addEventListener() {},
    removeEventListener() {},
  }
  globalThis.navigator = {
    serviceWorker: {
      async getRegistrations() {
        return [{ async unregister() { events.push('unregister') } }]
      },
    },
    storage: {
      async getDirectory() {
        return {
          async *entries() { for (const entry of opfsEntries) yield entry },
          async removeEntry(name, options) {
            if (!options?.recursive) throw new Error('OPFS cleanup must be recursive')
            opfsEntries.delete(name)
            events.push('opfs:' + name)
          },
        }
      },
    },
  }
  globalThis.caches = {
    async keys() { return ['shell-cache'] },
    async delete(name) { events.push('cache:' + name); return true },
  }
  globalThis.fetch = async (path) => {
    if (gatedPath === 'release' || path === '/entrypoint.mjs') {
      return new Response(JSON.stringify({
        error: { code: 'login_required', message: 'Login is required.' },
      }), { status: 401, headers: { 'content-type': 'application/json' } })
    }
    return new Response(JSON.stringify({
      generationId: 'fixture',
      autoStart: true,
      shellAssets: { entrypoint: '/entrypoint.mjs', serviceWorker: '/sw.mjs' },
    }), { status: 200, headers: { 'content-type': 'application/json' } })
  }

  new Function(script)()
  await Promise.race([
    redirectPromise,
    new Promise((_, reject) => setTimeout(() => reject(new Error('timeout waiting for login redirect')), 2000)),
  ])

  function assert(condition, message) {
    if (!condition) throw new Error(gatedPath + ': ' + message)
  }
  assert(redirectedTo === '/login', 'boot did not use the origin-relative login path')
  assert(events.includes('unregister'), 'boot did not unregister its service worker')
  assert(events.includes('cache:shell-cache'), 'boot did not clear Cache Storage')
  assert(opfsEntries.size === 0, 'boot did not clear shell OPFS state')
  assert(events.at(-1) === 'redirect', 'boot redirected before cleanup settled')
  assert(localStorage.getItem('presentation-preference') === 'compact', 'boot removed presentation preference')
  assert(localStorage.getItem('selected-map') === 'moon', 'boot removed selected map')
  assert(localStorage.getItem('map-draft-session') === '{"moves":[1,2]}', 'boot removed map draft')
}

await runScenario('release')
await runScenario('entrypoint')
console.log('boot-signed-out-recovery-fixture=passed')
`

const stableBootImportOrderFixtureScript = `
const bootPath = process.argv[2]
const fixtureDir = process.argv[3]
if (!bootPath) throw new Error('missing boot asset path')
if (!fixtureDir) throw new Error('missing fixture dir')

const script = await Bun.file(bootPath).text()

class StorageFixture {
  constructor(entries) {
    this.map = new Map(Object.entries(entries))
  }
  get length() {
    return this.map.size
  }
  getItem(key) {
    return this.map.has(key) ? this.map.get(key) : null
  }
  setItem(key, value) {
    this.map.set(key, String(value))
  }
  removeItem(key) {
    this.map.delete(key)
  }
  key(index) {
    return Array.from(this.map.keys())[index] ?? null
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function waitFor(predicate, label) {
  const deadline = Date.now() + 2000
  while (Date.now() < deadline) {
    if (predicate()) return
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  throw new Error('timeout waiting for ' + label + ': ' + (globalThis.__fixtureEvents ?? []).join(','))
}

function installEnvironment({ events, localStorage, sessionStorage, autoStart, entrypointPath, href, hash, readyState }) {
  globalThis.__fixtureEvents = events
  globalThis.localStorage = localStorage
  globalThis.sessionStorage = sessionStorage
  globalThis.CustomEvent = class CustomEvent {
    constructor(type, init) {
      this.type = type
      this.detail = init?.detail
    }
  }
  globalThis.performance = { mark() {} }
  const location = {
    href,
    hash,
    reload() {
      events.push('reload')
    },
    replace(next) {
      events.push('replace:' + next)
      this.href = String(next)
    },
  }
  globalThis.window = {
    location,
    history: {
      state: null,
      replaceState(state, title, next) {
        events.push('history:replaceState:' + next)
        location.href = String(next)
      },
    },
    dispatchEvent(event) {
      if (event?.type === 'spacewave:boot-status') {
        events.push('status:' + event.detail.phase)
      }
    },
    addEventListener() {},
    removeEventListener() {},
  }
  globalThis.document = {
    documentElement: { setAttribute() {} },
    readyState,
    querySelector() { return null },
    getElementById() { return null },
    addEventListener() {},
    removeEventListener() {},
  }
  globalThis.navigator = {
    serviceWorker: {
      async getRegistrations() {
        events.push('cleanup:service-workers')
        return [{ async unregister() { events.push('cleanup:service-worker.unregister') } }]
      },
    },
  }
  globalThis.caches = {
    async keys() {
      events.push('cleanup:caches')
      return ['bldr-generation-old']
    },
    async delete(name) {
      events.push('cleanup:cache.delete:' + name)
      return true
    },
  }
  globalThis.fetch = async (url) => {
    events.push('fetch:' + url)
    if (url === '/browser-release.json') {
      return Response.json({
        schemaVersion: 1,
        generationId: 'fixture-generation',
        autoStart,
        shellAssets: {
          entrypoint: entrypointPath,
          serviceWorker: 'sw-fixture.mjs',
          sharedWorker: 'shw-fixture.mjs',
          wasm: 'runtime-fixture.wasm',
          css: [],
        },
      })
    }
    return new Response('export default null')
  }
}

async function runCase({ name, autoStart, hash, readyState, allowPreload }) {
  const events = []
  const entrypointURL = 'data:text/javascript,'
  const localStorage = new StorageFixture({})
  const sessionStorage = new StorageFixture({})
  installEnvironment({
    events,
    localStorage,
    sessionStorage,
    autoStart,
    entrypointPath: entrypointURL,
    href: 'https://spacewave.test/' + hash,
    hash,
    readyState,
  })

  new Function(script)()
  await waitFor(() => events.includes('reload'), name + ' reset reload')

  const firstFetch = events.findIndex((event) => event.startsWith('fetch:'))
  const firstEntrypoint = events.findIndex((event) => event === 'status:entrypoint')
  assert(firstFetch === -1, name + ' fetched release before reset reload')
  assert(firstEntrypoint === -1, name + ' reached entrypoint before reset reload')
  assert(localStorage.getItem('spacewave-browser-app-state-version') === '1000000', name + ' did not store boot version')
  const resetHref = globalThis.window.location.href
  assert(new URL(resetHref).searchParams.has('brr'), name + ' reset reload did not add brr query: ' + resetHref)


  installEnvironment({
    events,
    localStorage,
    sessionStorage,
    autoStart,
    entrypointPath: entrypointURL,
    href: resetHref,
    hash,
    readyState,
  })
  new Function(script)()
  await waitFor(() => events.includes('status:entrypoint'), name + ' entrypoint phase')

  const cleanupStarts = events.filter((event) => event === 'cleanup:service-workers').length
  const cacheCleanups = events.filter((event) => event === 'cleanup:caches').length
  const reloads = events.filter((event) => event === 'reload').length
  assert(cleanupStarts === 1, name + ' cleanup ran more than once: ' + events.join(','))
  assert(cacheCleanups === 1, name + ' cache cleanup ran more than once: ' + events.join(','))
  assert(reloads === 1, name + ' reset reloaded more than once: ' + events.join(','))
  const cleanHref = globalThis.window.location.href
  assert(!new URL(cleanHref).searchParams.has('brr'), name + ' did not clear brr query after reset: ' + cleanHref)

  const reloadIndex = events.indexOf('reload')
  const releaseFetchIndex = events.indexOf('fetch:/browser-release.json')
  const entrypointIndex = events.indexOf('status:entrypoint')
  const entrypointAssetFetchIndex = events.indexOf('fetch:' + entrypointURL)
  assert(releaseFetchIndex > reloadIndex, name + ' release fetch did not wait for reset reload: ' + events.join(','))
  if (allowPreload) {
    assert(entrypointAssetFetchIndex > releaseFetchIndex, name + ' entrypoint asset preload did not wait for release fetch: ' + events.join(','))
    assert(entrypointIndex > entrypointAssetFetchIndex, name + ' entrypoint phase did not wait for preloaded asset: ' + events.join(','))
  } else {
    assert(entrypointIndex > releaseFetchIndex, name + ' entrypoint phase did not wait for release fetch: ' + events.join(','))
    assert(entrypointAssetFetchIndex > entrypointIndex, name + ' entrypoint asset fetch did not wait for entrypoint phase: ' + events.join(','))
  }
  assert(localStorage.getItem('spacewave-browser-app-state-version') === '1000000', name + ' reached entrypoint before current version')
}

await runCase({ name: 'browser-hash', autoStart: false, hash: '#/u/1' })
await runCase({ name: 'electron-autostart', autoStart: true, hash: '' })
await runCase({ name: 'browser-post-load-root-complete', autoStart: false, hash: '', readyState: 'complete', allowPreload: true })
await runCase({ name: 'browser-post-load-root-interactive', autoStart: false, hash: '', readyState: 'interactive', allowPreload: true })
console.log('boot-import-order-fixture=passed')
`

const stableBootEntrypointStreamProgressFixtureScript = `
import { realpathSync } from 'node:fs'

const bootPath = process.argv[2]
if (!bootPath) throw new Error('missing boot asset path')
const fixtureDirArg = process.argv[3]
if (!fixtureDirArg) throw new Error('missing fixture dir')
const fixtureDir = realpathSync(fixtureDirArg)

const script = await Bun.file(bootPath).text()

class StorageFixture {
  constructor(entries) {
    this.map = new Map(Object.entries(entries))
  }
  get length() {
    return this.map.size
  }
  getItem(key) {
    return this.map.has(key) ? this.map.get(key) : null
  }
  setItem(key, value) {
    this.map.set(key, String(value))
  }
  removeItem(key) {
    this.map.delete(key)
  }
  key(index) {
    return Array.from(this.map.keys())[index] ?? null
  }
}

class ElementFixture {
  constructor() {
    this.style = {}
    this.attributes = new Map()
    this.textContent = ''
    const classes = new Set()
    this.classList = {
      toggle(name, force) {
        if (force) {
          classes.add(name)
        } else {
          classes.delete(name)
        }
      },
      contains(name) {
        return classes.has(name)
      },
    }
  }
  closest() {
    return null
  }
  querySelector() {
    return null
  }
  querySelectorAll() {
    return []
  }
  setAttribute(name, value) {
    this.attributes.set(name, String(value))
  }
  getAttribute(name) {
    return this.attributes.has(name) ? this.attributes.get(name) : null
  }
  removeAttribute(name) {
    this.attributes.delete(name)
  }
  replaceChildren(...children) {
    this.textContent = children.join('')
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function waitFor(predicate, label) {
  const deadline = Date.now() + 2000
  while (Date.now() < deadline) {
    if (predicate()) return
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  throw new Error('timeout waiting for ' + label + ': ' + (globalThis.__fixtureEvents ?? []).join(','))
}

function streamChunks(chunks, streamError) {
  let index = 0
  return new ReadableStream({
    pull(controller) {
      if (index < chunks.length) {
        controller.enqueue(new Uint8Array(chunks[index]))
        index += 1
        return
      }
      if (streamError) {
        controller.error(new Error(streamError))
        return
      }
      controller.close()
    },
  })
}

async function runCase(testCase) {
  const events = []
  const progress = new ElementFixture()
  const progressLabel = new ElementFixture()
  const status = new ElementFixture()
  const localStorage = new StorageFixture({
    'spacewave-browser-app-state-version': '1000000',
  })
  const sessionStorage = new StorageFixture({
    'spacewave-browser-tab-state-version': '1000000',
  })
  const entrypointSource = "globalThis.__fixtureEvents.push('import:direct:" + testCase.name + "'); export default null"
  const entrypointFile = fixtureDir + '/entrypoint-' + testCase.name + '.mjs'
  await Bun.write(entrypointFile, entrypointSource)
  const entrypointPath = new URL(entrypointFile, 'file://').href
  const originalCreateObjectURL = URL.createObjectURL
  const originalRevokeObjectURL = URL.revokeObjectURL

  globalThis.__fixtureEvents = events
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    value() {
      throw new Error('entrypoint import must use the canonical module URL')
    },
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    value() {
      events.push('revoke:unexpected-object-url')
    },
  })
  globalThis.localStorage = localStorage
  globalThis.sessionStorage = sessionStorage
  globalThis.CustomEvent = class CustomEvent {
    constructor(type, init) {
      this.type = type
      this.detail = init?.detail
    }
  }
  globalThis.performance = { mark() {} }
  const location = {
    href: 'https://spacewave.test/',
    hash: '',
    reload() {
      events.push('reload')
    },
    replace(next) {
      events.push('replace:' + next)
      this.href = String(next)
    },
  }
  globalThis.window = {
    location,
    history: {
      state: null,
      replaceState(state, title, next) {
        events.push('history:replaceState:' + next)
        location.href = String(next)
      },
    },
    dispatchEvent(event) {
      if (event?.type === 'spacewave:boot-status') {
        const statusProgress = event.detail.progress
        events.push(
          'status:' +
            event.detail.phase +
            ':' +
            (statusProgress === undefined
              ? 'indeterminate'
              : String(Math.round(statusProgress * 100))),
        )
      }
    },
    addEventListener() {},
    removeEventListener() {},
  }
  const documentElement = new ElementFixture()
  globalThis.document = {
    documentElement,
    querySelector(selector) {
      if (selector === '[data-sw-boot-status]') return status
      if (selector === '[data-sw-boot-progress]') return progress
      if (selector === '[data-sw-boot-progress-label]') return progressLabel
      return null
    },
    getElementById(id) {
      if (id === 'sw-landing') return new ElementFixture()
      if (id === 'sw-loading') return new ElementFixture()
      return null
    },
    addEventListener() {},
    removeEventListener() {},
  }
  globalThis.navigator = {
    serviceWorker: {
      async getRegistrations() {
        events.push('cleanup:service-workers')
        return []
      },
    },
  }
  globalThis.caches = {
    async keys() {
      events.push('cleanup:caches')
      return []
    },
    async delete() {
      return true
    },
  }
  globalThis.fetch = async (url) => {
    events.push('fetch:' + url)
    if (url === '/browser-release.json') {
      const shellAssets = {
        entrypoint: entrypointPath,
        serviceWorker: 'sw-fixture.mjs',
        sharedWorker: 'shw-fixture.mjs',
        wasm: 'runtime-fixture.wasm',
        css: [],
      }
      if (testCase.sizeHint !== undefined) {
        shellAssets.entrypointDecompressedSize = testCase.sizeHint
      }
      return Response.json({
        schemaVersion: 1,
        generationId: 'fixture-generation',
        autoStart: true,
        shellAssets,
      })
    }
    if (url === '/runtime-fixture.wasm') {
      return new Response('wasm')
    }
    if (url === entrypointPath) {
      return new Response(streamChunks(testCase.chunks, testCase.streamError), {
        status: 200,
        headers: testCase.contentLength
          ? { 'content-length': String(testCase.contentLength) }
          : {},
      })
    }
    throw new Error('unexpected fetch ' + url)
  }

  try {
    new Function(script)()
    if (testCase.streamError) {
      await waitFor(
        () => events.some((event) => event.startsWith('status:entrypoint-error:')),
        testCase.name + ' entrypoint error status',
      )
    } else {
      await waitFor(
        () => events.includes('import:direct:' + testCase.name),
        testCase.name + ' canonical entrypoint import completion',
      )
    }

    const manifestIndex = events.indexOf('fetch:/browser-release.json')
    const entrypointFetchIndex = events.indexOf('fetch:' + entrypointPath)
    const importIndex = events.indexOf('import:direct:' + testCase.name)
    assert(manifestIndex !== -1, testCase.name + ' did not fetch release manifest: ' + events.join(','))
    assert(entrypointFetchIndex > manifestIndex, testCase.name + ' did not fetch entrypoint after manifest: ' + events.join(','))
    assert(documentElement.getAttribute('data-sw-boot-visibility') === 'loading', testCase.name + ' did not retain loading-shell visibility through boot')
    if (testCase.streamError) {
      assert(importIndex === -1, testCase.name + ' imported the entrypoint after its stream failed: ' + events.join(','))
    } else {
      assert(importIndex > entrypointFetchIndex, testCase.name + ' did not import the canonical entrypoint URL after progress stream: ' + events.join(','))
    }

    const progressEvents = events.filter((event) => event.startsWith('status:entrypoint:'))
    assert(!events.some((event) => event.startsWith('status:app:')), testCase.name + ' emitted removed app-phase progress: ' + events.join(','))
    if (testCase.streamError) {
      const errorEvents = events.filter((event) => event.startsWith('status:entrypoint-error:'))
      assert(progressEvents.includes(testCase.expectedPartialProgressEvent), testCase.name + ' did not report the partial raw entrypoint fraction before failure: ' + events.join(','))
      assert(errorEvents.length === 1 && errorEvents[0] === testCase.expectedErrorStatusEvent, testCase.name + ' error status did not retain the partial raw entrypoint fraction: ' + events.join(','))
      assert(globalThis.__swBootStatus.phase === 'entrypoint-error', testCase.name + ' did not retain the entrypoint error phase')
      assert(globalThis.__swBootStatus.state === 'error', testCase.name + ' did not retain the entrypoint error state')
      assert(globalThis.__swBootStatus.progress === testCase.expectedRawProgress, testCase.name + ' error status did not retain raw phase-local progress')
      assert(status.textContent === 'Connect: Downloading the application.', testCase.name + ' error status lost the entrypoint download label')
      assert(progress.style.width === testCase.expectedRenderedProgress, testCase.name + ' rendered progress fell from the partial entrypoint high-water')
      assert(progress.getAttribute('aria-valuenow') === testCase.expectedRenderedAriaValue, testCase.name + ' rendered aria-valuenow fell from the partial entrypoint high-water')
      assert(progressLabel.textContent === testCase.expectedRenderedProgress, testCase.name + ' rendered progress label fell from the partial entrypoint high-water')
    } else if (testCase.expectedProgressEvents) {
      for (const expected of testCase.expectedProgressEvents) {
        assert(progressEvents.includes(expected), testCase.name + ' missing progress event ' + expected + ': ' + events.join(','))
      }
      assert(progress.style.width === '26%', testCase.name + ' did not finish rendered progress at the 26% entrypoint ladder mark')
      assert(progress.getAttribute('aria-valuenow') === '26', testCase.name + ' aria-valuenow did not finish at the 26% entrypoint ladder mark')
      assert(progressLabel.textContent === '26%', testCase.name + ' progress label did not finish at the 26% entrypoint ladder mark')
    } else {
      assert(progressEvents.includes('status:entrypoint:indeterminate'), testCase.name + ' did not report unknown-total entrypoint progress: ' + events.join(','))
      assert(!progressEvents.some((event) => event !== 'status:entrypoint:indeterminate'), testCase.name + ' reported a raw fraction without a positive total: ' + events.join(','))
      assert(!progress.classList.contains('animate-progress-indeterminate'), testCase.name + ' rendered overall progress as indeterminate')
      assert(progress.style.width === '8%', testCase.name + ' did not remain at the 8% entrypoint ladder floor')
      assert(progress.getAttribute('aria-valuenow') === '8', testCase.name + ' unknown-total aria-valuenow did not remain at the 8% entrypoint ladder floor')
      assert(progress.getAttribute('aria-valuetext') === null, testCase.name + ' unknown-total progress exposed stale indeterminate aria-valuetext')
      assert(progressLabel.textContent === '8%', testCase.name + ' unknown-total progress label did not remain at the 8% entrypoint ladder floor')
    }
  } finally {
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: originalCreateObjectURL,
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: originalRevokeObjectURL,
    })
  }
}

await runCase({
  name: 'manifest-size',
  sizeHint: 10,
  chunks: [4, 6],
  expectedProgressEvents: ['status:entrypoint:40', 'status:entrypoint:100'],
})
await runCase({
  name: 'content-length',
  contentLength: 8,
  chunks: [2, 6],
  expectedProgressEvents: ['status:entrypoint:25', 'status:entrypoint:100'],
})
await runCase({
  name: 'unknown-total',
  chunks: [3, 5],
})
await runCase({
  name: 'partial-error',
  sizeHint: 10,
  chunks: [4],
  streamError: 'fixture stream failed',
  expectedPartialProgressEvent: 'status:entrypoint:40',
  expectedErrorStatusEvent: 'status:entrypoint-error:40',
  expectedRawProgress: 0.4,
  expectedRenderedProgress: '15%',
  expectedRenderedAriaValue: '15',
})

console.log('boot-entrypoint-stream-progress-fixture=passed')
`

const stableBootDownloadRegistryFixtureScript = `
import { realpathSync } from 'node:fs'

const bootPath = process.argv[2]
if (!bootPath) throw new Error('missing boot asset path')
const fixtureDirArg = process.argv[3]
if (!fixtureDirArg) throw new Error('missing fixture dir')
const fixtureDir = realpathSync(fixtureDirArg)

const script = await Bun.file(bootPath).text()

class StorageFixture {
  constructor(entries) {
    this.map = new Map(Object.entries(entries))
  }
  get length() {
    return this.map.size
  }
  getItem(key) {
    return this.map.has(key) ? this.map.get(key) : null
  }
  setItem(key, value) {
    this.map.set(key, String(value))
  }
  removeItem(key) {
    this.map.delete(key)
  }
  key(index) {
    return Array.from(this.map.keys())[index] ?? null
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function waitFor(predicate, label) {
  const deadline = Date.now() + 2000
  while (Date.now() < deadline) {
    if (predicate()) return
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  throw new Error('timeout waiting for ' + label)
}

function streamChunks(chunks) {
  return new ReadableStream({
    start(controller) {
      for (const size of chunks) {
        controller.enqueue(new Uint8Array(size))
      }
      controller.close()
    },
  })
}

const events = []
const downloadEventCounts = []
const localStorage = new StorageFixture({
  'spacewave-browser-app-state-version': '1000000',
})
const sessionStorage = new StorageFixture({
  'spacewave-browser-tab-state-version': '1000000',
})
globalThis.__fixtureEvents = events
const entrypointSource = "globalThis.__fixtureEvents.push('import:direct:registry'); export default null"
const entrypointFile = fixtureDir + '/entrypoint-registry.mjs'
await Bun.write(entrypointFile, entrypointSource)
const entrypointPath = new URL(entrypointFile, 'file://').href
Object.defineProperty(URL, 'createObjectURL', {
  configurable: true,
  value() {
    throw new Error('entrypoint import must use the canonical module URL')
  },
})
Object.defineProperty(URL, 'revokeObjectURL', {
  configurable: true,
  value() {
    events.push('revoke:unexpected-object-url')
  },
})
globalThis.localStorage = localStorage
globalThis.sessionStorage = sessionStorage
globalThis.CustomEvent = class CustomEvent {
  constructor(type, init) {
    this.type = type
    this.detail = init?.detail
  }
}
globalThis.performance = { mark() {} }
const location = {
  href: 'https://spacewave.test/',
  hash: '',
  reload() {},
  replace() {},
}
globalThis.window = {
  location,
  history: { state: null, replaceState() {} },
  dispatchEvent(event) {
    if (event?.type === 'spacewave-boot-download') {
      downloadEventCounts.push((event.detail ?? []).length)
    }
  },
  addEventListener() {},
  removeEventListener() {},
}
globalThis.document = {
  documentElement: { setAttribute() {} },
  querySelector() {
    return null
  },
  getElementById() {
    return null
  },
  addEventListener() {},
  removeEventListener() {},
}
globalThis.navigator = {
  serviceWorker: {
    async getRegistrations() {
      return []
    },
  },
}
globalThis.caches = {
  async keys() {
    return []
  },
  async delete() {
    return true
  },
}
globalThis.fetch = async (url) => {
  if (url === '/browser-release.json') {
    return Response.json({
      schemaVersion: 1,
      generationId: 'fixture-generation',
      autoStart: true,
      shellAssets: {
        entrypoint: entrypointPath,
        entrypointDecompressedSize: 12,
        serviceWorker: 'sw-fixture.mjs',
        sharedWorker: 'shw-fixture.mjs',
        wasm: '/runtime-fixture.wasm',
        css: [],
      },
    })
  }
  if (url === '/runtime-fixture.wasm') {
    return new Response(streamChunks([3, 5]), {
      status: 200,
      headers: { 'content-length': '8' },
    })
  }
  if (url === entrypointPath) {
    return new Response(streamChunks([5, 7]), {
      status: 200,
      headers: { 'content-length': '12' },
    })
  }
  throw new Error('unexpected fetch ' + url)
}

new Function(script)()

await waitFor(
  () => events.includes('import:direct:registry'),
  'canonical entrypoint import completion',
)

const downloads = globalThis.__swBootDownloads ?? []
const byId = new Map(downloads.map((d) => [d.id, d]))
await waitFor(
  () => {
    const runtime = (globalThis.__swBootDownloads ?? []).find((d) => d.id === 'runtime')
    return runtime && runtime.state === 'complete'
  },
  'runtime download completion',
)

const finalDownloads = globalThis.__swBootDownloads ?? []
const runtime = finalDownloads.find((d) => d.id === 'runtime')
const app = finalDownloads.find((d) => d.id === 'app')

assert(runtime, 'runtime download not registered: ' + JSON.stringify(finalDownloads))
assert(app, 'app download not registered: ' + JSON.stringify(finalDownloads))
assert(runtime.label === 'Runtime', 'runtime label wrong: ' + runtime.label)
assert(app.label === 'Application', 'app label wrong: ' + app.label)
assert(runtime.total === 8, 'runtime total not from content-length: ' + JSON.stringify(runtime))
assert(runtime.state === 'complete', 'runtime not complete: ' + JSON.stringify(runtime))
assert(runtime.loaded === 8, 'runtime loaded did not reach total: ' + JSON.stringify(runtime))
assert(app.total === 12, 'app total not from size hint: ' + JSON.stringify(app))
assert(app.state === 'complete', 'app not complete: ' + JSON.stringify(app))
assert(app.loaded === 12, 'app loaded did not reach total: ' + JSON.stringify(app))
assert(
  downloadEventCounts.some((count) => count >= 2),
  'boot-download event never reported both downloads: ' + downloadEventCounts.join(','),
)

console.log('boot-download-registry-fixture=passed')
`
