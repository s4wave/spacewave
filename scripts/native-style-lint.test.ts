import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { promisify } from 'node:util'
import { test } from 'node:test'

const execute = promisify(execFile)

// This check exercises the installed lint runner and its actual plugin interfaces.
test('native style lint accepts compiled skins and rejects utility producers', async () => {
  // Keep the probe outside ignored build folders and use the production plugins.
  const root = await mkdtemp(join(tmpdir(), 'spacewave-native-lint-'))
  const config = join(root, 'config.json')
  const source = join(root, 'classes.tsx')
  await writeFile(
    config,
    JSON.stringify({
      categories: { correctness: 'off' },
      jsPlugins: [
        {
          name: 'stylex',
          specifier: resolve(
            'node_modules/@stylexjs/eslint-plugin/lib/index.js',
          ),
        },
        {
          name: 'native-style',
          specifier: resolve('scripts/native-style-lint.mjs'),
        },
      ],
      rules: {
        'stylex/valid-styles': 'error',
        'native-style/native-classes': 'error',
      },
    }),
  )
  try {
    // Compiler-supported vendor values, conditional keys and native CSS variables stay valid.
    await writeFile(
      source,
      `import * as stylex from '@stylexjs/stylex';
const marker=stylex.defineMarker();
const styles=stylex.create({root:{flex:1, WebkitUserSelect:'none', WebkitFontSmoothing:'subpixel-antialiased', MozOsxFontSmoothing:'auto', zIndex:'var(--layer)', dynamicRangeLimit:'var(--hdr)', color:{default:'red', ':not(:first-child)':{default:null,[stylex.when.ancestor(':hover',marker)]:{default:null,':focus':'blue'}}}}});
const valid=<div className={stylex.props(styles.root).className}/>;
const skin=<div className="swb-bar"/>;
void valid;void skin;`,
    )
    await execute(resolve('node_modules/.bin/oxlint'), ['-c', config, source], {
      timeout: 30000,
    })

    // Catch direct, forwarded and imperative classes through one rule without accepting unknown properties.
    await writeFile(
      source,
      `import * as stylex from '@stylexjs/stylex';
import type {EditorThemeClasses} from 'lexical';
const theme:EditorThemeClasses={text:{bold:'font-bold'}};
const classes='p-2';const direct=<div className="flex font-mono"/>;const forwarded={className:classes};
document.body.className='rounded-md';const bad=stylex.create({root:{colro:'red'}});`,
    )
    const result = await execute(
      resolve('node_modules/.bin/oxlint'),
      ['-c', config, source],
      { timeout: 30000 },
    ).then(
      () => {
        throw new Error('Utility styles unexpectedly passed lint')
      },
      (error: { stdout: string; stderr: string; code: number }) => error,
    )
    const output = result.stdout + result.stderr
    assert.match(output, /native-classes/)
    for (const value of ['p-2', 'flex', 'font-mono', 'rounded-md', 'font-bold'])
      assert(output.includes(value), value)
    assert.match(output, /valid-styles/)
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
