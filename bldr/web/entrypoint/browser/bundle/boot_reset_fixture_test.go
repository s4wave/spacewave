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

function installEnvironment({ events, localStorage, sessionStorage, autoStart, entrypointPath, href, hash }) {
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
  globalThis.window = {
    location: {
      href,
      hash,
      reload() {
        events.push('reload')
      },
      replace(next) {
        events.push('replace')
        this.href = next
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
    return new Response('asset')
  }
}

async function runCase({ name, autoStart, hash }) {
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
  })

  new Function(script)()
  await waitFor(() => events.includes('reload'), name + ' reset reload')

  const firstFetch = events.findIndex((event) => event.startsWith('fetch:'))
  const firstEntrypoint = events.findIndex((event) => event === 'status:entrypoint')
  assert(firstFetch === -1, name + ' fetched release before reset reload')
  assert(firstEntrypoint === -1, name + ' reached entrypoint before reset reload')
  assert(localStorage.getItem('spacewave-browser-app-state-version') === '1000000', name + ' did not store boot version')

  installEnvironment({
    events,
    localStorage,
    sessionStorage,
    autoStart,
    entrypointPath: entrypointURL,
    href: 'https://spacewave.test/' + hash,
    hash,
  })
  new Function(script)()
  await waitFor(() => events.includes('status:entrypoint'), name + ' entrypoint phase')

  const cleanupStarts = events.filter((event) => event === 'cleanup:service-workers').length
  const cacheCleanups = events.filter((event) => event === 'cleanup:caches').length
  const reloads = events.filter((event) => event === 'reload').length
  assert(cleanupStarts === 1, name + ' cleanup ran more than once: ' + events.join(','))
  assert(cacheCleanups === 1, name + ' cache cleanup ran more than once: ' + events.join(','))
  assert(reloads === 1, name + ' reset reloaded more than once: ' + events.join(','))

  const reloadIndex = events.indexOf('reload')
  const releaseFetchIndex = events.indexOf('fetch:/browser-release.json')
  const entrypointIndex = events.indexOf('status:entrypoint')
  assert(reloadIndex !== -1, name + ' missing reload event')
  assert(releaseFetchIndex > reloadIndex, name + ' release fetch did not wait for reset reload: ' + events.join(','))
  assert(entrypointIndex > releaseFetchIndex, name + ' entrypoint phase did not wait for release fetch: ' + events.join(','))
  assert(globalThis.__swBootStatus?.phase === 'entrypoint', name + ' did not reach entrypoint phase: ' + events.join(','))
  assert(localStorage.getItem('spacewave-browser-app-state-version') === '1000000', name + ' reached entrypoint before current version')
}

await runCase({ name: 'browser-hash', autoStart: false, hash: '#/u/1' })
await runCase({ name: 'electron-autostart', autoStart: true, hash: '' })

console.log('boot-import-order-fixture=passed')
`
