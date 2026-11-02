import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeFloatingFixture pairs the real window manager, position and resize handles. */
export async function writeFloatingFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Relocate themes and the desktop flag while retaining the actual window implementation.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/FloatingWindow.tsx'
      : 'web/ui/FloatingWindow.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'FloatingWindow.tsx'),
    source
      .replace(
        /(['"])(?:\.\.\/)+(?:web\/)?style\/([\w-]+)\.stylex\.js\1/g,
        (_match, _quote, module) =>
          JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
      )
      .replace(
        /(['"])@aptre\/bldr\1/g,
        JSON.stringify(resolve('bldr/web/electron/electron.ts')),
      ),
  )

  // Caller opacity exercises dynamic variables beside the window's own geometry variables.
  await writeFile(
    join(root, 'FloatingSamples.tsx'),
    `
import {useState}from'react';
import * as stylex from '@stylexjs/stylex';
import {FloatingWindow,FloatingWindowManagerProvider}from'./FloatingWindow';
import{LuPlus}from'@s4wave/web/images/icons.js';
const styles=stylex.create({opacity:(opacity:number)=>({opacity}),icon:{width:28,height:28}});
function WindowSample({index}:{index:number}){const[state,setState]=useState({position:{x:20+index*40,y:30+index*210},size:{width:280-index*20,height:160},expanded:true});return state.expanded?<FloatingWindow id={'floating-'+index} title={'Window '+index} testId={'floating-'+index} state={state} onStateChange={setState} icon={<LuPlus ${version === 'before' ? 'className="size-7"' : '{...stylex.props(styles.icon)}'}/>} ${version === 'before' ? 'className="opacity-90"' : 'sx={styles.opacity(.9)}'}><div style={{padding:16}}>Window content {index}</div></FloatingWindow>:null}
export function FloatingSamples(){return <FloatingWindowManagerProvider><WindowSample index={0}/><WindowSample index={1}/></FloatingWindowManagerProvider>}
`,
  )
}
