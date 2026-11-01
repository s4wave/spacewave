import { readFile } from 'node:fs/promises'
import { PNG } from 'pngjs'

import { compareSnapshot, type Difference, type Snapshot } from './model.js'

// readRasterVariance accepts at most one 8-bit compositor rounding level in an otherwise identical state.
async function readRasterVariance(beforeFile: string, afterFile: string) {
  // Decode only a lone image-hash difference; semantic changes retain their screenshot difference directly.
  const before = PNG.sync.read(
    await readFile(beforeFile.replace(/\.json$/, '.png')),
  )
  const after = PNG.sync.read(
    await readFile(afterFile.replace(/\.json$/, '.png')),
  )
  if (before.width !== after.width || before.height !== after.height)
    return null
  let pixels = 0
  let maxChannelDelta = 0

  // Bound every changed pixel, rather than averaging away a small but significant image defect.
  for (let index = 0; index < before.data.length; index += 4) {
    let changed = false
    for (let channel = 0; channel < 4; channel++) {
      const delta = Math.abs(
        before.data[index + channel]! - after.data[index + channel]!,
      )
      if (delta > 1) return null
      maxChannelDelta = Math.max(maxChannelDelta, delta)
      changed ||= delta !== 0
    }
    if (changed) pixels++
  }
  return { pixels, maxChannelDelta }
}

/** compareCaptures compares real capture files and reports one-level raster variance separately. */
export async function* compareCaptures(
  beforeFile: string,
  afterFile: string,
  onRasterVariance?: (variance: {
    state: string
    pixels: number
    maxChannelDelta: number
  }) => void | Promise<void>,
): AsyncGenerator<Difference> {
  // Hold one pair of states; the structured comparator runs before the optional pixel decode.
  const before: Snapshot = JSON.parse(await readFile(beforeFile, 'utf8'))
  const after: Snapshot = JSON.parse(await readFile(afterFile, 'utf8'))
  let changed = false
  for (const difference of compareSnapshot(before, after)) {
    if (difference.property === 'screenshot' && !changed) {
      const variance = await readRasterVariance(beforeFile, afterFile)
      if (variance) {
        await onRasterVariance?.({ state: difference.state, ...variance })
        continue
      }
    }
    changed = true
    yield difference
  }
}
