// Static file server with Cross-Origin Isolation headers for OPFS + WebLock.
const port = Number(process.argv[2]) || 40001
const dir = import.meta.dir + '/static'

Bun.serve({
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
      mjs: 'application/javascript',
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
console.log(`Serving ${dir} on http://localhost:${port} (cross-origin isolated)`)
