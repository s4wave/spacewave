import { createHash } from 'node:crypto'
import { readFile, writeFile, mkdir, open } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { createWriteStream } from 'node:fs'
import { Readable } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { pathToFileURL } from 'node:url'

import { capture, type ParityManifest } from './style-parity/capture.js'
import type { CaptureIndex } from './style-parity/model.js'
import { compareCaptures } from './style-parity/compare.js'

// main dispatches capture and comparison without starting an application server.
async function main(argv: string[]) {
  // Select the operation and require a value for every requested option.
  const [command, ...args] = argv
  const option = (name: string) => {
    const index = args.indexOf(name)
    if (index < 0 || !args[index + 1] || args[index + 1].startsWith('--')) {
      throw new Error(`Missing ${name}`)
    }
    return args[index + 1]
  }
  if (command === 'compare') {
    await compare(
      resolve(option('--before')),
      resolve(option('--after')),
      resolve(option('--out')),
    )
    return
  }
  if (command !== 'capture') {
    throw new Error(
      'Usage: style-parity capture --base URL --revision COMMIT --manifest FILE --out DIR | compare --before DIR --after DIR --out FILE',
    )
  }

  // Validate the command before reserving its fresh output directory.
  const base = new URL(option('--base')).href
  const revision = option('--revision')
  const sourceStatus = args.includes('--source-status')
    ? option('--source-status')
    : 'unspecified'
  const out = resolve(option('--out'))
  const manifestPath = resolve(option('--manifest'))
  const manifestBytes = await readFile(manifestPath)
  const manifestHash = createHash('sha256').update(manifestBytes).digest('hex')
  const { default: manifest } = (await import(
    pathToFileURL(manifestPath).href
  )) as { default: ParityManifest }

  // Capture the complete matrix before publishing its success index.
  await mkdir(out, { recursive: false })
  const startedAt = new Date().toISOString()
  const started = performance.now()
  const snapshots = await capture(manifest, base, out)
  if (!(await readFile(manifestPath)).equals(manifestBytes))
    throw new Error(
      'The manifest changed during capture; rerun with one fixed manifest',
    )
  const index: CaptureIndex = {
    format: 2,
    revision,
    sourceStatus,
    manifestHash,
    startedAt,
    elapsedMs: performance.now() - started,
    snapshots,
  }
  await writeFile(join(out, 'capture.json'), JSON.stringify(index, null, 2))
}

// compare streams the report while holding only one pair of screens in memory.
async function compare(beforeDir: string, afterDir: string, out: string) {
  // Verify the capture matrix before inspecting any render differences.
  const readIndex = async (directory: string): Promise<CaptureIndex> =>
    JSON.parse(await readFile(join(directory, 'capture.json'), 'utf8'))
  const before = await readIndex(beforeDir)
  const after = await readIndex(afterDir)
  if (
    before.format !== 2 ||
    after.format !== 2 ||
    before.manifestHash !== after.manifestHash ||
    JSON.stringify(before.snapshots) !== JSON.stringify(after.snapshots)
  ) {
    throw new Error(
      'The capture manifests or matrices differ; capture the same states on both revisions',
    )
  }

  // Stream each screen's differences with backpressure and propagated I/O errors.
  await mkdir(resolve(out, '..'), { recursive: true })
  let differences = 0
  let rasterVariances = 0
  const rasterPath = out.replace(/\.jsonl$/, '') + '.raster.jsonl'
  const rasterReport = await open(rasterPath, 'w')
  async function* rows() {
    for (const path of before.snapshots) {
      for await (const difference of compareCaptures(
        join(beforeDir, path),
        join(afterDir, path),
        async (variance) => {
          rasterVariances++
          await rasterReport.write(JSON.stringify(variance) + '\n')
        },
      )) {
        differences++
        yield `${JSON.stringify(difference)}\n`
      }
    }
  }
  try {
    await pipeline(Readable.from(rows()), createWriteStream(out))
  } finally {
    await rasterReport.close()
  }

  // A difference fails the command while retaining its complete report.
  console.log(
    `${differences} differences; ${rasterVariances} one-level raster variations; report: ${out}; raster report: ${rasterPath}`,
  )
  if (differences) process.exitCode = 1
}

await main(process.argv.slice(2))
