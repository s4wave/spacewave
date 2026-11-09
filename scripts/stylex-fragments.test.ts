import { expect, test } from 'bun:test'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { parseSync, transformFromAstSync } from '@babel/core'
import { inlineFragments } from '../bldr/web/bundler/vite/stylex-fragments.js'

// compile parses a consumer, inlines its fragment spreads and prints the result.
async function compile(dir: string, source: string) {
  const file = join(dir, 'Consumer.tsx')
  const ast = parseSync(source, {
    filename: file,
    babelrc: false,
    configFile: false,
    parserOpts: { plugins: ['typescript', 'jsx'] },
  })!
  const deps = await inlineFragments(ast, file, async (spec, importer) =>
    resolve(importer, '..', spec.replace(/\.js$/, '.ts')),
  )
  const out = transformFromAstSync(ast, source, {
    filename: file,
    babelrc: false,
    configFile: false,
    code: true,
  })!
  return { code: out.code!, deps }
}

test('fragment spreads become static properties with token imports', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'stylex-fragments-'))
  try {
    await writeFile(join(dir, 'tokens.stylex.ts'), `export const fonts = {}\n`)
    await writeFile(
      join(dir, 'text.fragments.ts'),
      [
        `import { fonts } from './tokens.stylex.js'`,
        `const lead = 'var(--tw-leading, 1rem)'`,
        `export const text = {`,
        `  xs: { fontSize: fonts['--text-xs'], lineHeight: lead },`,
        `} as const`,
      ].join('\n'),
    )
    await writeFile(
      join(dir, 'kit.fragments.ts'),
      `export { text } from './text.fragments.js'\n`,
    )
    const { code, deps } = await compile(
      dir,
      [
        `import * as stylex from '@stylexjs/stylex'`,
        `import * as kit from './kit.fragments.js'`,
        `const styles = stylex.create({ title: { ...kit.text.xs, color: 'red' } })`,
      ].join('\n'),
    )
    expect(code).not.toContain('kit.fragments')
    expect(code).not.toContain('...')
    expect(code).toContain(`fontSize: __stylexFragment`)
    expect(code).toContain(`['--text-xs']`)
    expect(code).toContain(`lineHeight: 'var(--tw-leading, 1rem)'`)
    expect(code).toContain(`color: 'red'`)
    expect(code).toMatch(
      /import \{ fonts as __stylexFragment\d+ \} from ".*tokens\.stylex\.ts"/,
    )
    expect(deps.some((d) => d.endsWith('text.fragments.ts'))).toBe(true)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})

test('a non-static fragment value is rejected', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'stylex-fragments-'))
  try {
    await writeFile(
      join(dir, 'bad.fragments.ts'),
      `export const bad = { x: { width: Math.random() } } as const\n`,
    )
    await expect(
      compile(
        dir,
        `import * as b from './bad.fragments.js'\nconst s = { a: { ...b.bad.x } }\n`,
      ),
    ).rejects.toThrow()
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})
