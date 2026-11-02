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
import { prepareComponentStyles, readSharedStyleApis } from './component-api.js'
import { writeSheetFixture } from './sheet-fixture.js'
import { writeInputFixture } from './input-fixture.js'
import { writeButtonFixture } from './button-fixture.js'
import { writeDialogFixture } from './dialog-fixture.js'
import { writePathFixture } from './path-fixture.js'
import { writeTabsFixture } from './tabs-fixture.js'
import { writeTraceFixture } from './trace-fixture.js'
import { writeTreeFixture } from './tree-fixture.js'
import { writeBadgeFixture } from './badge-fixture.js'
import { writeFloatingFixture } from './floating-fixture.js'
import { stylexCss } from '../../bldr/web/bundler/vite/stylex.js'
import { capture } from '../style-parity/capture.js'
import { compareCaptures } from '../style-parity/compare.js'

// projectSources are real application components with layout, colors, borders and hover variants.
const projectSources = [
  'app/vm/VmErrorBanner.tsx',
  'plugin/notes/blog/TagChip.tsx',
  'web/ui/InfoCard.tsx',
  'web/ui/separator.tsx',
  'web/ui/label.tsx',
  'web/ui/EmptyState.tsx',
  'web/ui/ErrorState.tsx',
  'scripts/tailwind-to-stylex/testdata/components/ExpressionCases.tsx',
  'scripts/tailwind-to-stylex/testdata/components/OwnedStyles.tsx',
]

