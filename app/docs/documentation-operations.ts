import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'

export async function saveDocumentationPage(
  handle: FSHandle,
  draft: string,
  signal?: AbortSignal,
): Promise<void> {
  const encoded = new TextEncoder().encode(draft)
  await handle.writeAt(0n, encoded, signal)
  await handle.truncate(BigInt(encoded.byteLength), signal)
}

export async function createDocumentationPage(
  root: FSHandle,
  existingNames: readonly string[],
  signal?: AbortSignal,
): Promise<string> {
  const existing = new Set(existingNames)
  let name = 'untitled.md'
  let counter = 1
  while (existing.has(name)) {
    name = `untitled-${counter}.md`
    counter++
  }

  await root.mknod([name], MknodType.FILE, undefined, undefined, signal)
  using child = await root.lookup(name, signal)
  const title = name.replace(/\.md$/, '')
  await saveDocumentationPage(child, `# ${title}\n`, signal)
  return name
}
