import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeTreeFixture pairs the actual virtualized container and its row implementation. */
export async function writeTreeFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep row changes on the paired tree while using the unchanged production state and node contracts.
  for (const file of ['Tree', 'TreeRow']) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/${file}.tsx`
        : `web/ui/tree/${file}.tsx`,
      'utf8',
    )
    await writeFile(
      join(root, file + '.tsx'),
      source
        .replace(
          /(['"])(?:\.\.\/)+(?:web\/)?style\/([\w-]+)\.stylex\.js\1/g,
          (_match, _quote, module) =>
            JSON.stringify(resolve(`web/style/${module}.stylex.ts`)),
        )
        .replace(
          /(['"])\.\/(TreeNode|TreeState)\.js\1/g,
          (_match, _quote, module) =>
            JSON.stringify(resolve(`web/ui/tree/${module}.tsx`)),
        ),
    )
  }
}