test(
  process.env.STYLEX_PILOT_SOURCE
    ? 'handwritten pilots preserve their captured styles'
    : 'the oracle preserves original component styles and changes nothing on a second pass',
  async (context) => {
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

    // Physical sides cannot silently outrank a later logical breakpoint rule.
    const directional = transform(
      '<div className="px-4 pr-12 sm:px-5"/>',
      'directional.tsx',
      oracle,
    )
    assert.equal(directional.converted, 0)
    assert.match(
      directional.remaining[0]!.reason,
      /conditional direction composition/,
    )

    // Dead class constants and mixed recipe imports leave no unused runtime helpers.
    const cleaned = transform(
      `import{cn}from'cnfast';import{cva,type VariantProps}from'class-variance-authority';const recipe=cva('p-1',{variants:{tone:{muted:'text-muted-foreground'}}});export function Example({tone}:VariantProps<typeof recipe>){const classes=cn('border');return <div className={cn(classes,recipe({tone}))}/>}`,
      'cleanup.tsx',
      oracle,
    )
    assert.equal(cleaned.remaining.length, 0)
    assert.doesNotMatch(cleaned.source, /\bimport\s*\{[^}]*\b(?:cn|cva)\b/)
    assert.match(cleaned.source, /\bVariantProps\b/)

    // Keep partially converted inputs available for their coordinated hand pass.
    const mixed = `function Mixed({className,sx}:{className?:string;sx?:unknown}){return <div className={className}/>}`
    assert.equal(prepareComponentStyles(mixed, 'mixed.tsx').source, mixed)

    // Close every preview after the compiler and browser contracts finish.
    const servers: Awaited<ReturnType<typeof preview>>[] = []
    let passed = false
    try {
      for (const version of ['before', 'after'] as const) {
        const root = join(parent, version)
        await mkdir(root)
        // Scan this paired source explicitly; production CSS excludes generated artifacts.
        await writeFile(
          join(root, 'fixture.css'),
          `@import ${JSON.stringify(resolve('web/style/app.css'))};\n@source ${JSON.stringify(root)};\n`,
        )
        await writeFile(
          join(root, 'index.html'),
          '<!doctype html><html lang="en"><head></head><body><div id="root"></div><script type="module" src="/main.tsx"></script></body></html>',
        )
        // Keep the original button API in the before capture as shared primitives continue converting.
        await writeFile(
          join(root, 'button.tsx'),
          (
            await readFile(
              version === 'before'
                ? 'scripts/tailwind-to-stylex/testdata/components/button.tsx'
                : 'web/ui/button.tsx',
              'utf8',
            )
          ).replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/([\w-]+)\.stylex\.js\1/g,
            (_, quote, name) =>
              JSON.stringify(resolve('web/style', name + '.stylex.ts')),
          ),
        )
        let main = `import React from 'react';import{createRoot}from'react-dom/client';import{VmErrorBanner}from'./VmErrorBanner';import{TagChip}from'./TagChip';import{InfoCard}from'./InfoCard';import{Separator}from'./separator';import{EmptyState}from'./EmptyState';import{ErrorState}from'./ErrorState';import{ExpressionCases}from'./ExpressionCases';import{OwnedStyles}from'./OwnedStyles';import${JSON.stringify(join(root, 'fixture.css'))};createRoot(document.getElementById('root')!).render(<main><VmErrorBanner title="VM unavailable" message="Choose an available image to continue."/><TagChip tag="example"/><InfoCard title="Status" icon={<span>!</span>}>Ready</InfoCard><InfoCard compact title="Compact">Ready</InfoCard><InfoCard icon={<span>!</span>}>Untitled</InfoCard><ExpressionCases/><OwnedStyles/><Separator/><EmptyState title="Empty"/><EmptyState title="Compact" variant="compact" action={{label:'Create',onClick:()=>{}}}/><EmptyState title="Panel" variant="panel" className="p-6"/><ErrorState message="Try again" onRetry={()=>{}}/><ErrorState message="Inline" variant="inline"/><ErrorState message="Fullscreen" variant="fullscreen" className="p-6"/></main>);`
        await writeTraceFixture(root, version)
        await writeTreeFixture(root, version)
        await writeBadgeFixture(root, version)
        await writeFloatingFixture(root, version)
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{TraceSamples}from'./TraceSamples';",
        )
        main = main.replace('</main>', '<TraceSamples/></main>')
        // Render the actual virtualized tree inside a bounded viewport through its existing state hook.
        main = main.replace(
          "import React from 'react';",
          `import React from 'react';import{Tree}from'@s4wave/web/ui/tree/index.js';import{LuEllipsis}from'@s4wave/web/images/icons.js';${version === 'after' ? "import{treeRowActionIconStyles}from'@s4wave/web/ui/tree/styles.stylex.js';" : ''}import{useTreeState}from ${JSON.stringify(resolve('web/ui/tree/TreeState.tsx'))};function TreeSamples(){const state=useTreeState(null,'parity-tree',{expandedIds:new Set(['parity-tree-root']),selectedIds:new Set(['parity-tree-root'])});return <section style={{height:120,width:'100%',display:'flex'}}><Tree className="max-h-75" state={state} nodes={[{id:'parity-tree-root',name:'Tree root',children:[{id:'parity-tree-child',name:'Tree child'}],icons:[{icon:<LuEllipsis ${version === 'before' ? 'className="size-7"' : '{...stylex.props(treeRowActionIconStyles.icon)}'}/>,tooltip:'Tree action'}]},{id:'parity-tree-other',name:'Other row'}]}/></section>}`,
        )
        main = main.replace('</main>', '<TreeSamples/></main>')
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
                /(['"])(?:\.\.\/)+(?:web\/)?style\/(tokens|relations)\.stylex\.js\1/g,
                (_match, _quote, module) =>
                  JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
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
          `<OutputPanel lines={['PASS first','FAIL second','[stderr] Warning','Running: Build','Error: Unavailable','Plain output']}/><OutputPanel lines={[]}/><OutputPanel lines={['Last output']} error="Build failed"/><OutputPanel lines={['INFO Starting','INFO WARN overlap','Some WARN here','Fallback']} rules={${outputRules}}/></main>`,
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
        // Compare figure separators on empty, single and multiple-item strips.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{FigureStrip}from'./FigureStrip';",
        )
        main = main.replace(
          '</main>',
          `<FigureStrip figures={[]}/><FigureStrip figures={[{label:'Workers',value:2}]}/><FigureStrip figures={[{label:'Files',value:12,unit:'of 20',detail:'Ready'},{label:'Jobs',value:3},{label:'Devices',value:4,detail:'Connected'}]}/></main>`,
        )
        const figures = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/FigureStrip.tsx'
            : 'web/ui/FigureStrip.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'FigureStrip.tsx'),
          figures.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        // Retain typography on default and compact popovers, but not their status variant.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{Popover,PopoverTrigger,PopoverContent}from'./Popover';",
        )
        main = main.replace(
          '</main>',
          ['default', 'status', 'compact']
            .map(
              (variant) =>
                `<div style={{height:250}}><Popover open><PopoverTrigger asChild><button>${variant} popover</button></PopoverTrigger><PopoverContent variant="${variant}" align="start" aria-label="${variant} information" onOpenAutoFocus={event=>event.preventDefault()}><span>Popover information</span></PopoverContent></Popover></div>`,
            )
            .join('') + '</main>',
        )
        const popover = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/Popover.tsx'
            : 'web/ui/Popover.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'Popover.tsx'),
          popover
            .replace('../style/utils.js', '@s4wave/web/style/utils.js')
            .replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
        )
        // Compare every radio selection/disabled state and the copy affordance.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{RadioOption}from'./RadioOption';import{CopyableField}from'./CopyableField';",
        )
        main = main.replace(
          '</main>',
          [false, true]
            .flatMap((selected) =>
              [false, true].map(
                (disabled) =>
                  `<RadioOption selected={${selected}} disabled={${disabled}} onSelect={()=>{}} label="Radio ${selected} ${disabled}" tag="Option" description="Details"/>`,
              ),
            )
            .join('') +
            '<CopyableField label="Identifier" value="shared-value"/></main>',
        )
        for (const name of ['RadioOption', 'CopyableField']) {
          const input = await readFile(
            version === 'before'
              ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
              : `web/ui/${name}.tsx`,
            'utf8',
          )
          await writeFile(
            join(root, `${name}.tsx`),
            input.replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
          )
        }
        // Compare native Radix tooltip placement and enabled/disabled slider rendering.
        main = main.replace(
          "import React from 'react';",
          "import React from 'react';import{Tooltip,TooltipTrigger,TooltipContent}from'./tooltip';import{Slider}from'./range-slider';",
        )
        main = main.replace(
          '</main>',
          `<div style={{height:100}}><Tooltip open><TooltipTrigger asChild><button>Tooltip anchor</button></TooltipTrigger><TooltipContent side="bottom" align="start">Tooltip information</TooltipContent></Tooltip></div><Slider aria-label="Enabled range" defaultValue={[40]} max={100}/><Slider aria-label="Disabled range" defaultValue={[60]} max={100} disabled/></main>`,
        )
        for (const name of ['tooltip', 'range-slider']) {
          const input = await readFile(
            version === 'before'
              ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
              : `web/ui/${name}.tsx`,
            'utf8',
          )
          await writeFile(
            join(root, `${name}.tsx`),
            input
              .replace('../style/utils.js', '@s4wave/web/style/utils.js')
              .replace(
                /(['"])(?:\.\.\/)+(?:web\/)?style\/(tokens|relations)\.stylex\.js\1/g,
                (_match, _quote, module) =>
                  JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
              ),
          )
        }
        // Exercise Sonner's real DOM for every toast tone, description and action slot.
        main = main.replace(
          "import React from 'react';",
          `import React,{useEffect} from 'react';import{Toaster,toast}from'./toaster';function ToastSamples(){useEffect(()=>{for(const tone of ['success','info','warning','error'] as const)toast[tone](tone,{id:tone,description:'Toast details',duration:Infinity,action:{label:'Retry',onClick:()=>{}},cancel:{label:'Dismiss',onClick:()=>{}}});toast('Message',{id:'message',description:'Message details',duration:Infinity})},[]);return <Toaster visibleToasts={5} expand closeButton/>}`,
        )
        main = main.replace('</main>', '<ToastSamples/></main>')
        const toaster = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/toaster.tsx'
            : 'web/ui/toaster.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'toaster.tsx'),
          toaster.replace('../style/sonner.js', resolve('web/style/sonner.ts')),
        )
        // Keep the ordinary Window frame and its owned border state in the historical comparison.
        main = "import{Window}from'./Window';" + main
        main = main.replace(
          '</main>',
          '<Window data-area-id="fixture-window"><button>Window action</button></Window></main>',
        )
        const window = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/Window.tsx'
            : 'web/ui/Window.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'Window.tsx'),
          window
            .replace('../style/utils.js', '@s4wave/web/style/utils.js')
            .replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
        )
        // Compare compact and regular sections, including both header action states.
        const actionProps =
          version === 'before'
            ? 'className="text-foreground-alt hover:text-foreground flex size-4 items-center justify-center transition-colors disabled:cursor-not-allowed disabled:opacity-50"'
            : 'sx={collapsibleSectionActionStyles.button}'
        main =
          `import * as stylex from '@stylexjs/stylex';import{CollapsibleSection${version === 'after' ? ',collapsibleSectionActionStyles' : ''}}from'./CollapsibleSection';` +
          main
        main = main.replace(
          '</main>',
          [false, true]
            .flatMap((compact) =>
              [false, true].flatMap((open) =>
                [false, true].map(
                  (disabled) =>
                    `<CollapsibleSection title="Section ${compact} ${open} ${disabled}" compact={${compact}} open={${open}} onOpenChange={()=>{}} icon={<span>!</span>} badge={<span>2</span>} headerActions={<button type="button" disabled={${disabled}} ${actionProps} aria-label="Section action">+</button>}><p>Section content</p></CollapsibleSection>`,
                ),
              ),
            )
            .join('') + '</main>',
        )
        const collapsible = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/CollapsibleSection.tsx'
            : 'web/ui/CollapsibleSection.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'CollapsibleSection.tsx'),
          collapsible.replace(
            /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
            JSON.stringify(resolve('web/style/tokens.stylex.ts')),
          ),
        )
        // Preserve the border mask, configurable gradient and reduced-motion treatment.
        main = "import{ShineBorder}from'./shine-border';" + main
        main = main.replace(
          '</main>',
          '<div style={{position:"relative",height:48,borderRadius:8}}><ShineBorder/></div><div style={{position:"relative",height:48,borderRadius:8}}><ShineBorder duration={7} borderWidth={2} shineColor={["#22aaff","#ff7733"]}/></div></main>',
        )
        await writeFile(
          join(root, 'shine-border.tsx'),
          await readFile(
            version === 'before'
              ? 'scripts/tailwind-to-stylex/testdata/components/shine-border.tsx'
              : 'web/ui/shine-border.tsx',
            'utf8',
          ),
        )
        // Keep header variants, caller overrides and zero/one/multiple viewer names unchanged.
        main =
          `import{PanelHeader,PanelHeaderButton}from'./PanelHeader';import{ObjectViewerProvider}from'@s4wave/web/object/ObjectViewerContext.js';` +
          main
        main = main.replace(
          "createRoot(document.getElementById('root')!).render",
          `const panelOverrides=stylex.create({header:{paddingInline:'calc(var(--spacing) * 3)'},button:{padding:'calc(var(--spacing) * 2)'}});const panelViewers=['Text','Raw'].map(name=>({componentID:name,typeID:'fixture',name,component:()=>null}));createRoot(document.getElementById('root')!).render`,
        )
        main = main.replace(
          '</main>',
          ['default', 'secondary', 'transparent', 'compact']
            .flatMap((variant) =>
              [0, 1, 2].map(
                (count) =>
                  `<ObjectViewerProvider value={{visibleComponents:panelViewers.slice(0,${count}),selectedComponent:${count ? 'panelViewers[0]' : 'undefined'},onSelectComponent:()=>{},setObjectName:()=>{}}}><PanelHeader variant="${variant}" height={${variant === 'compact' ? 40 : 25}}><span>Header content</span><PanelHeaderButton title="Header action">+</PanelHeaderButton></PanelHeader></ObjectViewerProvider>`,
              ),
            )
            .join('') +
            `<PanelHeader ${version === 'before' ? 'className="px-3"' : 'sx={panelOverrides.header}'}><PanelHeaderButton title="Override action" ${version === 'before' ? 'className="p-2"' : 'sx={panelOverrides.button}'}>+</PanelHeaderButton></PanelHeader></main>`,
        )
        const panelHeader = await readFile(
          version === 'before'
            ? 'scripts/tailwind-to-stylex/testdata/components/PanelHeader.tsx'
            : 'web/ui/PanelHeader.tsx',
          'utf8',
        )
        await writeFile(
          join(root, 'PanelHeader.tsx'),
          panelHeader
            .replace('../style/utils.js', '@s4wave/web/style/utils.js')
            .replace(
              /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
              JSON.stringify(resolve('web/style/tokens.stylex.ts')),
            ),
        )
        // Isolate modal overlays from the ordinary page's hover and layout states.
        await writeSheetFixture(root, version)
        await writeInputFixture(root, version)
        await writeButtonFixture(root, version)
        await writeDialogFixture(root, version)
        await writePathFixture(root, version)
        await writeTabsFixture(root, version)
        main = "import{FloatingSamples}from'./FloatingSamples';" + main
        main = "import{BadgeSamples}from'./BadgeSamples';" + main
        main = "import{TabsSamples}from'./TabsSamples';" + main
        main = "import{PathSamples}from'./PathSamples';" + main
        main = "import{DialogSamples}from'./DialogSamples';" + main
        main = "import{ButtonSamples}from'./ButtonSamples';" + main
        main = "import{InputSamples}from'./InputSamples';" + main
        main = "import{SheetSamples}from'./SheetSamples';" + main
        main = main.replace(
          "createRoot(document.getElementById('root')!).render",
          "if(new URLSearchParams(location.search).has('floating'))createRoot(document.getElementById('root')!).render(<FloatingSamples/>);else if(new URLSearchParams(location.search).has('badge'))createRoot(document.getElementById('root')!).render(<BadgeSamples/>);else if(new URLSearchParams(location.search).has('tree'))createRoot(document.getElementById('root')!).render(<TreeSamples/>);else if(new URLSearchParams(location.search).has('tabs'))createRoot(document.getElementById('root')!).render(<TabsSamples/>);else if(new URLSearchParams(location.search).has('path'))createRoot(document.getElementById('root')!).render(<PathSamples/>);else if(new URLSearchParams(location.search).has('dialog'))createRoot(document.getElementById('root')!).render(<DialogSamples/>);else if(new URLSearchParams(location.search).has('buttons'))createRoot(document.getElementById('root')!).render(<ButtonSamples/>);else if(new URLSearchParams(location.search).has('input'))createRoot(document.getElementById('root')!).render(<InputSamples/>);else if(new URLSearchParams(location.search).has('sheet'))createRoot(document.getElementById('root')!).render(<SheetSamples/>);else createRoot(document.getElementById('root')!).render",
        )
        // Discover the actual tree API through its public barrel before converting the caller.
        const treeImports = await readSharedStyleApis(
          "import{Tree}from'@s4wave/web/ui/tree/index.js'",
          resolve('web/ui/stylex-tree-fixture.tsx'),
        )
        const mainApis = prepareComponentStyles(main, join(root, 'main.tsx'), {
          convertProps: false,
          sharedComponents: treeImports,
        }).components
        await writeFile(
          join(root, 'main.tsx'),
          version === 'before'
            ? main.replace('@s4wave/web/ui/tree/index.js', './Tree')
            : transform(main, join(root, 'main.tsx'), oracle, {
                components: new Set([
                  'Separator',
                  'EmptyState',
                  'ErrorState',
                  ...mainApis,
                ]),
              })
                .source.replace('@s4wave/web/ui/tree/index.js', './Tree')
                .replace(
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
            /\/(OwnedStyles|separator|label|Tree|EmptyState|ErrorState)\.tsx$/.test(
              file,
            )
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
          // Exercise a dynamic native caller through the generated public API, beside its inline variable.
          let output = migration.source
          if (version === 'after' && file.endsWith('/OwnedStyles.tsx')) {
            output = output.replace(
              /(<Box\s+label="Dynamic native caller"\s+)sx=\{[^}]+\}/,
              '$1sx={compiledStyles.dynamic(123)}',
            )
            assert.notEqual(
              output,
              migration.source,
              'The dynamic native caller must reach its compiled API',
            )
            output += `
import{stylexProps as nativeAttributes}from'@s4wave/web/style/props.js';
const forwardedNoClass:{className?:never}={};
// @ts-expect-error Native attributes cannot replace a component's sx protocol.
const invalidNativeDelegate=<Box label="Invalid native attributes" {...nativeAttributes(undefined,forwardedNoClass)}/>;
void invalidNativeDelegate;
`
          }
          await writeFile(
            join(root, file.split('/').at(-1)!),
            (version === 'before'
              ? source
              : process.env.STYLEX_PILOT_SOURCE &&
                  !file.endsWith('/ExpressionCases.tsx') &&
                  !file.endsWith('/OwnedStyles.tsx')
                ? await readFile(file, 'utf8')
                : output
            )
              .replace(
                /(['"])(?:\.\.\/)+(?:web\/)?style\/(tokens|relations)\.stylex\.js\1/g,
                (_match, _quote, module) =>
                  JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
              )
              .replace(
                /(['"])\.\/(TreeNode|TreeState|TreeRow)\.js\1/g,
                (_match, _quote, module) =>
                  JSON.stringify(resolve(`web/ui/tree/${module}.tsx`)),
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

      // Require the actual toast DOM and its measured layout before observing either build.
      const ready = async (page: import('playwright').Page) => {
        await page.locator('main button').first().waitFor()
        await page
          .locator('[data-sonner-toast][data-mounted="true"]')
          .nth(4)
          .waitFor()
      }

      // Require usable tree geometry before any pointer action can exercise a row.
      const treeReady = async (page: import('playwright').Page) => {
        await page
          .getByRole('treeitem', { name: 'Tree child', exact: true })
          .waitFor()
        await page.evaluate(() => document.fonts.ready)
        const box = await page.getByRole('tree').boundingBox()
        assert(
          box && box.width > 0,
          'The tree fixture must have a usable width',
        )
      }

      // Compare default and hovered controls in both engines at every accepted size.
      const manifest = {
        states: [
          ...['default', 'dragged', 'resized', 'minimized'].map((mode) => ({
            id: `floating-${mode}`,
            path: `/?floating=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page
                .getByTestId(mode === 'minimized' ? 'floating-1' : 'floating-0')
                .waitFor({ timeout: 5000 })
              await page.evaluate(() => document.fonts.ready)
            },
            setup: async (page: import('playwright').Page) => {
              // Exercise the mounted window and require its actual state transition before capture.
              const panel = page.getByTestId('floating-0')
              await panel.waitFor({ timeout: 5000 })
              await page.evaluate(() => document.fonts.ready)
              if (mode === 'minimized')
                await panel
                  .getByRole('button', { name: 'Minimize', exact: true })
                  .click()
              if (mode === 'dragged' || mode === 'resized') {
                const handle =
                  mode === 'dragged'
                    ? panel.locator('[role="presentation"]')
                    : panel.getByRole('separator', {
                        name: 'Resize se',
                        exact: true,
                      })
                const initial = await panel.boundingBox()
                const box = await handle.boundingBox()
                assert(initial && box)
                await page.mouse.move(
                  box.x + box.width / 2,
                  box.y + box.height / 2,
                )
                await page.mouse.down()
                await page.mouse.move(
                  box.x + box.width / 2 + 30,
                  box.y + box.height / 2 + 20,
                )
                await page.mouse.up()
                const moved = await panel.boundingBox()
                assert(moved)
                if (mode === 'dragged') {
                  assert.equal(moved.x, initial.x + 30)
                  assert.equal(moved.y, initial.y + 20)
                } else {
                  assert.equal(moved.width, initial.width + 30)
                  assert.equal(moved.height, initial.height + 20)
                }
              }
            },
          })),
          ...['default', 'hovered', 'focused'].map((mode) => ({
            id: `badge-${mode}`,
            path: `/?badge=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page.locator('#badge-fixtures').waitFor()
              await page.evaluate(() => document.fonts.ready)
            },
            setup: async (page: import('playwright').Page) => {
              if (mode === 'hovered')
                await page
                  .getByRole('link', { name: 'Link 2', exact: true })
                  .hover()
              if (mode === 'focused')
                await page
                  .getByRole('link', { name: 'Invalid 4', exact: true })
                  .focus()
            },
          })),
          { id: 'components', path: '/', ready },
          {
            id: 'reduced-motion',
            path: '/',
            ready,
            setup: async (page: import('playwright').Page) => {
              await page.emulateMedia({ reducedMotion: 'reduce' })
            },
          },
          {
            id: 'hovered',
            path: '/',
            ready,
            setup: async (page: import('playwright').Page) => {
              await ready(page)
              await page.evaluate(() => document.fonts.ready)
              await page
                .getByRole('button', { name: 'Unselected branch', exact: true })
                .hover()
            },
          },
          {
            id: 'relations-hovered',
            path: '/',
            ready,
            setup: async (page: import('playwright').Page) => {
              // Choose the hover scroll position after mounted content and font metrics settle.
              await ready(page)
              await page.evaluate(() => document.fonts.ready)
              await page
                .getByRole('button', { name: 'Code hover', exact: true })
                .hover()
            },
          },
          ...['default', 'selected', 'hovered', 'coarse'].map((mode) => ({
            id: `tree-${mode}`,
            path: `/?tree=${mode}`,
            hasTouch: mode === 'coarse',
            ready: treeReady,
            setup: async (page: import('playwright').Page) => {
              await treeReady(page)
              const child = page.getByRole('treeitem', {
                name: 'Tree child',
                exact: true,
              })
              if (mode === 'hovered') await child.hover()
              if (mode === 'selected') await child.click()
            },
          })),
          ...['default', 'selected'].map((mode) => ({
            id: `tabs-${mode}`,
            path: `/?tabs=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page
                .getByRole('tab', { name: 'First 0', exact: true })
                .waitFor()
            },
            setup: async (page: import('playwright').Page) => {
              if (mode === 'selected')
                await page
                  .getByRole('tab', { name: 'Second 0', exact: true })
                  .click()
            },
          })),
          ...['default', 'editing', 'touch-default', 'touch-editing'].map(
            (mode) => ({
              id: `path-${mode}`,
              path: `/?path=${mode}`,
              hasTouch: mode.startsWith('touch'),
              ready: async (page: import('playwright').Page) => {
                await page.locator('#path-fixtures').waitFor()
              },
              setup: async (page: import('playwright').Page) => {
                if (mode.endsWith('editing'))
                  await page
                    .getByRole('button', { name: 'Edit path', exact: true })
                    .nth(1)
                    .click({ timeout: 5000 })
              },
            }),
          ),
          ...[
            'default',
            'compact',
            'editor',
            'panel',
            'caller-md',
            'caller-sm',
            'command-default',
            'command-palette',
          ].map((mode) => ({
            id: `dialog-${mode}`,
            path: `/?dialog=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page.getByRole('dialog').waitFor()
            },
          })),
          ...['default', 'focused', 'hovered'].map((mode) => ({
            id: `buttons-${mode}`,
            path: `/?buttons=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page
                .getByRole('button', { name: 'Focus button', exact: true })
                .waitFor()
            },
            setup: async (page: import('playwright').Page) => {
              if (mode === 'focused')
                await page
                  .getByRole('button', { name: 'Focus button', exact: true })
                  .focus()
              if (mode === 'hovered')
                await page
                  .getByRole('button', {
                    name: 'Dashboard active',
                    exact: true,
                  })
                  .hover()
            },
          })),
          ...['default', 'focused'].map((mode) => ({
            id: `input-${mode}`,
            path: `/?input=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page
                .getByRole('textbox', { name: 'Focus reference', exact: true })
                .waitFor()
            },
            setup: async (page: import('playwright').Page) => {
              if (mode === 'focused')
                await page
                  .getByRole('textbox', {
                    name: 'Focus reference',
                    exact: true,
                  })
                  .focus()
            },
          })),
          {
            id: 'sheet-inline',
            path: '/?sheet=inline',
            ready: async (page: import('playwright').Page) => {
              await page.getByRole('dialog').nth(5).waitFor()
            },
          },
          ...['modal', 'modal-absolute'].map((mode) => ({
            id: `sheet-${mode}`,
            path: `/?sheet=${mode}`,
            ready: async (page: import('playwright').Page) => {
              await page
                .getByRole('dialog', { name: 'Modal sheet', exact: true })
                .waitFor()
            },
          })),
        ],
      }
      // A focused follow-up retains the same browser protocol while selecting its changed states.
      const selected = process.env.STYLEX_MIGRATION_STATES?.split(',')
      if (selected) {
        for (const id of selected)
          assert(
            manifest.states.some((state) => state.id === id),
            `Unknown migration state: ${id}`,
          )
        manifest.states = manifest.states.filter((state) =>
          selected.includes(state.id),
        )
        console.log(`Selected migration states: ${selected.join(',')}`)
      }
      // Give each bounded area its own deadline and close its browser when that deadline expires.
      const groups = [
        {
          name: 'shared components',
          ids: ['components', 'reduced-motion', 'hovered', 'relations-hovered'],
        },
        {
          name: 'sheets and fields',
          ids: [
            'input-default',
            'input-focused',
            'sheet-inline',
            'sheet-modal',
            'sheet-modal-absolute',
          ],
        },
        {
          name: 'floating windows',
          ids: [
            'floating-default',
            'floating-dragged',
            'floating-resized',
            'floating-minimized',
          ],
        },
        {
          name: 'badges',
          ids: ['badge-default', 'badge-hovered', 'badge-focused'],
        },
        {
          name: 'tree rows',
          ids: ['tree-default', 'tree-selected', 'tree-hovered', 'tree-coarse'],
        },
        {
          name: 'tabs',
          ids: ['tabs-default', 'tabs-selected'],
        },
        {
          name: 'path navigation',
          ids: [
            'path-default',
            'path-editing',
            'path-touch-default',
            'path-touch-editing',
          ],
        },
        {
          name: 'dialog variants',
          ids: [
            'dialog-default',
            'dialog-compact',
            'dialog-editor',
            'dialog-panel',
          ],
        },
        {
          name: 'dialog callers',
          ids: [
            'dialog-caller-md',
            'dialog-caller-sm',
            'dialog-command-default',
            'dialog-command-palette',
          ],
        },
        {
          name: 'buttons',
          ids: ['buttons-default', 'buttons-focused', 'buttons-hovered'],
        },
      ]
      for (const state of manifest.states)
        assert(
          groups.some((group) => group.ids.includes(state.id)),
          `Ungrouped migration state: ${state.id}`,
        )
      let allPassed = true
      for (const group of groups) {
        const states = manifest.states.filter((state) =>
          group.ids.includes(state.id),
        )
        if (!states.length) continue
        let groupPassed = false
        await context.test(
          group.name,
          { timeout: 300_000 },
          async (subtest) => {
            // Observe both built revisions through the same isolated browser protocol.
            const before = join(parent, 'before-capture', group.name)
            const after = join(parent, 'after-capture', group.name)
            const paths = await capture(
              { states },
              servers[0]!.resolvedUrls!.local[0]!,
              before,
              subtest.signal,
            )
            await capture(
              { states },
              servers[1]!.resolvedUrls!.local[0]!,
              after,
              subtest.signal,
            )

            // Retain every exact geometry, accessibility, style and raster comparison.
            for (const path of paths) {
              const differences = await Array.fromAsync(
                compareCaptures(join(before, path), join(after, path)),
              )
              assert.deepEqual(
                differences,
                [],
                `${path}: ${JSON.stringify(differences.slice(0, 5))}`,
              )
            }
            groupPassed = true
          },
        )
        allPassed &&= groupPassed
      }
      passed = allPassed
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
