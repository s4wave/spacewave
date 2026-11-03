import { readFile, writeFile } from 'node:fs/promises'
import { dirname, join, posix, resolve } from 'node:path'

/** writeSystemFixture keeps the complete original tile and tone palette together. */
export async function writeSystemFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Original icons retain their class inputs; the native tile scopes rules at its SVG owners.
  for (const name of ['SystemTile.tsx', 'tone.ts']) {
    const file = 'app/system/' + name
    const source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/system-' + name
        : file,
      'utf8',
    )
    await writeFile(
      join(root, name),
      source.replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          ref === './tone.js'
            ? `${prefix}./tone${suffix}`
            : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ),
    )
  }

  // An explicit caller size verifies that the old direct-child icon requirement still wins.
  await writeFile(
    join(root, 'SystemSamples.tsx'),
    `
import React from'react';import * as stylex from'@stylexjs/stylex';import{SystemTile}from'./SystemTile';
import{LuGauge}from${JSON.stringify(version === 'before' ? 'react-icons/lu' : resolve('web/images/icons.ts'))};
const styles=stylex.create({icon:{width:24,height:24}});
export function SystemSamples(){return <main id="system-fixtures" style={{padding:24,display:'grid',gridTemplateColumns:'repeat(auto-fit,minmax(260px,1fr))',gap:16}}>{(['nominal','active','warning','error','pending']as const).map(tone=><SystemTile key={tone} title={'System '+tone} icon={<LuGauge ${version === 'before' ? 'className="size-6"' : 'sx={styles.icon}'}/>} tone={tone} headline="12 resources" onOpen={()=>{}}>Available resources</SystemTile>)}</main>}
`,
  )
}
