import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeRootsFixture compares image variables and both Forge link contexts. */
export async function writeRootsFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Relocate each real root without changing its bundled image or theme imports.
  for (const [name, file] of [
    ['AppLogo', 'web/images/AppLogo.tsx'],
    ['ForgeEntityLink', 'web/forge/ForgeEntityLink.tsx'],
  ] as const) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/roots-${name}.tsx`
        : file,
      'utf8',
    )
    await writeFile(
      join(root, name + '.tsx'),
      source.replace(
        './spacewave-icon.png',
        resolve('web/images/spacewave-icon.png'),
      ),
    )
  }

  // Only navigation is consumed here; the fixture never reads or starts a World resource.
  const prop = (before: string, after: string) =>
    version === 'before'
      ? `className=${JSON.stringify(before)}`
      : `sx={styles.${after}}`
  await writeFile(
    join(root, 'RootsSamples.tsx'),
    `
import React from'react';import * as stylex from'@stylexjs/stylex';
import{AppLogo}from'./AppLogo';import{ForgeEntityLink}from'./ForgeEntityLink';import{SpaceContainerContext}from'@s4wave/web/contexts/SpaceContainerContext.js';
const styles=stylex.create({small:{width:'28px',height:'28px'},header:{maxHeight:'var(--window-topbar-height)',userSelect:'none',WebkitUserSelect:'none'},label:{color:'var(--color-foreground)',minWidth:0,fontSize:'var(--text-sm)',lineHeight:'var(--text-sm--line-height)',fontWeight:'var(--font-weight-medium)','--tw-font-weight':'var(--font-weight-medium)'},plain:{color:'var(--color-foreground-alt)',padding:'8px'}});
export function RootsSamples(){return <main id="roots-fixtures" style={{padding:16,display:'grid',gap:16,width:350}}><AppLogo ${prop('size-7', 'small')}/><AppLogo topbarHeight={28} ${prop('max-h-(--window-topbar-height) select-none', 'header')}/><ForgeEntityLink objectKey="object:outside" label="Outside space"/><ForgeEntityLink objectKey="object:custom-outside" label="Custom outside" ${prop('text-foreground-alt p-2', 'plain')}/><SpaceContainerContext.Provider spaceId="parity" spaceState={{}} spaceWorldResource={undefined as never} spaceWorld={undefined as never} navigateToRoot={()=>{}} navigateToObjects={()=>{}} buildObjectUrls={()=>[]} navigateToSubPath={()=>{}}><ForgeEntityLink objectKey="object:default" label="Default link"/><ForgeEntityLink objectKey="object:custom" label="Custom link" ${prop('text-foreground min-w-0 text-sm font-medium', 'label')}/></SpaceContainerContext.Provider></main>}
`,
  )
}
