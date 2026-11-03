import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeReferenceFixture compares enum badges, object references and focus-scope styling. */
export async function writeReferenceFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Retain original root APIs while each current implementation uses its actual native children.
  for (const [name, path] of [
    ['StateBadge', 'web/forge/StateBadge.tsx'],
    ['ObjectLink', 'web/object/ObjectLink.tsx'],
    ['FocusContext', 'web/command/FocusContext.tsx'],
  ] as const) {
    let source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
        : path,
      'utf8',
    )
    source = source
      .replace('./TabContext.js', resolve('web/object/TabContext.tsx'))
      .replace('./object.pb.js', resolve('web/object/object.pb.ts'))
    await writeFile(join(root, name + '.tsx'), source)
  }
  const scope =
    version === 'before'
      ? 'className="p-2 text-foreground-alt"'
      : 'sx={styles.scope}'
  await writeFile(
    join(root, 'ReferenceSamples.tsx'),
    `
import React from 'react';import * as stylex from '@stylexjs/stylex';
import{StateBadge}from'./StateBadge';import{ObjectLink}from'./ObjectLink';import{FocusContextProvider}from'./FocusContext';import{CommandFocusContext}from'@s4wave/sdk/command/command.pb.js';
const styles=stylex.create({scope:{padding:'8px',color:'var(--color-foreground-alt)'}});
const labels={0:'IDLE',1:'RUNNING',2:'COMPLETED',3:'STOPPING',4:'ERROR'};
export function ReferenceSamples(){return <main id="reference-fixtures" style={{padding:16,display:'grid',gap:12,maxWidth:500}}>
{(['pill','dot']as const).flatMap(variant=>[0,1,2,3,4,99].map(state=><div key={variant+state}><StateBadge state={state} labels={labels} variant={variant}/></div>))}
{[false,true].flatMap(compact=>['object:example',''].map(objectKey=><div key={String(compact)+objectKey}><ObjectLink objectKey={objectKey} label="Example object" kind="Document" status="Ready" compact={compact}/></div>))}
<FocusContextProvider focusContext={CommandFocusContext.GLOBAL} ${scope}><button>Focus scope</button></FocusContextProvider>
</main>}
`,
  )
}
