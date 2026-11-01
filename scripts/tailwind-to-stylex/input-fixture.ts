import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeInputFixture prepares every native field variant and its disabled and invalid states. */
export async function writeInputFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the original input beside its native port, with both using the actual theme.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/input.tsx'
      : 'web/ui/input.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'input.tsx'),
    source.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
      JSON.stringify(resolve('web/style/tokens.stylex.ts')),
    ),
  )

  // A focused invalid field and a file input exercise the real browser pseudo-elements too.
  const override =
    version === 'before' ? 'className="flex-1"' : 'sx={styles.grow}'
  await writeFile(
    join(root, 'InputSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import {Input} from './input';
const styles=stylex.create({grow:{flex:1}});
const variants=[undefined,null,'config','configCompact','search','searchPanel','command','displayName','plugin','toolbar','configMono','wizard','wizardError','wizardMono'] as const;
export function InputSamples(){return <main id="input-fixtures">
 {variants.map((variant,index)=><section key={index} style={{display:'flex',gap:8,marginBottom:8}}>
  <Input aria-label={'Normal '+index} variant={variant} placeholder="Field value" defaultValue="Field value" ${override}/>
  <Input aria-label={'Disabled '+index} variant={variant} placeholder="Disabled field" disabled ${override}/>
  <Input aria-label={'Invalid '+index} variant={variant} placeholder="Invalid field" aria-invalid ${override}/>
 </section>)}
 <Input aria-label="Choose file" type="file"/>
 <Input aria-label="Focus reference" variant="config" aria-invalid defaultValue="Select this text"/>
</main>}
`,
  )
}
