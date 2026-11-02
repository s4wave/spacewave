import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'

/** writeGroupFixture exercises joined native controls and the public group recipe. */
export async function writeGroupFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Actual Button and Input roots retain their own variant styles inside both group axes.
  await writeFile(
    join(root, 'GroupSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import{ButtonGroup,ButtonGroupText,ButtonGroupSeparator,buttonGroupVariants}from'./button-group';
import{Button}from'./button';
import{Input}from'./input';
import{LuPlus}from'@s4wave/web/images/icons.js';
export function GroupSamples(){return <main id="group-fixtures" style={{padding:16,display:'grid',alignContent:'start',gap:16}}>
{(['horizontal','vertical',null]as const).map((orientation,index)=><ButtonGroup orientation={orientation} key={index}><ButtonGroupText><LuPlus/>Label</ButtonGroupText><Input aria-label={'Group field '+index} defaultValue="Example"/><Button variant="outline">Apply {index}</Button><Button variant="outline">More {index}</Button></ButtonGroup>)}
<ButtonGroup><ButtonGroup><Button variant="outline">First</Button><Button variant="outline">Second</Button></ButtonGroup><ButtonGroup><Button variant="outline">Third</Button><Button variant="outline">Fourth</Button></ButtonGroup></ButtonGroup>
<ButtonGroup><ButtonGroupText asChild><span>Units</span></ButtonGroupText><ButtonGroupSeparator/><Button variant="outline">Reset</Button></ButtonGroup>
<div role="group" ${version === 'before' ? 'className={buttonGroupVariants({orientation:"vertical"})}' : '{...stylex.props(buttonGroupVariants({orientation:"vertical"}))}'}><Button variant="outline">Public first</Button><Button variant="outline">Public last</Button></div>
<Button variant="outline">Outside group</Button>
</main>}
`,
  )
}
