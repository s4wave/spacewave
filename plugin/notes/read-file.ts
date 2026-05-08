import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

export async function readFileText(
  handle: FSHandle,
  abortSignal?: AbortSignal,
): Promise<string> {
  const size = await handle.getSize(abortSignal)
  if (size === 0n) return ''

  const result = await handle.readAt(0n, size, abortSignal)
  return new TextDecoder().decode(result.data)
}
