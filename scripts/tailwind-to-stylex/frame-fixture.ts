import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'

/** writeFrameFixture retains frame overlays, bar controls and native caller variables. */
export async function writeFrameFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Freeze the class-based control family together, including its original icon owners.
  for (const name of ['frame', 'bar', 'bottom-bar-item']) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/frame-${name}.tsx`
        : `web/frame/${name}.tsx`,
      'utf8',
    )
    await writeFile(
      join(root, `${name}.tsx`),
      source.replaceAll(
        "'./bottom-bar-context.js'",
        "'@s4wave/web/frame/bottom-bar-context.js'",
      ),
    )
  }
  const frame =
    version === 'before'
      ? 'className="min-w-[211px]"'
      : 'sx={styles.width(211)}'
  const bar =
    version === 'before'
      ? `className:'viewer-bottom-bar px-1'`
      : `sx:[viewerBarMarker,viewerBarStyles.root]`
  const icon =
    version === 'before'
      ? 'className="size-7 text-blue-500"'
      : 'sx={styles.icon}'
  await writeFile(
    join(root, 'FrameSamples.tsx'),
    `
import React,{useState}from'react';import * as stylex from '@stylexjs/stylex';
import{Frame}from'./frame';import{BottomBarItem}from'./bottom-bar-item';import{Button}from'./button';import{LuPlus}from'@s4wave/web/images/icons.js';
import{viewerBarMarker,viewerBarStyles}from'@s4wave/web/style/viewer-bar.stylex.js';
const styles=stylex.create({width:(minWidth:number)=>({minWidth}),icon:{width:'calc(var(--spacing) * 7)',height:'calc(var(--spacing) * 7)',color:'var(--color-blue-500)'}});
export function FrameSamples(){const[open,setOpen]=useState(false);return <main id="frame-fixtures" style={{height:500,maxWidth:680,display:'flex'}}><Frame ${frame} style={{opacity:.9,'--caller-frame':'retained'} as React.CSSProperties} overlay={open?<button onClick={()=>setOpen(false)}>Close overlay</button>:undefined} overlayLabel="Frame overlay" onCloseOverlay={()=>setOpen(false)} topBar={{left:<span>Frame title</span>,right:<span>Ready</span>}} bottomBar={{${bar},left:<><BottomBarItem><LuPlus ${icon}/><span>Default item</span></BottomBarItem><BottomBarItem selected><LuPlus/><span>Selected item</span></BottomBarItem><BottomBarItem disabled><LuPlus/><span>Disabled item</span></BottomBarItem><BottomBarItem trailing={<Button variant="ghost" size="iconXs" aria-label="More actions">⋯</Button>}><LuPlus/><span>Compound item</span></BottomBarItem></>,right:<BottomBarItem><LuPlus/></BottomBarItem>}}><button onClick={()=>setOpen(true)}>Open overlay</button><p>Frame content</p></Frame></main>}
`,
  )
}
