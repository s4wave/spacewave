import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'

/** writeCodeFixture pairs code headers, lines, content and both action placements. */
export async function writeCodeFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Retain the original compound selectors beside the actual native owners.
  await writeFile(
    join(root, 'code-block.tsx'),
    await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/code-block.tsx'
        : 'web/ui/code-block.tsx',
      'utf8',
    ).then((source) => source.replace('@s4wave/web/ui/button.js', './button')),
  )

  // Header actions remain inline while root actions overlay the scrollable content.
  await writeFile(
    join(root, 'CodeSamples.tsx'),
    `
import{CodeBlock,CodeBlockHeader,CodeBlockTitle,CodeBlockActions,CodeBlockCopyButton,CodeBlockContent,CodeBlockLine,CodeBlockFooter}from'./code-block';
import{LuPlus}from'@s4wave/web/images/icons.js';
export function CodeSamples(){return <main id="code-fixtures" style={{padding:16,maxWidth:520,display:'grid',gap:12}}>
{(['default','sm']as const).flatMap(size=>(['default','destructive']as const).map(variant=><CodeBlock key={size+variant} size={size} variant={variant}>
<CodeBlockHeader><CodeBlockTitle><LuPlus/>{size} {variant}</CodeBlockTitle><CodeBlockActions><CodeBlockCopyButton value="example"/></CodeBlockActions></CodeBlockHeader>
<CodeBlockContent lineNumbers startLine={3} wrap style={{opacity:.9,'--caller-code':'retained'}}><CodeBlockLine>const value = 1</CodeBlockLine><CodeBlockLine variant="added">added line</CodeBlockLine><CodeBlockLine variant="removed">removed line</CodeBlockLine><CodeBlockLine variant="highlighted">highlighted line</CodeBlockLine><CodeBlockLine/></CodeBlockContent>
<CodeBlockFooter>Five lines</CodeBlockFooter><CodeBlockActions><CodeBlockCopyButton value="example"/></CodeBlockActions></CodeBlock>))}
<CodeBlock size="sm"><CodeBlockContent lineNumbers startLine={7}>{'first\\n\\nthird\\n'}</CodeBlockContent><CodeBlockActions><CodeBlockCopyButton value="first"/></CodeBlockActions></CodeBlock>
</main>}
`,
  )
}
