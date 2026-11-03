import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix, resolve } from 'node:path'

/** writeViewerFixture compares configuration and monitoring views through their real roots. */
export async function writeViewerFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Preserve the original modules, resolving their relative imports from their source area.
  for (const file of [
    'app/configeditors/GitRepoConfigEditor.tsx',
    'app/prom/PromTargetView.tsx',
  ]) {
    let source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/viewer-' +
            basename(file)
        : file,
      'utf8',
    )
    if (version === 'before') {
      // These views already used native controls and icons at the preserved revision.
      for (const [from, to] of [
        ['@s4wave/web/images/icons.js', 'web/images/icons.ts'],
        ['@s4wave/web/ui/input.js', 'web/ui/input.tsx'],
        ['@s4wave/web/ui/button.js', 'web/ui/button.tsx'],
      ])
        source = source.replaceAll(
          "'" + from + "'",
          JSON.stringify(resolve(to!)),
        )
    }
    await writeFile(
      join(root, 'viewer-' + basename(file)),
      source.replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ),
    )
  }

  // One populated view reaches the clone card, scrape tones, action state and optional chart slot.
  const chart =
    version === 'before'
      ? 'chart={<div style={{height:72}}>Chart</div>}'
      : 'renderChart={sx=><div sx={sx} style={{height:72}}>Chart</div>}'
  await writeFile(
    join(root, 'ViewerSamples.tsx'),
    `
import React from'react';import{GitRepoConfigEditor}from'./viewer-GitRepoConfigEditor';import{PromTargetView}from'./viewer-PromTargetView';
const noop=()=>{};export function ViewerSamples(){const clone=new URLSearchParams(location.search).get('app-helper')==='viewer-clone';return <main id="app-helper-fixtures" style={{padding:24,display:'grid',gap:24,maxWidth:720}}><GitRepoConfigEditor value={{clone,cloneOpts:{url:'https://example.com/source.git',ref:'main',depth:1,recursive:true}}} onValueChange={noop}/><PromTargetView objectKey="prom/target/example" info={{meta:{scrape:{url:'https://example.com/metrics',intervalMs:30000},labels:{labels:[{name:'region',value:'west'}]}}}} state={clone?{state:'down',status:{lastError:'Connection paused',lastScrapeMs:1893456000000n,lastDurationMs:20,series:4n,devicePeerId:'device'}}:{state:'unapproved'}} action="scrape-here" deviceName={()=> 'Desktop'} nowMs={1893456001000} saving={false} saveError={null} onAction={noop} ${chart}/></main>}
`,
  )
}
