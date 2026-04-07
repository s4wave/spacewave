// Headless Playwright runner for OPFS benchmarks.
// Usage: bun prototypes/opfs-bench/run.ts
import { chromium } from 'playwright'

const port = 40001
const dir = import.meta.dir + '/static'

// Start server.
const server = Bun.serve({
  port,
  async fetch(req) {
    const url = new URL(req.url)
    let path = url.pathname
    if (path === '/') path = '/index.html'
    const file = Bun.file(dir + path)
    if (!(await file.exists())) {
      return new Response('Not found', { status: 404 })
    }
    const ext = path.split('.').pop() ?? ''
    const types: Record<string, string> = {
      html: 'text/html',
      js: 'application/javascript',
      wasm: 'application/wasm',
    }
    return new Response(file, {
      headers: {
        'Content-Type': types[ext] ?? 'application/octet-stream',
        'Cross-Origin-Opener-Policy': 'same-origin',
        'Cross-Origin-Embedder-Policy': 'require-corp',
      },
    })
  },
})

console.log(`Server on http://localhost:${port}`)

const browser = await chromium.launch({ headless: true })
const context = await browser.newContext()
const page = await context.newPage()

page.on('console', (msg) => {
  const text = msg.text()
  // Print benchmark output lines directly.
  if (text.includes('---') || text.includes('n=') || text.includes('PASS') ||
      text.includes('FAIL') || text.includes('VERDICT') || text.includes('===') ||
      text.includes('SUMMARY') || text.includes('ERROR')) {
    console.log(text)
  }
})

await page.goto(`http://localhost:${port}/`)

// Run all benchmarks.
console.log('Running benchmarks...\n')
await page.evaluate(() => (window as any).runAll())

// Wait for completion: __benchResults is set after runAll().
await page.waitForFunction(() => (window as any).__benchResults, { timeout: 120000 })

// Extract full log text.
const logText = await page.evaluate(() => document.getElementById('log')!.textContent)
console.log('\n' + logText)

await browser.close()
server.stop()
process.exit(0)
