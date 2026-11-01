import assert from 'node:assert/strict'
import { once } from 'node:events'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { test } from 'node:test'
import { transformSync } from '@babel/core'
import stylexPlugin from '@stylexjs/babel-plugin'
import * as stylex from '@stylexjs/stylex'
import { chromium, webkit } from 'playwright'

// compile evaluates only this test's compiler output to use the real runtime merge.
function compile(id: string, declaration: string) {
  const result = transformSync(
    `import * as stylex from '@stylexjs/stylex'; export const styles = stylex.create(${declaration});`,
    {
      filename: `${process.cwd()}/${id}.js`,
      plugins: [[stylexPlugin, { dev: false, runtimeInjection: false }]],
    },
  )!
  const code = result
    .code!.replace(/^import .*;$/gm, '')
    .replace('export const styles', 'const styles')
  return {
    styles: new Function(`${code};return styles`)() as Record<
      string,
      stylex.StyleXStyles
    >,
    rules: (
      result.metadata as unknown as {
        stylex: Parameters<typeof stylexPlugin.processStylexRules>[0]
      }
    ).stylex,
  }
}

test('separate sheets preserve compiler priority through WebView loading and inline replacement', async () => {
  // Compose shorthand and logical-property conflicts through StyleX's actual runtime.
  const shared = compile('shared', '{base:{marginTop:17,marginLeft:19}}')
  const plugin = compile('plugin', '{override:{margin:0,marginInlineStart:0}}')
  const props = stylex.props(shared.styles.base, plugin.styles.override)
  const css = (rules: typeof shared.rules) =>
    stylexPlugin.processStylexRules(rules, {
      useLayers: { before: ['reset'], prefix: 'stylex' },
    })
  const sheets = {
    shared: css(shared.rules),
    plugin: css(plugin.rules),
    combined: css([...shared.rules, ...plugin.rules]),
  }
  assert.match(sheets.shared, /stylex\.priority4000/)

  // Serve the independently compiled stylesheets through real link loads.
  const client = await readFile(
    new URL('./stylex-order.browser.js', import.meta.url),
    'utf8',
  )
  const server = createServer((request, response) => {
    const name = request.url?.slice(1) as keyof typeof sheets
    response.setHeader(
      'Content-Type',
      name in sheets ? 'text/css' : 'text/html',
    )
    response.end(
      name in sheets
        ? sheets[name]
        : `<!doctype html><html><head></head><body><button class="${props.className}">Probe</button></body></html>`,
    )
  })

  // Wait for the server's assigned local origin before starting either browser.
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert(address && typeof address !== 'string')
  const base = `http://127.0.0.1:${address.port}`

  // Exercise each arrival order, cached initialization, duplicate owners and Vite replacement.
  try {
    for (const [engine, launcher] of Object.entries({ chromium, webkit })) {
      const browser = await launcher.launch()
      try {
        for (const order of [
          ['shared', 'plugin'],
          ['plugin', 'shared'],
          ['combined'],
        ] as const) {
          const page = await browser.newPage()
          await page.goto(base)
          await page.evaluate(client)
          for (const name of order)
            await page.evaluate(
              (href) => globalThis.mountStylexSheet(href),
              `${base}/${name}`,
            )
          const margins = await page
            .locator('button')
            .evaluate((button) => [
              getComputedStyle(button).marginTop,
              getComputedStyle(button).marginLeft,
            ])
          assert.deepEqual(margins, ['17px', '19px'], `${engine}: ${order}`)
          await page.evaluate(() => {
            globalThis.activateStylexOrder()
            globalThis.activateStylexOrder()
          })
          assert.equal(
            await page.locator('style[data-bldr-stylex-order]').count(),
            1,
          )
          await page.close()
        }
        const page = await browser.newPage()
        await page.goto(base)
        await page.addStyleTag({ content: sheets.plugin })
        const sharedStyle = await page.addStyleTag({ content: sheets.shared })
        await page.evaluate(client)
        await page.evaluate(() => globalThis.activateStylexOrder())
        assert.equal(
          await page
            .locator('button')
            .evaluate((button) => getComputedStyle(button).marginTop),
          '17px',
        )
        await sharedStyle.evaluate((style, css) => {
          style.textContent = css.replaceAll('17px', '23px')
        }, sheets.shared)
        await page.waitForFunction(
          () =>
            getComputedStyle(document.querySelector('button')!).marginTop ===
            '23px',
        )
        // New earlier CSS must not establish a conflicting order ahead of the owner.
        await page.evaluate((css) => {
          const style = document.createElement('style')
          style.textContent = css
          document.head.prepend(style)
        }, sheets.shared)
        await page.waitForFunction(() =>
          document.head.firstElementChild?.hasAttribute(
            'data-bldr-stylex-order',
          ),
        )
        assert.equal(
          await page
            .locator('button')
            .evaluate((button) => getComputedStyle(button).marginTop),
          '23px',
        )
        await page.close()
      } finally {
        await browser.close()
      }
    }
  } finally {
    server.closeAllConnections()
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    )
  }
})
