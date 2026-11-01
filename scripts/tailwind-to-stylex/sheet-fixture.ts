import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeSheetFixture prepares isolated inline and modal views of the real sheet. */
export async function writeSheetFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the original component beside the native port without changing either implementation.
  const component = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/sheet.tsx'
      : 'web/ui/sheet.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'sheet.tsx'),
    component.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
      JSON.stringify(resolve('web/style/tokens.stylex.ts')),
    ),
  )

  // The two documentation callers preserve the sheet's existing breakpoint limit.
  const contentStyle =
    version === 'before'
      ? 'className={width === "sidebar" ? "w-70 max-w-(--max-width-docs-sidebar)" : width === "content" ? "w-60 max-w-(--max-width-content)" : undefined}'
      : 'sx={width === "sidebar" ? styles.sidebar : width === "content" ? styles.content : undefined}'
  const actionStyle =
    version === 'before' ? 'className="p-2"' : 'sx={styles.action}'

  // Inline portals cover every side; the separate modal view includes the real overlay.
  await writeFile(
    join(root, 'SheetSamples.tsx'),
    `
import React,{useState} from 'react';
import * as stylex from '@stylexjs/stylex';
import{Sheet,SheetTrigger,SheetClose,SheetContent,SheetHeader,SheetFooter,SheetTitle,SheetDescription}from'./sheet';
const styles=stylex.create({
 sidebar:{width:'calc(var(--spacing) * 70)',maxWidth:{default:'var(--max-width-docs-sidebar)','@media (min-width: 40rem)':'var(--container-sm)'}},
 content:{width:'calc(var(--spacing) * 60)',maxWidth:{default:'var(--max-width-content)','@media (min-width: 40rem)':'var(--container-sm)'}},
 action:{padding:'calc(var(--spacing) * 2)'},
});
function InlineSheet({side,variant,width}:{side:'right'|'left'|'top'|'bottom';variant:'default'|'docs'|'docsPlain';width?:'sidebar'|'content'}){
 const[container,setContainer]=useState<HTMLElement|null>(null);
 return <section ref={setContainer} style={{position:'relative',height:220,marginBottom:8,isolation:'isolate'}}>
  {container&&<Sheet open modal={false}>
   <SheetTrigger ${actionStyle}>Inline trigger</SheetTrigger>
   <SheetContent side={side} variant={variant} showCloseButton={!width} portalContainer={container} position="absolute" ${contentStyle} onOpenAutoFocus={event=>event.preventDefault()} onCloseAutoFocus={event=>event.preventDefault()} onInteractOutside={event=>event.preventDefault()}>
    <SheetHeader><SheetTitle variant={variant}>{side} {variant} preview</SheetTitle><SheetDescription>Sheet information</SheetDescription></SheetHeader>
    <p>Sheet content</p>
    <SheetFooter><SheetClose ${actionStyle}>Done</SheetClose></SheetFooter>
   </SheetContent>
  </Sheet>}
 </section>
}
export function SheetSamples(){
 const[open,setOpen]=useState(true);
 const[container,setContainer]=useState<HTMLElement|null>(null);
 const mode=new URLSearchParams(location.search).get('sheet');
 const absolute=mode==='modal-absolute';
 if(mode?.startsWith('modal'))return <main id="sheet-fixtures" ref={setContainer} style={{position:'relative',height:absolute?300:undefined}}>
  {(!absolute||container)&&<Sheet open={open} onOpenChange={setOpen}>
   <SheetTrigger ${actionStyle}>Open sheet</SheetTrigger>
   <SheetContent portalContainer={absolute?container:undefined} position={absolute?'absolute':'fixed'} onOpenAutoFocus={event=>event.preventDefault()}>
    <SheetHeader><SheetTitle>Modal sheet</SheetTitle><SheetDescription>Review the sheet</SheetDescription></SheetHeader>
    <p>Modal content</p><SheetFooter><SheetClose ${actionStyle}>Done</SheetClose></SheetFooter>
   </SheetContent>
  </Sheet>}
 </main>;
 return <main id="sheet-fixtures">
  <InlineSheet side="right" variant="default"/>
  <InlineSheet side="left" variant="default"/>
  <InlineSheet side="top" variant="docs"/>
  <InlineSheet side="bottom" variant="docsPlain"/>
  <InlineSheet side="left" variant="docs" width="sidebar"/>
  <InlineSheet side="left" variant="docs" width="content"/>
 </main>;
}
`,
  )
}
