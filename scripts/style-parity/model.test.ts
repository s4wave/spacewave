import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { capture, type ParityManifest } from './capture.js'
import { compareSnapshot, type Snapshot } from './model.js'

test('parity detects one changed property in both engines at every size', async () => {
  // Serve a deterministic page with an animation and an accessible control.
  let color = 'rgb(255, 0, 0)'
  const server = createServer((request, response) => {
    response.statusCode = request.url === '/missing' ? 404 : 200
    response.setHeader('Content-Type', 'text/html')
    response.end(`<!doctype html><html lang="en"><head><style>
      body { background: white; margin: 16px; }
      button { outline: 2px solid ${color}; }
      @keyframes pulse { from { opacity: 1; } to { opacity: 0.5; } }
      span { animation: pulse 1s linear infinite; }
    </style></head><body><h1>Parity</h1><button>Open</button><span>Ready</span></body></html>`)
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert(address && typeof address !== 'string')
  const base = `http://127.0.0.1:${address.port}`

  // Keep every capture in a disposable directory owned by this test.
  const parent = process.env.STYLE_PARITY_TEST_ARTIFACTS ?? tmpdir()
  await mkdir(parent, { recursive: true })
  const root = await mkdtemp(join(parent, 'style-parity-'))
  console.log(`self-check artifacts: ${root}`)
  const manifest: ParityManifest = {
    states: [{ id: 'fixture', path: '/', ready: 'button' }],
  }

  // Compare separate captures, then change only the button's outline color.
  try {
    for (const name of ['before', 'same', 'changed'])
      await mkdir(join(root, name))
    const paths = await capture(manifest, base, join(root, 'before'))
    await capture(manifest, base, join(root, 'same'))
    color = 'rgb(0, 0, 255)'
    await capture(manifest, base, join(root, 'changed'))
    for (const path of paths) {
      const read = async (name: string): Promise<Snapshot> =>
        JSON.parse(await readFile(join(root, name, path), 'utf8'))
      const before = await read('before')
      assert.deepEqual([...compareSnapshot(before, await read('same'))], [])
      const differences = [...compareSnapshot(before, await read('changed'))]
      assert.deepEqual(differences.map(({ property }) => property).sort(), [
        'screenshot',
        'styles.outline-color',
      ])
      const changed = differences.find(
        ({ property }) => property === 'styles.outline-color',
      )!
      assert.equal(
        changed.element,
        'html>body:nth-of-type(1)>button:nth-of-type(1)',
      )
      assert.equal(changed.before, 'rgb(255, 0, 0)')
      assert.equal(changed.after, 'rgb(0, 0, 255)')
    }
    assert.equal(paths.length, 6)

    // A failed state preserves diagnostics while later states still capture.
    const failureDir = join(root, 'failure')
    await mkdir(failureDir)
    await assert.rejects(
      capture(
        {
          states: [
            {
              id: 'failed',
              path: '/missing',
              ready: 'button',
            },
            ...manifest.states,
          ],
        },
        base,
        failureDir,
      ),
      /6 captures failed/,
    )
    for (const path of paths) {
      const failure = JSON.parse(
        await readFile(
          join(failureDir, path.replace('fixture.json', 'failed.failure.json')),
          'utf8',
        ),
      )
      assert.match(failure.error, /HTTP 404/)
      assert.equal(failure.probes[0], 'saved')
      assert.equal(failure.probes[1], 'saved')
      assert.equal(
        JSON.parse(await readFile(join(failureDir, path), 'utf8')).id,
        'fixture',
      )
    }

    // Keep structural and accessibility regressions visible in the same comparison.
    const before = await readFile(join(root, 'before', paths[0]), 'utf8')
    const removed: Snapshot = JSON.parse(before)
    removed.elements.pop()
    assert.ok(
      [...compareSnapshot(JSON.parse(before), removed)].some(
        ({ property }) => property === 'element',
      ),
    )
    const renamed: Snapshot = JSON.parse(before)
    renamed.aria = 'changed accessible tree'
    assert.deepEqual(
      [...compareSnapshot(JSON.parse(before), renamed)].map(
        ({ property }) => property,
      ),
      ['aria'],
    )
  } finally {
    server.closeAllConnections()
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    )
    if (!process.env.STYLE_PARITY_TEST_ARTIFACTS)
      await rm(root, { recursive: true, force: true })
  }
})
