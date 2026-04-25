// Static file server for WebRTC transfer prototype tests.
const port = Number(process.argv[2]) || 40718
const dir = import.meta.dir + '/static'

Bun.serve({
  port,
  async fetch(req) {
    const url = new URL(req.url)
    let path = url.pathname
    if (path === '/') path = '/webrtc-transfer.html'
    const file = Bun.file(dir + path)
    if (!(await file.exists())) {
      return new Response('Not found', { status: 404 })
    }
    const ext = path.split('.').pop() ?? ''
    const types: Record<string, string> = {
      html: 'text/html',
      js: 'application/javascript',
      json: 'application/json',
      css: 'text/css',
    }
    return new Response(file, {
      headers: {
        'Content-Type': types[ext] ?? 'application/octet-stream',
        // Cross-origin isolation not strictly needed for these tests
        // but included for consistency with other prototypes.
        'Cross-Origin-Opener-Policy': 'same-origin',
        'Cross-Origin-Embedder-Policy': 'require-corp',
      },
    })
  },
})
console.log(
  `Serving ${dir} on http://localhost:${port} (cross-origin isolated)`,
)
