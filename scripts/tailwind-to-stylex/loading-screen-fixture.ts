import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeLoadingScreenFixture preserves the skin shared with static bootstrap HTML. */
export async function writeLoadingScreenFixture(
  root: string,
  version: 'before' | 'after',
) {
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/LoadingScreen.tsx'
      : 'web/ui/loading/LoadingScreen.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'LoadingScreen.tsx'),
    source.replace(
      './useReducedMotion.js',
      resolve('web/ui/loading/useReducedMotion.ts'),
    ),
  )
  await writeFile(
    join(root, 'LoadingScreenSamples.tsx'),
    `
import React from 'react';import{LoadingScreen}from'./LoadingScreen';
export function LoadingScreenSamples(){const mode=new URLSearchParams(location.search).get('loading-screen');return <main id="loading-screen-fixtures" style={{height:600}}><LoadingScreen view={{state:mode==='error'?'error':'loading',title:'Preparing Spacewave',detail:'Starting the runtime',progress:.58,error:mode==='error'?'Startup did not finish.':undefined,onRetry:mode==='error'?()=>{}:undefined,onCancel:()=>{}}}/></main>}
`,
  )
}
