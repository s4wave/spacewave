import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeIconDefaultsFixture pairs the actual controls that give descendant icons overridable defaults. */
export async function writeIconDefaultsFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the original class-based scopes beside the actual SVG-owner composition.
  for (const name of ['DropdownMenu', 'Menubar', 'code-block']) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`
        : `web/ui/${name}.tsx`,
      'utf8',
    )
    await writeFile(
      join(root, `${name}-icons.tsx`),
      source.replace(/(['"])(\.\.?\/[^'"]+)\1/g, (_match, _quote, specifier) =>
        JSON.stringify(resolve('web/ui', specifier)),
      ),
    )
  }

  // Explicit caller size and color must survive each control's defaults and destructive state.
  const caller =
    version === 'before'
      ? 'className="size-7 text-[#112233] group-data-[selected=true]:opacity-50"'
      : 'sx={styles.caller}'
  const checkboxWidth =
    version === 'before' ? 'className="w-[180px]"' : 'sx={styles.width(180)}'
  const submenuSize =
    version === 'before'
      ? 'className="pointer-coarse:min-h-11"'
      : 'sx={styles.coarseSubmenu}'
  await writeFile(
    join(root, 'IconDefaultsSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import {groupMarker} from '@s4wave/web/style/relations.stylex.js';
import{LuPlus}from'@s4wave/web/images/icons.js';
import{DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem,DropdownMenuCheckboxItem,DropdownMenuRadioGroup,DropdownMenuRadioItem,DropdownMenuSub,DropdownMenuSubTrigger}from'./DropdownMenu-icons';
import{Menubar,MenubarMenu,MenubarTrigger,MenubarContent,MenubarItem,MenubarCheckboxItem,MenubarRadioGroup,MenubarRadioItem,MenubarSub,MenubarSubTrigger}from'./Menubar-icons';
import{Command,CommandList,CommandItem}from'./command';
import{CodeBlockTitle}from'./code-block-icons';
import{ButtonGroupText}from'./button-group';
import{TraceIcon}from'./trace';
const styles=stylex.create({width:(width:number)=>({width}),coarseSubmenu:{minHeight:{default:null,'@media (pointer: coarse)':44}},caller:{width:28,height:28,color:'rgb(17,34,51)',opacity:{default:null,[stylex.when.ancestor(':is([data-selected="true"])',groupMarker)]:.5}}});
function InlineSamples(){return <>
<Command value="caller"><CommandList><CommandItem variant="dashboard" value="default"><LuPlus/>Default command</CommandItem><CommandItem variant="dashboard" value="caller"><LuPlus ${caller}/>Caller command</CommandItem></CommandList></Command>
<CodeBlockTitle><LuPlus/>Default code title</CodeBlockTitle><CodeBlockTitle><LuPlus ${caller}/>Caller code title</CodeBlockTitle>
<ButtonGroupText><LuPlus/>Default group</ButtonGroupText><ButtonGroupText><LuPlus ${caller}/>Caller group</ButtonGroupText>
<TraceIcon><LuPlus/></TraceIcon><TraceIcon><LuPlus ${caller}/></TraceIcon>
</>}
function DropdownSamples(){return <DropdownMenu open modal={false}><DropdownMenuTrigger>Icon dropdown</DropdownMenuTrigger><DropdownMenuContent>
<DropdownMenuItem><LuPlus/>Default item</DropdownMenuItem><DropdownMenuItem><LuPlus ${caller}/>Caller item</DropdownMenuItem><DropdownMenuItem variant="destructive"><LuPlus ${caller}/>Destructive item</DropdownMenuItem><DropdownMenuItem inset><LuPlus/>Inset item</DropdownMenuItem><DropdownMenuItem variant="selected"><LuPlus/>Selected item</DropdownMenuItem><DropdownMenuItem disabled><LuPlus/>Disabled item</DropdownMenuItem>
<DropdownMenuCheckboxItem checked ${checkboxWidth} style={{opacity:.9,'--checkbox-caller':'retained'}}><LuPlus ${caller}/>Checked item</DropdownMenuCheckboxItem>
<DropdownMenuRadioGroup value="chosen"><DropdownMenuRadioItem value="chosen"><LuPlus ${caller}/>Radio item</DropdownMenuRadioItem></DropdownMenuRadioGroup>
<DropdownMenuSub><DropdownMenuSubTrigger ${submenuSize}><LuPlus ${caller}/>Submenu item</DropdownMenuSubTrigger></DropdownMenuSub>
</DropdownMenuContent></DropdownMenu>}
function MenubarSamples(){return <Menubar value="icons"><MenubarMenu value="icons"><MenubarTrigger>Icon menubar</MenubarTrigger><MenubarContent>
<MenubarItem><LuPlus/>Default item</MenubarItem><MenubarItem><LuPlus ${caller}/>Caller item</MenubarItem><MenubarItem variant="destructive"><LuPlus ${caller}/>Destructive item</MenubarItem><MenubarItem inset><LuPlus/>Inset item</MenubarItem><MenubarItem variant="dimmed"><LuPlus/>Dimmed item</MenubarItem><MenubarItem disabled><LuPlus/>Disabled item</MenubarItem>
<MenubarCheckboxItem checked ${checkboxWidth} style={{opacity:.9,'--checkbox-caller':'retained'}}><LuPlus ${caller}/>Checked item</MenubarCheckboxItem>
<MenubarRadioGroup value="chosen"><MenubarRadioItem value="chosen"><LuPlus ${caller}/>Radio item</MenubarRadioItem></MenubarRadioGroup>
<MenubarSub><MenubarSubTrigger ${submenuSize}><LuPlus ${caller}/>Submenu item</MenubarSubTrigger></MenubarSub>
</MenubarContent></MenubarMenu></Menubar>}
export function IconDefaultsSamples(){const mode=new URLSearchParams(location.search).get('icon-defaults');return <main id="icon-defaults-fixture">{mode==='dropdown'?<DropdownSamples/>:mode==='menubar'?<MenubarSamples/>:<InlineSamples/>}</main>}
`,
  )
}
