import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix, resolve } from 'node:path'

/** writeHelpersFixture compares resource tones, lifecycle badges and real virtual file rows. */
export async function writeHelpersFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Freeze each coupled family, retaining the shared native icon owner of this checkpoint.
  const files = [
    'web/sdk/app/lifecycle.tsx',
    'web/devtools/ResourceDetailsPanel.tsx',
    'web/editors/file-browser/FileList.tsx',
    'web/editors/file-browser/FileListEntry.tsx',
  ]
  const names = new Set(files.map((file) => basename(file)))
  for (const file of files) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/helpers-${basename(file)}`
        : file,
      'utf8',
    )
    const relocated = source.replace(
      /(from\s*['"])(\.[^'"]+)(['"])/g,
      (_, prefix, ref, suffix) =>
        names.has(basename(ref).replace(/\.js$/, '.tsx'))
          ? `${prefix}./${basename(ref)}${suffix}`
          : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
    )
    await writeFile(
      join(root, basename(file)),
      version === 'before'
        ? relocated.replaceAll(
            "'@s4wave/web/images/icons.js'",
            JSON.stringify(resolve('web/images/icons.ts')),
          )
        : relocated,
    )
  }

  // Each resource's visible fields are fixed; loading duration belongs to the live DevTools journey.
  await writeFile(
    join(root, 'HelpersSamples.tsx'),
    `
import React from'react';import{PluginLifecycleBadge}from'./lifecycle';import{ResourceDetailsPanel}from'./ResourceDetailsPanel';import{FileList,useFileListState}from'./FileList';
import{StateNamespaceProvider,atom}from'@s4wave/web/state/persist.js';import type{TrackedResource}from'@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js';import type{SpacePluginLifecycleState}from'@s4wave/sdk/space/space.pb.js';
// The resource duration is a fixed fixture input; other fixture pages keep their real clock.
const fixtureTime=Date.UTC(2026,0,1);if(new URLSearchParams(location.search).get('helpers')?.startsWith('resources'))Date.now=()=>fixtureTime;
const state=atom({});const entries=[{id:'docs',name:'Documents',isDir:true},{id:'pictures',name:'Pictures',isDir:true},{id:'readme',name:'README.md',isDir:false},{id:'notes',name:'notes.txt',isDir:false}];
const resources:TrackedResource[]=(['ready','loading','error']as const).map(status=>({id:'resource-'+status,resourceId:7,resourceType:'ExampleResource',released:status==='error',parentIds:[],state:status,error:status==='error'?{name:'Error',message:'Example unavailable',stack:'Example resource could not load.'}:null,retry:()=>{},createdAt:fixtureTime-30000,debugLabel:'Example',debugDetails:{attempt:1,available:status==='ready'}}));
function FileSamples(){const state=useFileListState();return <div style={{height:380,width:'100%'}}><FileList state={state} entries={entries} dropTargetEntryId="docs"/></div>}
export function HelpersSamples(){const mode=new URLSearchParams(location.search).get('helpers')??'badges';return <main id="helpers-fixtures" style={{padding:16,width:'100%',display:'flex',flexWrap:'wrap',alignContent:'start',gap:12}}><StateNamespaceProvider rootAtom={state}>{mode==='badges'?<>{[0,1,2,3,4,5,6,7,99].map(value=><PluginLifecycleBadge key={value} plugin={{state:value as SpacePluginLifecycleState}}/>)}</>:mode.startsWith('resources')?<>{resources.map(resource=><ResourceDetailsPanel key={resource.id} resource={resource}/>)}</>:<FileSamples/>}</StateNamespaceProvider></main>}
`,
  )
}
