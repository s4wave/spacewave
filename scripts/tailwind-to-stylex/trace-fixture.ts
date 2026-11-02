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

  // Stack frames exercise the real source-link owner inside the trace list.
  const stack = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/stack-trace.tsx'
      : 'web/ui/stack-trace.tsx',
    'utf8',
  )
  await writeFile(
    join(root, 'stack-trace.tsx'),
    stack.replace('@s4wave/web/ui/trace.js', './trace'),
  )

  // Closed, open, error and caller-override states share the same real controls.
  await writeFile(
    join(root, 'TraceSamples.tsx'),
    `
import * as stylex from '@stylexjs/stylex';
import{StackTrace}from'./stack-trace';
import{Trace,TraceLine,TraceTitle,TraceCollapsible,TraceCollapsibleTrigger,TraceCollapsibleContent}from'./trace';
const styles=stylex.create({override:{padding:12}});
export function TraceSamples(){return <section id="trace-fixtures">{[false,true].flatMap(open=>['done','error','running','pending'].map(status=><Trace key={status+open}><TraceCollapsible open={open} onOpenChange={()=>{}}><TraceCollapsibleTrigger ${version === 'before' ? 'className="p-3"' : 'sx={styles.override}'}><TraceLine status={status as 'done'}><TraceTitle>{status} trace {String(open)}</TraceTitle></TraceLine></TraceCollapsibleTrigger><TraceCollapsibleContent>Nested trace detail</TraceCollapsibleContent></TraceCollapsible></Trace>))}<StackTrace error="Call failed" frames={[{text:"file.ts:42",href:"#source"},{text:"external frame"}]}/></section>}
`,
  )
}
