import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeButtonFixture prepares the actual action primitives and their recipe choices. */
export async function writeButtonFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep wrapper imports on the paired button rather than mixing old and new contracts.
  for (const file of ['DashboardButton', 'DropdownTriggerButton']) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/${file}.tsx`
        : `web/ui/${file}.tsx`,
      'utf8',
    )
    await writeFile(
      join(root, file + '.tsx'),
      source
        .replace(
          /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
          JSON.stringify(resolve('web/style/tokens.stylex.ts')),
        )
        .replace(
          /(['"])(?:@s4wave\/web\/ui\/|\.\/)?button\.js\1/g,
          "'./button'",
        ),
    )
  }

  // One row per variant and size covers both independent axes without a redundant Cartesian grid.
  const override =
    version === 'before' ? 'className="w-full"' : 'sx={styles.full}'
  await writeFile(
    join(root, 'ButtonSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import {Button,buttonVariants} from './button';
import {DashboardButton} from './DashboardButton';
import {DropdownTriggerButton} from './DropdownTriggerButton';
import {IconContext,LuPlus} from '@s4wave/web/images/icons.js';
import {Spinner} from './Spinner';
const styles=stylex.create({full:{width:'100%'},largeIcon:{width:32,height:32}});
const variants=['default','destructive','outline','secondary','ghost','link','brandOutline','quiet','destructiveOutline','dangerGhost','brandGhost','muted','brandSoft','selectedBrand','capture','unselectedRow','selectedRow','debugOutline','debugGhost','destructiveSubtle','errorGhost','selectedFinder','unselectedFinder','category'] as const;
const sizes=['default','sm','lg','icon','xs','toolbar','toolbarSm','toolbarWide','brandWide','toolbarText','compact8','iconSm','iconXs','row','finderRow','commandRow','category'] as const;
export function ButtonSamples(){return <main id="button-fixtures"><IconContext.Provider value={{color:'rgb(17, 34, 51)'}}>
 {variants.map(variant=><section key={variant} style={{display:'flex',gap:8,marginBottom:8}}>
  <Button variant={variant}><LuPlus/>Action {variant}</Button>
  <Button variant={variant} disabled><LuPlus/>Disabled {variant}</Button>
 </section>)}
 {sizes.map(size=><section key={size} style={{display:'flex',gap:8,marginBottom:8}}><Button size={size}><LuPlus/>Size {size}</Button></section>)}
 <Button variant={null} size={null}>No recipe defaults</Button>
 <Button variant="capture" data-capturing="true"><LuPlus/>Capturing</Button>
 <Button variant="outline" ${override}><LuPlus/>Caller width</Button>
 <Button asChild><a href="#button-target"><LuPlus/>Child link</a></Button>
 <Button><Spinner size="sm"/>Busy action</Button>
 <Button>${version === 'before' ? '<LuPlus className="size-8"/>' : '<LuPlus {...stylex.props(styles.largeIcon)}/>'}Large icon action</Button>
 {['default','active','destructive','primary'].map(variant=><DashboardButton key={variant} variant={variant as 'default'} icon={<LuPlus/>}>Dashboard {variant}</DashboardButton>)}
 <DropdownTriggerButton icon={<LuPlus/>}>Outline dropdown</DropdownTriggerButton>
 <DropdownTriggerButton triggerStyle="ghost" icon={<LuPlus/>}>Ghost dropdown</DropdownTriggerButton>
 <button type="button" ${version === 'before' ? "className={buttonVariants({variant:'outline',size:'sm'})}" : "sx={buttonVariants({variant:'outline',size:'sm'})}"}>Recipe action</button>
 <Button aria-label="Focus button">Focus action</Button>
</IconContext.Provider></main>}
`,
  )
}
