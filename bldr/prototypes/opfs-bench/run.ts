import { mkdir } from 'node:fs/promises'
import path from 'node:path'

import { chromium, firefox, webkit } from 'playwright'

const dir = import.meta.dir + '/static'
const port = 30000 + Math.floor(Math.random() * 20000)

const server = Bun.serve({
  port,
  async fetch(req) {
    const url = new URL(req.url)
    let filePath = url.pathname
    if (filePath === '/') {
      filePath = '/index.html'
    }
    const file = Bun.file(dir + filePath)
    if (!(await file.exists())) {
      return new Response('Not found', { status: 404 })
    }
    const ext = filePath.split('.').pop() ?? ''
    const types: Record<string, string> = {
      html: 'text/html',
      js: 'application/javascript',
      wasm: 'application/wasm',
      json: 'application/json',
      css: 'text/css',
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

type BrowserPlan = {
  browserName: string
  launch: typeof chromium
  suite: 'full' | 'smoke'
}

const browserPlans: BrowserPlan[] = [
  { browserName: 'chromium', launch: chromium, suite: 'full' },
  { browserName: 'firefox', launch: firefox, suite: 'smoke' },
  { browserName: 'webkit', launch: webkit, suite: 'smoke' },
]

async function runBrowser(plan: BrowserPlan) {
  console.log(`\n=== ${plan.browserName} (${plan.suite}) ===`)
  let browser
  try {
    browser = await plan.launch.launch({ headless: true })
  } catch (err) {
    return {
      browserName: plan.browserName,
      suite: plan.suite,
      ok: false,
      error: err instanceof Error ? err.message : String(err),
    }
  }

  try {
    const context = await browser.newContext()
    const page = await context.newPage()
    const timeoutMs = plan.suite === 'full' ? 120000 : 45000
    page.on('console', (msg) => {
      const text = msg.text()
      if (!text.trim()) {
        return
      }
      console.log(`[${plan.browserName}] ${text}`)
    })
    await page.goto(`http://localhost:${server.port}/`, { timeout: timeoutMs })
    const results = await Promise.race([
      page.evaluate((suite) => (window as any).runAll(suite), plan.suite),
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error(`benchmark timed out after ${timeoutMs}ms`)), timeoutMs),
      ),
    ])
    const logText = await page.evaluate(() => document.getElementById('log')?.textContent ?? '')
    console.log(
      `[${plan.browserName}] done in ${results?.meta?.elapsedMs?.toFixed?.(1) ?? 'n/a'}ms`,
    )
    return {
      browserName: plan.browserName,
      suite: plan.suite,
      ok: true,
      logText,
      results,
    }
  } catch (err) {
    return {
      browserName: plan.browserName,
      suite: plan.suite,
      ok: false,
      error: err instanceof Error ? err.stack ?? err.message : String(err),
    }
  } finally {
    await browser.close()
  }
}

console.log(`Server on http://localhost:${server.port}`)

const aggregate = {
  startedAt: new Date().toISOString(),
  host: `http://localhost:${server.port}/`,
  browsers: [] as Awaited<ReturnType<typeof runBrowser>>[],
}

for (const plan of browserPlans) {
  aggregate.browsers.push(await runBrowser(plan))
}

aggregate['finishedAt'] = new Date().toISOString()

const outDir = path.join(import.meta.dir, '..', '..', '.tmp')
await mkdir(outDir, { recursive: true })
const outPath = path.join(
  outDir,
  `opfs-bench-results-${aggregate.startedAt.replaceAll(':', '').replaceAll('.', '-')}.json`,
)
await Bun.write(outPath, JSON.stringify(aggregate, null, 2))

console.log(`\nResults written to ${outPath}`)

server.stop()
