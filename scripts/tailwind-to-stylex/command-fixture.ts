import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'

/** writeCommandFixture exercises the real filtering, group headings and modal styles. */
export async function writeCommandFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Pair original class contracts with native owner styles at the same DOM elements.
  const icon =
    version === 'before'
      ? 'className="size-7 text-blue-500"'
      : 'sx={styles.icon}'
  const caller =
    version === 'before' ? 'className="min-w-40"' : 'sx={styles.root(160)}'
  const list =
    version === 'before'
      ? `className={variant==='folder'?'[&_[cmdk-list-sizer]]:grid [&_[cmdk-list-sizer]]:grid-cols-2 [&_[cmdk-list-sizer]]:items-start':undefined}`
      : `sizerSx={variant==='folder'&&styles.sizer}`
  const heading =
    version === 'before'
      ? 'className="[&_[cmdk-group-heading]]:opacity-80"'
      : 'headingSx={styles.heading}'
  await writeFile(
    join(root, 'CommandSamples.tsx'),
    `
import React from 'react';
import * as stylex from '@stylexjs/stylex';
import {Command,CommandDialog,CommandInput,CommandList,CommandEmpty,CommandGroup,CommandItem,CommandSeparator,CommandShortcut,CommandFooter} from './command';
import {LuPlus} from '@s4wave/web/images/icons.js';
const styles=stylex.create({sizer:{display:'grid',gridTemplateColumns:'repeat(2, minmax(0, 1fr))',alignItems:'flex-start'},heading:{opacity:.8},icon:{width:'calc(var(--spacing) * 7)',height:'calc(var(--spacing) * 7)',color:'var(--color-blue-500)'},root:(width:number)=>({minWidth:width})});
function Options({variant}:{variant?:'default'|'landing'|'dashboard'|'folder'|'palette'}){return <>
<CommandInput variant={variant==='palette'?'default':variant} placeholder="Search commands"/>
<CommandList ${list} variant={variant==='landing'?'landing':variant==='palette'?'palette':'default'}>
<CommandEmpty variant={variant==='folder'?'folder':'dashboard'}>No matches</CommandEmpty>
{(['default','compact','palette','landing',null]as const).map((group,index)=><CommandGroup key={String(group)} heading={'Group '+String(group)} variant={group}>
{(index===0?['default','landing','dashboard','dashboardIdentifier','palette','paletteDisabled','back','conflict','compact']:['default']).map(item=><CommandItem key={item} value={String(group)+' '+item} variant={item as any} disabled={item==='paletteDisabled'}><LuPlus ${icon}/>{String(group)} {item}<CommandShortcut variant={item==='conflict'?'warning':'brand'}>⌘K</CommandShortcut></CommandItem>)}
</CommandGroup>)}
<CommandSeparator alwaysRender/><CommandGroup heading="Caller heading" ${heading}><CommandItem value="last">Last action</CommandItem></CommandGroup>
</CommandList><CommandFooter>Choose a command</CommandFooter></>}
export function CommandSamples(){const mode=new URLSearchParams(location.search).get('command');return <main id="command-fixtures" style={{padding:16,display:'grid',gap:16,maxWidth:520}}>
{mode?.startsWith('dialog')?<CommandDialog open variant={mode==='dialog-palette'?'palette':'default'} showCloseButton={false}><Options/></CommandDialog>:(['default','landing','dashboard','folder','palette']as const).map(variant=><Command key={variant} variant={variant} ${caller} style={{'--command-caller':'retained'} as React.CSSProperties}><Options variant={variant}/></Command>)}
</main>}
`,
  )
}
