import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeBadgeFixture pairs semantic appearances, invalid states and direct-child SVG rules. */
export async function writeBadgeFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the historical recipe and the actual native implementation in separate builds.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/badge.tsx'
      : 'web/ui/badge.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'badge.tsx'),
    source.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/([\w-]+)\.stylex\.js\1/g,
      (_match, _quote, module) =>
        JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
    ),
  )

  // Every variant has span, link and invalid-link outcomes, plus caller and nested-icon controls.
  await writeFile(
    join(root, 'BadgeSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import {Badge,badgeVariants} from './badge';
import {IconContext,LuPlus} from '@s4wave/web/images/icons.js';
const styles=stylex.create({override:{paddingInline:12},large:{width:28,height:28}});
const variants=[undefined,null,'default','secondary','destructive','outline','subtle','conflict','brand','success','muted','release'] as const;
export function BadgeSamples(){return <main id="badge-fixtures"><IconContext.Provider value={{color:'rgb(17,34,51)'}}>
{variants.map((variant,index)=><section key={index} style={{display:'flex',gap:8,marginBottom:8}}>
<Badge variant={variant}><LuPlus/>Status {index}</Badge>
<Badge variant={variant} asChild><a href="#badge-target"><LuPlus/>Link {index}</a></Badge>
<Badge variant={variant} asChild><a href="#badge-target" aria-invalid="true"><LuPlus/>Invalid {index}</a></Badge>
</section>)}
<Badge ${version === 'before' ? 'className="px-3"' : 'sx={styles.override}'}>Caller padding</Badge>
<Badge><LuPlus ${version === 'before' ? 'className="size-7"' : '{...stylex.props(styles.large)}'}/>Direct icon</Badge>
<Badge><span><LuPlus ${version === 'before' ? 'className="size-7"' : '{...stylex.props(styles.large)}'}/></span>Nested icon</Badge>
<span ${version === 'before' ? 'className={badgeVariants({variant:"outline"})}' : 'sx={badgeVariants({variant:"outline"})}'}>Public recipe</span>
<a href="#badge-target" aria-label="Focus badge">Outside link</a>
</IconContext.Provider></main>}
`,
  )
}
