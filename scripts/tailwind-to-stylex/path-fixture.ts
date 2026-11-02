import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writePathFixture prepares the actual breadcrumb and editable path states. */
export async function writePathFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the old and new control isolated while retaining the real theme and icon owner.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/PathInput.tsx'
      : 'web/ui/path/PathInput.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'PathInput.tsx'),
    source.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
      JSON.stringify(resolve('web/style/tokens.stylex.ts')),
    ),
  )

  // Both ordinary and touch layouts exercise the root and long active breadcrumb.
  const override =
    version === 'before' ? 'className="mt-1"' : 'sx={styles.caller}'
  await writeFile(
    join(root, 'PathSamples.tsx'),
    `
import React from 'react';
import * as stylex from '@stylexjs/stylex';
import{PathInput}from'./PathInput';
const styles=stylex.create({caller:{marginTop:'calc(var(--spacing) * 1)'}});
export function PathSamples(){
 const mode=new URLSearchParams(location.search).get('path')!;
 const touch=mode.startsWith('touch');
 return <main id="path-fixtures" style={{width:'100%',minWidth:0}}>
  <PathInput path="/" mobileTouchTargets={touch} ${override}/>
  <PathInput path={mode.endsWith('editing')?'/Users/example':'/Users/testuser/Documents/Very long working directory name'} mobileTouchTargets={touch} ${override}/>
 </main>;
}
`,
  )
}
