import { test } from 'node:test'
import { createHash } from 'node:crypto'
import { PNG } from 'pngjs'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { once } from 'node:events'
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { capture, type ParityManifest } from './capture.js'
import { compareSnapshot, type Snapshot } from './model.js'
import { compareCaptures } from './compare.js'

test('parity detects one changed property in both engines at every size', async () => {
  // Serve a deterministic page with an animation and an accessible control.
  let color = 'rgb(255, 0, 0)'
  let placeholder = 'rgb(0, 128, 0)'
  let selectionColor = 'rgb(255, 255, 0)'
  const server = createServer((request, response) => {
    response.statusCode = request.url === '/missing' ? 404 : 200
    response.setHeader('Content-Type', 'text/html')
    response.end(`<!doctype html><html lang="en"><head><style>
      body { background: white; color: rgb(32, 64, 96); margin: 16px; }
      button { outline: 2px solid ${color}; }
      button::selection { color: ${selectionColor}; }
      input::placeholder { color: ${placeholder}; }
      @keyframes pulse { from { opacity: 0.25; } to { opacity: 0.5; } }
      span { animation: pulse 1s linear infinite; }
      span::selection { background-color: rgb(255, 128, 0); }
      .scope:where([data-label="a,b"]) { & span::selection { text-shadow: 1px 1px red; } }
      button, .scope span::selection { text-decoration-color: rgb(10, 20, 30); }
    </style></head><body><h1>Parity</h1><button>Open</button><span>Ready</span><input aria-label="Search" placeholder="Search"><div class="scope" data-label="a,b"><span>Scoped</span></div></body></html>`)
  })

  // Bind the actual HTTP fixture before opening any browser context.
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
  let passed = false
  try {
    for (const name of [
      'before',
      'same',
      'changed',
      'pseudo',
      'selection',
      'subtle',
    ])
      await mkdir(join(root, name))
    const paths = await capture(manifest, base, join(root, 'before'))
    await capture(manifest, base, join(root, 'same'))
    color = 'rgb(0, 0, 255)'
    await capture(manifest, base, join(root, 'changed'))
    placeholder = 'rgb(255, 0, 255)'
    await capture(manifest, base, join(root, 'pseudo'))
    selectionColor = 'rgb(255, 0, 255)'
    await capture(manifest, base, join(root, 'selection'))
    color = 'rgb(0, 0, 254)'
    await capture(manifest, base, join(root, 'subtle'))
    for (const path of paths) {
      const read = async (name: string): Promise<Snapshot> =>
        JSON.parse(await readFile(join(root, name, path), 'utf8'))
      const before = await read('before')
      assert.equal(
        before.elements.find((element) =>
          element.path.endsWith('span:nth-of-type(1)'),
        )?.styles.opacity,
        '1',
      )
      assert.deepEqual([...compareSnapshot(before, await read('same'))], [])
      // Nested highlight selectors stay within their parent, and regular selector-list entries stay ordinary.
      const button = before.elements.find(
        (element) => element.role === 'button',
      )!
      assert.deepEqual(Object.keys(button.pseudos['::selection']!), ['color'])
      assert.equal(
        before.elements.find((element) =>
          element.path.endsWith('h1:nth-of-type(1)'),
        )?.pseudos['::selection'],
        undefined,
      )
      const scoped = before.elements.find((element) =>
        element.path.endsWith('div:nth-of-type(1)>span:nth-of-type(1)'),
      )!
      assert(scoped.pseudos['::selection']?.['text-shadow'])
      assert(scoped.pseudos['::selection']?.['text-decoration-color'])
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

    // A real CSS color change of one channel level retains both its property and image difference.
    for (const path of paths) {
      const differences = await Array.fromAsync(
        compareCaptures(
          join(root, 'selection', path),
          join(root, 'subtle', path),
        ),
      )
      assert.deepEqual(differences.map(({ property }) => property).sort(), [
        'screenshot',
        'styles.outline-color',
      ])
    }

    // A real late DOM update between the screenshot and tree sample must be recaptured.
    const coherentDir = join(root, 'coherent')
    await mkdir(coherentDir)
    await capture(
      {
        states: [
          {
            id: 'fixture',
            path: '/',
            ready: 'button',
            setup: async (page) => {
              const screenshot = page.screenshot.bind(page)
              let changes = 0
              page.screenshot = async (options) => {
                const png = await screenshot(options)
                if (changes === 0) {
                  changes++
                  await page.evaluate(() => {
                    document.querySelector('h1')!.textContent = 'Loaded'
                    document.body.style.backgroundColor = 'rgb(0, 0, 255)'
                  })
                } else if (changes === 1) {
                  changes++
                  await page.evaluate(() => {
                    document.body.style.backgroundColor = 'rgb(0, 255, 0)'
                  })
                }
                return png
              }
            },
            observe: async (page, snapshot) => {
              assert(snapshot.aria.includes('Loaded'))
              assert.equal(await page.locator('h1').textContent(), 'Loaded')
            },
          },
        ],
      },
      base,
      coherentDir,
    )
    for (const path of paths) {
      const snapshot: Snapshot = JSON.parse(
        await readFile(join(coherentDir, path), 'utf8'),
      )
      assert(snapshot.aria.includes('Loaded'))
      const png = PNG.sync.read(
        await readFile(join(coherentDir, path.replace(/\.json$/, '.png'))),
      )
      assert.deepEqual([...png.data.subarray(0, 4)], [0, 255, 0, 255])
    }

    // A one-level raster-only variation remains recorded separately; a two-level image defect still fails.
    const originalFile = join(root, 'before', paths[0]!)
    const rasterFile = join(root, 'raster.json')
    const raster: Snapshot = JSON.parse(await readFile(originalFile, 'utf8'))
    const png = PNG.sync.read(
      await readFile(originalFile.replace(/\.json$/, '.png')),
    )
    const noise: Array<{ pixels: number; maxChannelDelta: number }> = []
    for (const delta of [1, 2]) {
      png.data[0] = 255 - delta
      raster.screenshot = createHash('sha256')
        .update(`${png.width}x${png.height}:`)
        .update(png.data)
        .digest('hex')
      await writeFile(rasterFile, JSON.stringify(raster))
      await writeFile(
        rasterFile.replace(/\.json$/, '.png'),
        PNG.sync.write(png),
      )
      const differences = await Array.fromAsync(
        compareCaptures(originalFile, rasterFile, (variance) => {
          noise.push(variance)
        }),
      )
      assert.deepEqual(
        differences.map(({ property }) => property),
        delta === 1 ? [] : ['screenshot'],
      )
    }
    assert.equal(noise.length, 1)
    assert.equal(noise[0]!.pixels, 1)
    assert.equal(noise[0]!.maxChannelDelta, 1)

    // Attribute a native control's pseudo-element change to its CSS property.
    for (const path of paths) {
      const read = async (name: string): Promise<Snapshot> =>
        JSON.parse(await readFile(join(root, name, path), 'utf8'))
      const differences = [
        ...compareSnapshot(await read('changed'), await read('pseudo')),
      ]
      const color = differences.find(
        ({ property }) => property === '::placeholder.color',
      )
      assert(color, `Missing placeholder color difference: ${path}`)
      assert.equal(color.before, 'rgb(0, 128, 0)')
      assert.equal(color.after, 'rgb(255, 0, 255)')
      assert(differences.some(({ property }) => property === 'screenshot'))
    }

    // Selection color is independently observable even when it does not paint in an unselected screenshot.
    for (const path of paths) {
      const read = async (name: string): Promise<Snapshot> =>
        JSON.parse(await readFile(join(root, name, path), 'utf8'))
      const differences = [
        ...compareSnapshot(await read('pseudo'), await read('selection')),
      ]
      assert.deepEqual(
        differences.map(({ property }) => property),
        ['::selection.color'],
      )
      assert.equal(differences[0]!.before, 'rgb(255, 255, 0)')
      assert.equal(differences[0]!.after, 'rgb(255, 0, 255)')
    }

    // Cancelling a live readiness wait closes its browser and returns the caller's reason.
    const stopped = new AbortController()
    await assert.rejects(
      capture(
        {
          states: [
            {
              id: 'cancelled',
              path: '/',
              ready: async (page) => {
                stopped.abort(new Error('cancel pending capture'))
                await page.locator('#never-ready').waitFor()
              },
            },
          ],
        },
        base,
        join(root, 'cancelled'),
        stopped.signal,
      ),
      /cancel pending capture/,
    )

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
    passed = true
  } finally {
    server.closeAllConnections()
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    )
    if (passed && !process.env.STYLE_PARITY_TEST_ARTIFACTS)
      await rm(root, { recursive: true, force: true })
  }
})
