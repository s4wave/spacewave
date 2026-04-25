// sync.ts copies generated entrypoint files from the bldr build output
// into cmd/spacewave-cli/, prepending a build tag to .go files.

import { $ } from 'bun'
import { join, resolve } from 'path'
import { readdir, copyFile, readFile, writeFile } from 'fs/promises'

const dir = import.meta.dirname
const root = resolve(dir, '../..')

const goos = (await $`go env GOOS`.text()).trim()
const goarch = (await $`go env GOARCH`.text()).trim()
const src = join(root, '.bldr/build/desktop', goos, goarch, 'spacewave-cli/entrypoint')

const entries = await readdir(src)
for (const name of entries) {
  const srcPath = join(src, name)
  const dstPath = join(dir, name)
  if (name.endsWith('.go')) {
    const content = await readFile(srcPath, 'utf-8')
    const tagged = '//go:build !js\n\n' + content
    await writeFile(dstPath, tagged)
  } else if (name.endsWith('.bin')) {
    await copyFile(srcPath, dstPath)
  }
}
