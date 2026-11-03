import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { test } from 'node:test'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { pathToFileURL } from 'node:url'
import react from '@vitejs/plugin-react'
import { build, createServer, preview } from 'vite'
import { chromium, webkit } from 'playwright'

import { readStylexOrder } from '../app/prerender/stylex-order.js'
import { buildPageHtml } from '../app/prerender/html-template.js'
import { stylexCss } from '../bldr/web/bundler/vite/stylex.js'

// appSource changes both an extracted rule and its compiled class name.
const appSource = (color: string) => `import React,{useState} from 'react';
import * as stylex from '@stylexjs/stylex';
import {metrics} from '@fixture/tokens.stylex.js';
import {ImportedStyle} from './ImportedStyle';
const styles=stylex.create({button:{color:'${color}',marginTop:metrics.space},balanced:{textWrap:'balance'},geometry:(z:number)=>({position:'absolute',zIndex:z,minHeight:{default:null,'@media (pointer: coarse)':'48px','@media (width < 40rem)':'48px'}})});
export function App(){ const [count,setCount]=useState(0);return <><button sx={styles.button} onClick={()=>setCount(count+1)}>{count}<ImportedStyle/></button><div data-geometry className="theme-only" style={{left:31,width:47,height:53}} sx={styles.geometry(7)}/><div data-geometry sx={styles.geometry(8)} className="theme-only" style={{left:37,width:59,height:61}}/><aside className="theme-only">Theme</aside><div className="landing-finish"><p data-balanced sx={styles.balanced}>Balanced public copy</p><p data-pretty>Default public copy</p></div></> }`

