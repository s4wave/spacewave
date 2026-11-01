import { transformSync } from '@babel/core'
import stylexPlugin from '@stylexjs/babel-plugin'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
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
  'web/ui/separator.tsx',
  'web/ui/EmptyState.tsx',
  'web/ui/ErrorState.tsx',
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

    // Unsupported shorthand must fail compilation instead of silently removing an animation.
    assert.throws(
      () =>
        transformSync(
          'import * as stylex from "@stylexjs/stylex"; const styles=stylex.create({spin:{animation:"spin 1s linear infinite"}}); export const props=stylex.props(styles.spin)',
          {
            filename: 'animation.ts',
            plugins: [
              [
                stylexPlugin,
                { runtimeInjection: false, propertyValidationMode: 'throw' },
              ],
            ],
          },
        ),
      /animation is not supported/,
    )

    // Lexical shadows keep their ordinary application contract rather than inheriting an import's meaning.
    const shadowed = transform(
      `import {cn} from 'cnfast';function Example(cn:(value:string)=>string){return <div className={cn('text-sm')}/>}`,
      'shadowed.tsx',
      oracle,
    )
    assert.equal(shadowed.converted, 0)
    assert.equal(shadowed.remaining.length, 1)

    // A global @apply alias must not silently become only the utility's color declaration.
    const alias = transform(
      '<div className="text-text-secondary"/>',
      'alias.tsx',
      oracle,
    )
    assert.equal(alias.converted, 0)
    assert.match(alias.remaining[0]!.reason, /global class adds declarations/)

    // Decline a condition merge that would change Tailwind's cascade.
    const overlapping = transform(
      `import {cn} from 'cnfast';function Example({active}:{active:boolean}){return <div className={cn('text-muted-foreground hover:text-foreground',active && 'text-brand')}/>}`,
      'overlap.tsx',
      oracle,
    )
    assert.equal(overlapping.converted, 0)
    assert.match(
      overlapping.remaining[0]!.reason,
      /conditional property composition/,
    )

    // Keep partially converted inputs available for their coordinated hand pass.
    const mixed = `function Mixed({className,sx}:{className?:string;sx?:unknown}){return <div className={className}/>}`
    assert.equal(prepareComponentStyles(mixed, 'mixed.tsx').source, mixed)

    // Close every preview after the compiler and browser contracts finish.
    const servers: Awaited<ReturnType<typeof preview>>[] = []
    let passed = false
    try {
      for (const version of ['before', 'after']) {
        const root = join(parent, version)
        await mkdir(root)
        await writeFile(
          join(root, 'index.html'),
          '<!doctype html><html lang="en"><head></head><body><div id="root"></div><script type="module" src="/main.tsx"></script></body></html>',
        )
        // Keep the original button API in the before capture as shared primitives continue converting.
        await writeFile(
          join(root, 'button.tsx'),
          await readFile(
            version === 'before'
              ? 'scripts/tailwind-to-stylex/testdata/components/button.tsx'
              : 'web/ui/button.tsx',
            'utf8',
          ),
        )
        let main = `import React from 'react';import{createRoot}from'react-dom/client';import{VmErrorBanner}from'./VmErrorBanner';import{TagChip}from'./TagChip';import{InfoCard}from'./InfoCard';import{Separator}from'./separator';import{EmptyState}from'./EmptyState';import{ErrorState}from'./ErrorState';import{ExpressionCases}from'./ExpressionCases';import{OwnedStyles}from'./OwnedStyles';import${JSON.stringify(resolve('web/style/app.css'))};createRoot(document.getElementById('root')!).render(<main><VmErrorBanner title="VM unavailable" message="Choose an available image to continue."/><TagChip tag="example"/><InfoCard title="Status" icon={<span>!</span>}>Ready</InfoCard><InfoCard compact title="Compact">Ready</InfoCard><InfoCard icon={<span>!</span>}>Untitled</InfoCard><ExpressionCases/><OwnedStyles/><Separator/><EmptyState title="Empty"/><EmptyState title="Compact" variant="compact" action={{label:'Create',onClick:()=>{}}}/><EmptyState title="Panel" variant="panel" className="p-6"/><ErrorState message="Try again" onRetry={()=>{}}/><ErrorState message="Inline" variant="inline"/><ErrorState message="Fullscreen" variant="fullscreen" className="p-6"/></main>);`
        // Preserve progress labels, bounds and reduced-motion behavior through native CSS.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{ProgressBar}from'./ProgressBar';",
        )
        main = main.replace(
          '</main>',
          '<ProgressBar value={42}/><ProgressBar value={175}/><ProgressBar indeterminate/><ProgressBar value={62} rate="1.5 MiB/s"/><ProgressBar indeterminate rate="Uploading"/></main>',
        )
        const progress = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/ProgressBar.tsx'
            : 'web/ui/loading/ProgressBar.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'ProgressBar.tsx'),
          progress.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        // Exercise every public spinner size and tone with the original implementation beside it.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{Spinner}from'./Spinner';",
        )
        main = main.replace(
          '</main>',
          ['sm', 'md', 'lg', 'xl']
            .map((size) =>
              [
                'default',
                'foreground',
                'muted',
                'subtle',
                'brand',
                'destructive',
                'success',
              ]
                .map(
                  (variant) => `<Spinner size="${size}" variant="${variant}"/>`,
                )
                .join(''),
            )
            .join('') + '</main>',
        )
        const spinner = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/Spinner.tsx'
            : 'web/ui/loading/Spinner.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'Spinner.tsx'),
          spinner.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        // Keep inline loading tones on the parent so its spinner and label inherit together.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{LoadingInline}from'./LoadingInline';",
        )
        main = main.replace(
          '</main>',
          ['brand', 'muted', 'subtle', 'destructive']
            .map(
              (tone) =>
                `<LoadingInline tone="${tone}" label="Loading ${tone}"/>`,
            )
            .join('') + '</main>',
        )
        const inline = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/LoadingInline.tsx'
            : 'web/ui/loading/LoadingInline.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'LoadingInline.tsx'),
          inline
            .replace('./Spinner.js', './Spinner')
            .replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
        )
        // Compare every loading state, optional transfer detail and recoverable error control.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{LoadingCard}from'./LoadingCard';",
        )
        main = main.replace(
          '</main>',
          ['loading', 'active', 'synced', 'error']
            .map(
              (state) =>
                `<LoadingCard view={{state:'${state}',title:'Loading card ${state}',detail:'Details',progress:0.42,rate:{up:'1 MiB/s',down:'2 MiB/s'},lastActivity:'Just now',error:${state === 'error' ? "'Try again'" : 'undefined'},onRetry:()=>{},onCancel:()=>{}}}/>`,
            )
            .join('') + '</main>',
        )
        const loading = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/LoadingCard.tsx'
            : 'web/ui/loading/LoadingCard.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'LoadingCard.tsx'),
          loading
            .replace('./Spinner.js', './Spinner')
            .replace('./ProgressBar.js', './ProgressBar')
            .replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
        )
        await writeFile(
          join(root, 'types.ts'),
          await readFile('web/ui/loading/types.ts', 'utf8'),
        )
        // Keep status labels, console typography and value overrides on their owning elements.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{StatusList}from'./StatusList';import{StatsBar}from'./StatsBar';import{AuthProgressCard}from'./AuthProgressCard';import * as fixtureStyles from '@stylexjs/stylex';const valueStyles=fixtureStyles.create({brand:{color:'var(--color-brand)'},warning:{color:'var(--color-warning)'}});",
        )
        const valueOverride =
          version === 'before'
            ? "valueClassName:'text-brand'"
            : 'valueSx:valueStyles.brand'
        main = main.replace(
          '</main>',
          `<StatusList items={(['success','error','pending','none'] as const).map(status=>({id:status,label:status,status,detail:'Details'}))}/><StatsBar stats={[]}/><StatsBar stats={[{label:'Files',value:12},{label:'Ready',value:'Yes',${valueOverride}}]}/><AuthProgressCard title="Authentication" detail="Preparing credentials" steps={['Create key','Verify','Finish']}/></main>`,
        )
        for (const [name, file] of [
          ['StatusList', 'web/ui/StatusList.tsx'],
          ['StatsBar', 'web/ui/StatsBar.tsx'],
          ['AuthProgressCard', 'web/ui/credential/AuthProgressCard.tsx'],
        ]) {
          const input = await readFile(
            version === 'before'
              ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
              : file!,
            'utf8',
          )
          await writeFile(
            join(root, `${name}.tsx`),
            input
              .replace('@s4wave/web/ui/loading/Spinner.js', './Spinner')
              .replace('@s4wave/web/ui/loading/ProgressBar.js', './ProgressBar')
              .replace(
                /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
                JSON.stringify(resolve('web/style/tokens.stylex.ts')),
              ),
          )
        }
        // Preserve default and caller-selected output rules without accepting class strings.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{OutputPanel}from'./OutputPanel';",
        )
        const outputRules =
          version === 'before'
            ? "[{match:'INFO',className:'text-brand'},{match:(line:string)=>line.includes('WARN'),className:'text-warning'}]"
            : "[{match:'INFO',sx:valueStyles.brand},{match:(line:string)=>line.includes('WARN'),sx:valueStyles.warning}]"
        main = main.replace(
          '</main>',
          `<OutputPanel lines={['PASS first','FAIL second','[stderr] Warning','Running: Build','Error: Unavailable','Plain output']}/><OutputPanel lines={[]}/><OutputPanel lines={['Last output']} error="Build failed"/><OutputPanel lines={['INFO Starting','Some WARN here','Fallback']} rules={${outputRules}}/></main>`,
        )
        const output = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/OutputPanel.tsx'
            : 'web/ui/OutputPanel.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'OutputPanel.tsx'),
          output.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        // Compare every existing card separator/action combination against its explicit new inputs.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{Card,CardHeader,CardTitle,CardDescription,CardContent,CardFooter,CardAction}from'./card';",
        )
        let cards = ''
        for (const bordered of [false, true])
          for (const action of [false, true]) {
            const separator = bordered
              ? version === 'before'
                ? 'className="border-b"'
                : 'bordered'
              : ''
            const footer = bordered
              ? version === 'before'
                ? 'className="border-t"'
                : 'bordered'
              : ''
            cards += `<Card><CardHeader ${separator}><CardTitle>Card</CardTitle><CardDescription>Description</CardDescription>${action ? '<CardAction><button>Action</button></CardAction>' : ''}</CardHeader><CardContent>Content</CardContent><CardFooter ${footer}>Footer</CardFooter></Card>`
          }
        main = main.replace('</main>', cards + '</main>')
        const card = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/card.tsx'
            : 'web/ui/card.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'card.tsx'),
          card.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        await writeFile(
          join(root, 'main.tsx'),
          version === 'before'
            ? main
            : transform(main, join(root, 'main.tsx'), oracle, {
                components: new Set(['Separator', 'EmptyState', 'ErrorState']),
              }).source.replace(
                /(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js/g,
                resolve('web/style/tokens.stylex.ts'),
              ),
        )
        for (const file of projectSources) {
          const source = await readFile(
            resolve(
              'scripts/tailwind-to-stylex/testdata/components',
              file.split('/').at(-1)!,
            ),
            'utf8',
          )
          const componentApi =
            /\/(OwnedStyles|separator|EmptyState|ErrorState)\.tsx$/.test(file)
          const prepare = componentApi
            ? prepareComponentStyles
            : (source: string) => ({ source, components: new Set<string>() })
          const prepared = prepare(source, file)
          const options = {
            components: prepared.components,
            componentProps: componentApi,
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
            )
              .replace(
                /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
                JSON.stringify(resolve('web/style/tokens.stylex.ts')),
              )
              .replace(
                /(['"])\.\/button\.js\1/g,
                JSON.stringify(join(root, 'button.tsx')),
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

      // Check the generated public prop types as well as their compiled runtime behavior.
      const config = join(parent, 'tsconfig.json')
      await writeFile(
        config,
        JSON.stringify({
          extends: resolve('tsconfig.json'),
          compilerOptions: {
            allowImportingTsExtensions: true,
            types: ['vite/client'],
          },
          include: [
            'after/*.tsx',
            resolve('bldr/web/bldr-react/stylex-types.ts'),
            resolve('web/react-css.d.ts'),
          ],
          exclude: [],
        }),
      )
      try {
        await promisify(execFile)(
          'bun',
          ['run', 'check:stylex-migrate-fixture', config],
          { timeout: 30_000 },
        )
      } catch (error) {
        const output = error as { stdout?: string; stderr?: string }
        await writeFile(
          join(parent, 'typecheck.log'),
          `${output.stdout ?? ''}\n${output.stderr ?? ''}`,
        )
        throw error
      }

      // Compare default and hovered controls in both engines at every accepted size.
      const manifest = {
        states: [
          { id: 'components', path: '/', ready: 'main button' },
          {
            id: 'reduced-motion',
            path: '/',
            ready: 'main button',
            setup: async (page: import('playwright').Page) => {
              await page.emulateMedia({ reducedMotion: 'reduce' })
            },
          },
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
      passed = true
    } finally {
      for (const server of servers)
        await new Promise<void>((resolve, reject) =>
          server.httpServer.close((error) =>
            error ? reject(error) : resolve(),
          ),
        )
      if (passed && !process.env.STYLEX_MIGRATION_ARTIFACTS)
        await rm(parent, { recursive: true, force: true })
      else console.log(`Migration artifacts: ${parent}`)
    }
  },
)
