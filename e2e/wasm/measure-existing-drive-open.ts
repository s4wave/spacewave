// Measures a client-side reopen of an existing drive route. The script returns
// timing marks for visible UI layers without reloading the WASM process.
export default async function (args: {
  targetHash: string
  deadlineMs: number
}): Promise<Record<string, number | string | boolean | null>> {
  const start = performance.now()
  const deadline = start + args.deadlineMs
  const out: Record<string, number | string | boolean | null> = {
    completed: false,
    routeMs: null,
    loadingMs: null,
    unixfsShellMs: null,
    unixfsReadyMs: null,
    url: '',
    text: '',
  }

  const elapsed = () => Math.round(performance.now() - start)
  const rootText = () => {
    const root = document.querySelector('[data-testid="unixfs-browser"]')
    return root?.textContent ?? ''
  }
  const hasDriveEntries = (text: string) => text.includes('getting-started.md')

  await new Promise<void>((resolve) => {
    if (window.location.hash === args.targetHash) {
      out.routeMs = elapsed()
      resolve()
      return
    }
    const onHashChange = () => {
      if (window.location.hash !== args.targetHash) {
        return
      }
      window.removeEventListener('hashchange', onHashChange)
      out.routeMs = elapsed()
      resolve()
    }
    window.addEventListener('hashchange', onHashChange)
    window.location.hash = args.targetHash
  })

  return new Promise((resolve, reject) => {
    const tick = () => {
      const root = document.querySelector('[data-testid="unixfs-browser"]')
      const text = rootText()
      if (out.loadingMs === null && text.includes('Loading...')) {
        out.loadingMs = elapsed()
      }
      if (out.unixfsShellMs === null && root) {
        out.unixfsShellMs = elapsed()
      }
      if (hasDriveEntries(text)) {
        out.completed = true
        out.unixfsReadyMs = elapsed()
        out.url = window.location.href
        out.text = text
        resolve(out)
        return
      }
      if (performance.now() > deadline) {
        out.url = window.location.href
        out.text = text
        reject(
          new Error(
            `existing drive did not become ready: ${JSON.stringify(out)}`,
          ),
        )
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
