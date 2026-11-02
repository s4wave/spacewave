import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeTabsFixture prepares the actual tab recipes and their form panel layouts. */
export async function writeTabsFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the historical implementation and native port on the same real Radix dependency.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/tabs.tsx'
      : 'web/ui/tabs.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'tabs.tsx'),
    source.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/tokens\.stylex\.js\1/g,
      JSON.stringify(resolve('web/style/tokens.stylex.ts')),
    ),
  )

  // Several ordinary children expose the form gap previously supplied by space utilities.
  const override =
    version === 'before' ? 'className="px-3"' : 'sx={styles.caller}'
  await writeFile(
    join(root, 'TabsSamples.tsx'),
    `
import React from 'react';
import{cn}from'cnfast';
import * as stylex from '@stylexjs/stylex';
import{Tabs,TabsList,TabsTrigger,TabsContent${version === 'after' ? ',tabsContentItemStyles' : ''}}from'./tabs';
const itemClass=${version === 'after' ? 'stylex.props(tabsContentItemStyles.item).className' : 'undefined'};
const styles=stylex.create({caller:{paddingInline:'calc(var(--spacing) * 3)'}});
export function TabsSamples(){return <main id="tabs-fixtures">
 {[undefined,'form','formTight',undefined].map((content,index)=><section key={index} style={{marginBottom:16}}>
  <Tabs defaultValue="first" variant={index===3?'debug':undefined}>
   <TabsList variant={index===3?'debug':undefined}>
    <TabsTrigger value="first" variant={index===3?'debug':undefined} ${override}>First {index}</TabsTrigger>
    <TabsTrigger value="second" variant={index===3?'debug':undefined}>Second {index}</TabsTrigger>
    <TabsTrigger value="disabled" variant={index===3?'debug':undefined} disabled>Disabled {index}</TabsTrigger>
   </TabsList>
   {['first','second'].map(value=><TabsContent key={value} value={value} variant={content as 'form'|'formTight'}>
    <p className={cn(itemClass,"text-foreground-alt/60 text-xs")}>Panel {value} {index}</p><div className={cn(itemClass,"relative")} style={{border:'1px solid',padding:12}}>Panel details</div><button type="button" className={cn(itemClass,"text-xs")}>Panel action</button>
   </TabsContent>)}
  </Tabs>
 </section>)}
</main>}
`,
  )
}
