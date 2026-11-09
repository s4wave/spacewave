// stylex-dry folds the Tailwind codemod's StyleX output into tokens and the kit.
//
// Usage: bun scripts/stylex-dry/main.ts [tw] [space] [kit] [--stats]
// With no pass names it runs every pass in order. Each pass keeps every
// element's computed style; scripts/style-parity verifies that claim.
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Source } from './source.ts'
import {
  STYLE_DIR,
  type TwValues,
  collectCssTwValues,
  collectTwValues,
  pruneImports,
  readVars,
  registeredInitials,
  removeDeadSetters,
  resolveTw,
  spaceModule,
  spacing,
} from './passes.ts'
import { type Usage, extractKit, kitIndex, loadKit } from './kit.ts'

const root = resolve(import.meta.dirname, '../..')

/** gitFiles lists tracked files matching pathspecs. */
function gitFiles(...specs: string[]): string[] {
  return execFileSync('git', ['ls-files', '-z', '--', ...specs], {
    cwd: root,
    encoding: 'utf8',
  })
    .split('\0')
    .filter(Boolean)
    .map((f) => resolve(root, f))
}

/** styleFiles lists the sources that declare StyleX styles, excluding tests and tools. */
function styleFiles(): string[] {
  return gitFiles('*.ts', '*.tsx').filter((f) => {
    const rel = f.slice(root.length + 1)
    if (/\.(test|e2e\.test)\.tsx?$/.test(rel) || rel.startsWith('scripts/'))
      return false
    if (rel.startsWith(STYLE_DIR + '/kit/')) return false
    return /import \* as \w+ from '@stylexjs\/stylex'/.test(
      readFileSync(f, 'utf8'),
    )
  })
}

/** each runs a rewrite over every style file and commits the edits. */
function each(label: string, files: string[], fn: (s: Source) => number) {
  let total = 0
  let touched = 0
  for (const f of files) {
    const s = new Source(f)
    const n = fn(s)
    total += n
    if (s.commit()) touched++
  }
  console.log(`${label}: ${total} rewrites in ${touched} files`)
}

const args = process.argv.slice(2)
const passes = args.filter((a) => !a.startsWith('--'))
const run = (p: string) => !passes.length || passes.includes(p)
const files = styleFiles()

if (run('tw')) {
  const css = gitFiles('*.css')
    .map((f) => readFileSync(f, 'utf8'))
    .join('\n')
  const initial = registeredInitials(css)
  const values: TwValues = new Map()
  for (const f of files) collectTwValues(new Source(f), values)
  collectCssTwValues(css, initial, values)
  each('resolve --tw-* readers', files, (s) => resolveTw(s, initial, values))
  // A setter survives while any TypeScript or CSS still reads its variable.
  const read = new Set<string>()
  for (const f of [...gitFiles('*.ts', '*.tsx'), ...gitFiles('*.css')])
    readVars(readFileSync(f, 'utf8'), read)
  each('remove unread --tw-* setters', files, (s) => removeDeadSetters(s, read))
}

if (run('space')) {
  const scale = new Set<number>()
  const modulePath = resolve(root, STYLE_DIR, 'space.stylex.ts')
  if (existsSync(modulePath)) {
    for (const m of readFileSync(modulePath, 'utf8').matchAll(
      /^\s+'?(-?[\d.]+)'?:/gm,
    ))
      scale.add(Number(m[1]))
  }
  // The kit's own steps belong to the scale too.
  for (const f of gitFiles(STYLE_DIR + '/kit/*.fragments.ts')) {
    for (const m of readFileSync(f, 'utf8').matchAll(
      /space\[\s*'?(-?[\d.]+)'?\s*\]/g,
    ))
      scale.add(Number(m[1]))
  }
  each('spacing scale', files, (s) => spacing(s, scale, root))
  writeFileSync(modulePath, spaceModule(scale))
}

if (run('kit')) {
  writeFileSync(resolve(root, STYLE_DIR, 'kit.fragments.ts'), kitIndex(root))
  const kit = loadKit(root)
  const usage: Usage = new Map()
  // Token modules export only variables and constants, so they keep their sets.
  const components = files.filter((f) => !/\.stylex\.tsx?$/.test(f))
  each('kit extraction', components, (s) => extractKit(s, kit, root, usage))
  for (const e of kit.kit) {
    const n = usage.get(`${e.ns}.${e.key}`) ?? 0
    if (args.includes('--stats') || !n) console.log(`  ${e.ns}.${e.key}: ${n}`)
  }
}

each('prune unused token imports', files, pruneImports)
