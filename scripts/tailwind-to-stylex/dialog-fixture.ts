import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeDialogFixture prepares the real modal variants and their command callers. */
export async function writeDialogFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Copy each implementation with its actual dependencies and an isolated dialog import.
  for (const name of ['dialog', 'command']) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
        : `web/ui/${name}.tsx`,
      'utf8',
    )
    await writeFile(
      join(root, `${name}.tsx`),
      source
        .replace(
          /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
          JSON.stringify(resolve('web/style/tokens.stylex.ts')),
        )
        .replace(/@s4wave\/web\/ui\/dialog(?:\.js)?/g, './dialog'),
    )
  }

  // Preserve both the phone cap and the caller's large-screen width in each override.
  const overrides =
    version === 'before'
      ? `className={mode==='caller-md'?'max-w-md':mode==='caller-sm'?'overflow-hidden sm:max-w-sm':undefined}`
      : `sx={mode==='caller-md'?styles.medium:mode==='caller-sm'?styles.small:undefined}`
  const action = version === 'before' ? 'className="p-2"' : 'sx={styles.action}'
  await writeFile(
    join(root, 'DialogSamples.tsx'),
    `
import React from 'react';
import * as stylex from '@stylexjs/stylex';
import{Dialog,DialogTrigger,DialogClose,DialogContent,DialogHeader,DialogFooter,DialogTitle,DialogDescription}from'./dialog';
import{CommandDialog,CommandInput,CommandList,CommandItem}from'./command';
const styles=stylex.create({
 medium:{maxWidth:{default:'var(--container-md)','@media (min-width: 40rem)':'var(--container-lg)'}},
 small:{overflow:'hidden',maxWidth:{default:'calc(100% - 2rem)','@media (min-width: 40rem)':'var(--container-sm)'}},
 action:{padding:'calc(var(--spacing) * 2)'},
});
export function DialogSamples(){
 const mode=new URLSearchParams(location.search).get('dialog')!;
 if(mode.startsWith('command'))return <CommandDialog open variant={mode==='command-palette'?'palette':'default'} title="Dialog commands">
  <CommandInput placeholder="Find a command"/>
  <CommandList><CommandItem>First command</CommandItem><CommandItem>Second command</CommandItem></CommandList>
 </CommandDialog>;
 const variant=mode==='compact'||mode==='editor'||mode==='panel'?mode:'default';
 const header=variant==='editor'||variant==='panel'?variant:'default';
 return <main id="dialog-fixtures"><Dialog open>
  <DialogTrigger ${action}>Open dialog</DialogTrigger>
  <DialogContent variant={variant} ${overrides} showCloseButton={mode!=='caller-sm'} onOpenAutoFocus={event=>event.preventDefault()}>
   <DialogHeader variant={header}>
    <DialogTitle variant={mode==='compact'?'withIcon':mode==='editor'?'editor':'default'}>Dialog preview</DialogTitle>
    <DialogDescription variant={mode==='compact'?'relaxed':mode==='editor'?'editor':'default'}>Review the dialog information.</DialogDescription>
   </DialogHeader>
   <p>Dialog content</p><DialogFooter><DialogClose ${action}>Done</DialogClose></DialogFooter>
  </DialogContent>
 </Dialog></main>;
}
`,
  )
}
