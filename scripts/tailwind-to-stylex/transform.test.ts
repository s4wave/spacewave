import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { build, preview } from 'vite'
import react from '@vitejs/plugin-react'
import tailwind from '@tailwindcss/vite'

import { createOracle } from './oracle.js'
import { transform } from './transform.js'
import { prepareComponentStyles } from './component-api.js'
import { stylexCss } from '../../bldr/web/bundler/vite/stylex.js'
import { capture } from '../style-parity/capture.js'
import { compareCaptures } from '../style-parity/compare.js'

// projectSources are real application components with layout, colors, borders and hover variants.
const projectSources = [
  'app/vm/VmErrorBanner.tsx',
  'plugin/notes/blog/TagChip.tsx',
  'web/ui/InfoCard.tsx',
  'scripts/tailwind-to-stylex/testdata/components/ExpressionCases.tsx',
  'scripts/tailwind-to-stylex/testdata/components/OwnedStyles.tsx',
]

test(
  process.env.STYLEX_PILOT_SOURCE
    ? 'handwritten pilots preserve their captured styles'
    : 'the oracle preserves original component styles and changes nothing on a second pass',
  async () => {
    // Build both versions through the ordinary Tailwind and native StyleX compilers.
    await mkdir('.bldr', { recursive: true })
    const parent = await mkdtemp(resolve('.bldr/stylex-migration-fixture-'))
    const oracle = await createOracle(resolve('web/style/app.css'))

    // Lexical shadows keep their ordinary application contract rather than inheriting an import's meaning.
    const shadowed = transform(
      `import {cn} from 'cnfast';function Example(cn:(value:string)=>string){return <div className={cn('text-sm')}/>}`,
      'shadowed.tsx',
      oracle,
    )
    assert.equal(shadowed.converted, 0)
    assert.equal(shadowed.remaining.length, 1)
    const servers: Awaited<ReturnType<typeof preview>>[] = []
    try {
      for (const version of ['before', 'after']) {
        const root = join(parent, version)
        await mkdir(root)
        await writeFile(
          join(root, 'index.html'),
          '<!doctype html><html lang="en"><head></head><body><div id="root"></div><script type="module" src="/main.tsx"></script></body></html>',
        )
        await writeFile(
          join(root, 'main.tsx'),
          `import React from 'react';import{createRoot}from'react-dom/client';import{VmErrorBanner}from'./VmErrorBanner';import{TagChip}from'./TagChip';import{InfoCard}from'./InfoCard';import{ExpressionCases}from'./ExpressionCases';import{OwnedStyles}from'./OwnedStyles';import${JSON.stringify(resolve('web/style/app.css'))};createRoot(document.getElementById('root')!).render(<main><VmErrorBanner title="VM unavailable" message="Choose an available image to continue."/><TagChip tag="example"/><InfoCard title="Status" icon={<span>!</span>}>Ready</InfoCard><InfoCard compact title="Compact">Ready</InfoCard><InfoCard icon={<span>!</span>}>Untitled</InfoCard><ExpressionCases/><OwnedStyles/></main>);`,
        )
        for (const file of projectSources) {
          const source = await readFile(
            resolve(
              'scripts/tailwind-to-stylex/testdata/components',
              file.split('/').at(-1)!,
            ),
            'utf8',
          )
          const prepare = file.endsWith('/OwnedStyles.tsx')
            ? prepareComponentStyles
            : (source: string) => ({ source, components: new Set<string>() })
          const prepared = prepare(source, file)
          const options = {
            components: prepared.components,
            componentProps: file.endsWith('/OwnedStyles.tsx'),
          }
          const migration = transform(prepared.source, file, oracle, options)
          assert.equal(migration.remaining.length, 0, file)
          assert(migration.converted > 0, file)
          assert.equal(
            transform(
              prepare(migration.source, file).source,
              file,
              oracle,
              options,
            ).source,
            migration.source,
          )
          await writeFile(
            join(root, file.split('/').at(-1)!),
            (version === 'before'
              ? source
              : process.env.STYLEX_PILOT_SOURCE &&
                  !file.endsWith('/ExpressionCases.tsx') &&
                  !file.endsWith('/OwnedStyles.tsx')
                ? await readFile(file, 'utf8')
                : migration.source
            ).replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
          )
        }
        const config = {
          configFile: false as const,
          root,
          plugins: [stylexCss(), tailwind(), react()],
          resolve: { alias: { '@s4wave/web': resolve('web') } },
          logLevel: 'warn' as const,
          preview: { host: '127.0.0.1', port: 0 },
        }
        await build(config)
        servers.push(await preview(config))
      }

      // Compare default and hovered controls in both engines at every accepted size.
      const manifest = {
        states: [
          { id: 'components', path: '/', ready: 'main button' },
          {
            id: 'hovered',
            path: '/',
            ready: 'main button',
            setup: async (page: import('playwright').Page) => {
              await page
                .getByRole('button', { name: 'example', exact: true })
                .hover()
            },
          },
        ],
      }
      const paths = await capture(
        manifest,
        servers[0]!.resolvedUrls!.local[0]!,
        join(parent, 'before-capture'),
      )
      await capture(
        manifest,
        servers[1]!.resolvedUrls!.local[0]!,
        join(parent, 'after-capture'),
      )
      for (const path of paths) {
        const differences = await Array.fromAsync(
          compareCaptures(
            join(parent, 'before-capture', path),
            join(parent, 'after-capture', path),
          ),
        )
        assert.deepEqual(
          differences,
          [],
          `${path}: ${JSON.stringify(differences.slice(0, 5))}`,
        )
      }
    } finally {
      for (const server of servers)
        await new Promise<void>((resolve, reject) =>
          server.httpServer.close((error) =>
            error ? reject(error) : resolve(),
          ),
        )
      if (!process.env.STYLEX_MIGRATION_ARTIFACTS)
        await rm(parent, { recursive: true, force: true })
      else console.log(`Migration artifacts: ${parent}`)
    }
  },
)
