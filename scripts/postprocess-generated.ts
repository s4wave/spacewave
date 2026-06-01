import { readFile, writeFile } from 'fs/promises'
import { resolve } from 'path'

const root = resolve(import.meta.dirname, '..')

async function ensureBuildTag(path: string, tag: string) {
  const fullPath = resolve(root, path)
  const content = await readFile(fullPath, 'utf-8')
  const header = `//go:build ${tag}\n\n`
  if (content.startsWith(header)) {
    return
  }
  if (content.startsWith('//go:build ')) {
    throw new Error(`${path} already has a different build tag`)
  }
  await writeFile(fullPath, header + content)
}

await ensureBuildTag('sdk/layout/layout_srpc.pb.go', '!goscript')
