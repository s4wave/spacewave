import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeTraceFixture prepares the actual disclosure trigger with its historical indicator. */
export async function writeTraceFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Relocate only theme modules; the unchanged collapsible primitive keeps its production path.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/trace.tsx'
      : 'web/ui/trace.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'trace.tsx'),
    source.replace(
      /(['"])(?:\.\.\/)+(?:web\/)?style\/([\w-]+)\.stylex\.js\1/g,
      (_match, _quote, module) =>
        JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
    ),
  )

  // Closed, open, error and caller-override states share the same real controls.
  await writeFile(
    join(root, 'TraceSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import{Trace,TraceLine,TraceTitle,TraceCollapsible,TraceCollapsibleTrigger,TraceCollapsibleContent}from'./trace';
const styles=stylex.create({override:{padding:12}});
export function TraceSamples(){return <section>{[false,true].flatMap(open=>['done','error'].map(status=><Trace key={status+open}><TraceCollapsible open={open} onOpenChange={()=>{}}><TraceCollapsibleTrigger ${version === 'before' ? 'className="p-3"' : 'sx={styles.override}'}><TraceLine status={status as 'done'}><TraceTitle>{status} trace {String(open)}</TraceTitle></TraceLine></TraceCollapsibleTrigger><TraceCollapsibleContent>Nested trace detail</TraceCollapsibleContent></TraceCollapsible></Trace>))}</section>}
`,
  )
}
