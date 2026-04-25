const port = Number.parseInt(Bun.argv[2] ?? '', 10) || 40719

Bun.serve({
  port,
  async fetch(req) {
    const url = new URL(req.url)
    let path = url.pathname
    if (path === '/') path = '/index.html'

    const file = Bun.file(`${import.meta.dir}/static${path}`)
    if (!(await file.exists())) {
      return new Response('Not Found', { status: 404 })
    }

    return new Response(file, {
      headers: {
        'Cross-Origin-Opener-Policy': 'same-origin',
        'Cross-Origin-Embedder-Policy': 'require-corp',
      },
    })
  },
})

console.log(`webrtc-bridge server listening on http://localhost:${port}`)