test('native CSS ships in the manifest and updates without losing component state', async () => {
  // Use real React, compiler, Vite assets and browsers rather than emulating CSS delivery.
  const repository = process.cwd()
  await mkdir('.bldr', { recursive: true })
  const root = await mkdtemp(resolve('.bldr/stylex-vite-fixture-'))
  await writeFile(
    join(root, 'index.html'),
    '<!doctype html><html><head></head><body><div id="root"></div><script type="module" src="/main.tsx"></script></body></html>',
  )
  await writeFile(
    join(root, 'main.tsx'),
    `import './theme-entry';import React from 'react';import {createRoot} from 'react-dom/client';import {App} from './App';createRoot(document.getElementById('root')!).render(<App/>);`,
  )
  await writeFile(
    join(root, 'tsconfig.json'),
    JSON.stringify({ compilerOptions: { paths: { '@fixture/*': ['./*'] } } }),
  )
  await writeFile(
    join(root, 'tokens.stylex.ts'),
    `import * as stylex from '@stylexjs/stylex';
const identity = <T>(value:T):T => value;
const assertion = <string>'valid';
void identity(assertion);
export const metrics=stylex.defineConsts({space:'17px'});`,
  )

  // An unused token binding still contributes CSS through the explicit theme entry.
  await writeFile(
    join(root, 'ThemeOnly.stylex.ts'),
    `import * as stylex from '@stylexjs/stylex';export const palette=stylex.defineVars({'--theme-only-color':'rgb(2, 3, 4)'});`,
  )
  await writeFile(
    join(root, 'native.css'),
    '.theme-only { color:var(--theme-only-color) }',
  )
  await writeFile(
    join(root, 'theme-entry.ts'),
    `import './ThemeOnly.stylex';import './native.css';import ${JSON.stringify(resolve('web/style/app.ts'))};`,
  )
  await writeFile(join(root, 'App.tsx'), appSource('red'))

  // An intrinsic sx consumer can import compiled styles without importing their runtime.
  await writeFile(
    join(root, 'SharedStyles.ts'),
    `import * as stylex from '@stylexjs/stylex';export const shared=stylex.create({imported:{paddingLeft:19}});`,
  )
  await writeFile(
    join(root, 'ImportedStyle.tsx'),
    `import {shared} from './SharedStyles';export function ImportedStyle(){return <span data-imported-style sx={shared.imported}/>;}`,
  )
  const options = {
    configFile: false as const,
    root,
    plugins: [stylexCss(), react(), stylexCss()],
    logLevel: 'warn' as const,
    build: { manifest: true },
    server: {
      port: 0,
      host: '127.0.0.1',
      fs: { allow: [repository] },
      // Immediate syntax-error corrections must bypass Chokidar's 50 ms change suppression.
      watch: { awaitWriteFinish: { stabilityThreshold: 50, pollInterval: 10 } },
    },
    preview: { port: 0, host: '127.0.0.1' },
  }

  // A StyleX-only module must contribute a real CSS dependency to the build graph.
  try {
    await build(options)
    const manifest = JSON.parse(
      await readFile(join(root, 'dist/.vite/manifest.json'), 'utf8'),
    )
    assert(
      manifest['index.html'].css?.length,
      'StyleX CSS is absent from the manifest',
    )
    // Server and client compilation must agree before any hydration code runs.
    const order = readStylexOrder(
      manifest['index.html'].css.map((file: string) =>
        join(root, 'dist', file),
      ),
    )
    await build({
      ...options,
      build: {
        ssr: join(root, 'App.tsx'),
        outDir: join(root, 'ssr'),
        rolldownOptions: { output: { entryFileNames: 'App.mjs' } },
      },
    })
    const { App } = await import(pathToFileURL(join(root, 'ssr/App.mjs')).href)
    const html = buildPageHtml({
      body: renderToString(createElement(App)),
      title: 'SSR',
      description: 'SSR parity fixture',
      canonicalUrl: 'https://example.test/',
      ogImage: '',
      bootstrapScript: '',
      criticalCss: '',
      stylexOrder: order,
      mainCssUrl: '/' + manifest['index.html'].css[0],
      additionalCssUrls: manifest['index.html'].css
        .slice(1)
        .map((file: string) => '/' + file),
      iconUrl: '',
    })
    await writeFile(join(root, 'dist/ssr.html'), html)
    const production = await preview(options)
    // The preceding production build sets NODE_ENV; dev must start in development.
    process.env.NODE_ENV = 'development'
    const development = await createServer(options)
    assert.equal(development.config.isProduction, false)
    await development.listen()
    try {
      for (const [engine, launcher] of Object.entries({ chromium, webkit })) {
        const browser = await launcher.launch()
        try {
          const staticContext = await browser.newContext({
            javaScriptEnabled: false,
          })
          const staticPage = await staticContext.newPage()
          await staticPage.goto(production.resolvedUrls!.local[0]! + 'ssr.html')
          assert.deepEqual(
            await staticPage
              .locator('button')
              .evaluate((button) => [
                getComputedStyle(button).color,
                getComputedStyle(button).marginTop,
              ]),
            ['rgb(255, 0, 0)', '17px'],
          )
          assert.equal(
            await staticPage
              .locator('aside.theme-only')
              .evaluate((element) => getComputedStyle(element).color),
            'rgb(2, 3, 4)',
            `${engine}: bootstrap markup needs theme-only CSS before hydration`,
          )
          assert.deepEqual(
            await staticPage
              .locator('.landing-finish p')
              .evaluateAll((elements) =>
                elements.map((element) => getComputedStyle(element).textWrap),
              ),
            ['balance', 'pretty'],
            `${engine}: native wrapping must override the landing skin's paragraph default`,
          )
          assert.deepEqual(
            await staticPage
              .locator('[data-geometry]')
              .evaluateAll((elements) =>
                elements.map((element) => {
                  const css = getComputedStyle(element)
                  return [
                    css.left,
                    css.width,
                    css.height,
                    css.zIndex,
                    css.color,
                  ]
                }),
              ),
            [
              ['31px', '47px', '53px', '7', 'rgb(2, 3, 4)'],
              ['37px', '59px', '61px', '8', 'rgb(2, 3, 4)'],
            ],
            `${engine}: intrinsic sx must retain inline lengths, dynamic stacking and opaque skins in either attribute order`,
          )
          await staticContext.close()
          const page = await browser.newPage()
          page.on('pageerror', (error) =>
            console.log(`${engine}: pageerror: ${error.message}`),
          )
          await page.goto(production.resolvedUrls!.local[0]!)
          await page.waitForFunction(
            () =>
              getComputedStyle(document.querySelector('button')!).color ===
              'rgb(255, 0, 0)',
          )
          assert.equal(
            await page
              .locator('[data-imported-style]')
              .evaluate((element) => getComputedStyle(element).paddingLeft),
            '19px',
            `${engine}: an imported style must compile without a local StyleX import`,
          )
          await page.goto(development.resolvedUrls!.local[0]!)
          await page.getByRole('button', { name: '0', exact: true }).click()
          await page.getByRole('button', { name: '1', exact: true }).waitFor()
          await writeFile(join(root, 'App.tsx'), appSource('blue'))
          await page.waitForFunction(
            () =>
              getComputedStyle(document.querySelector('button')!).color ===
              'rgb(0, 0, 255)',
          )
          assert.equal(
            await page.locator('button').textContent(),
            '1',
            `${engine}: CSS edit reloaded the component`,
          )
          await writeFile(
            join(root, 'tokens.stylex.ts'),
            `import * as stylex from '@stylexjs/stylex';export const metrics=stylex.defineConsts({space:'23px'});`,
          )
          await page.waitForFunction(
            () =>
              getComputedStyle(document.querySelector('button')!).marginTop ===
              '23px',
          )
          assert.equal(await page.locator('button').textContent(), '1')
          await writeFile(join(root, 'App.tsx'), 'export function App( {')
          await page.locator('vite-error-overlay').waitFor()
          await writeFile(join(root, 'App.tsx'), appSource('red'))
          await page
            .waitForFunction(
              () =>
                getComputedStyle(document.querySelector('button')!).color ===
                'rgb(255, 0, 0)',
            )
            .catch(async (error) => {
              console.log(
                JSON.stringify(
                  await page.evaluate(() => ({
                    button: document.querySelector('button')?.outerHTML,
                    styles: [...document.querySelectorAll('style')].map(
                      (style) => style.textContent,
                    ),
                    overlay:
                      document.querySelector('vite-error-overlay')?.shadowRoot
                        ?.textContent,
                  })),
                ),
              )
              throw error
            })
          assert.equal(await page.locator('button').textContent(), '1')
          await writeFile(
            join(root, 'tokens.stylex.ts'),
            `import * as stylex from '@stylexjs/stylex';
const identity = <T>(value:T):T => value;
const assertion = <string>'valid';
void identity(assertion);
export const metrics=stylex.defineConsts({space:'17px'});`,
          )
          await page.waitForFunction(
            () =>
              getComputedStyle(document.querySelector('button')!).marginTop ===
              '17px',
          )
          await page.close()
        } finally {
          await browser.close()
        }
      }
    } finally {
      await development.close()
      await new Promise<void>((resolve, reject) =>
        production.httpServer.close((error) =>
          error ? reject(error) : resolve(),
        ),
      )
    }
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
