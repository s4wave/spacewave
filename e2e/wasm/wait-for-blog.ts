// Polls until the blog quickstart finishes navigation to a space route and the
// reading view's Edit posts button is visible.
export default async function (args: { deadlineMs: number }): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const ready = () => {
      const hash = window.location.hash
      if (!hash.includes('/u/') || !hash.includes('/so/')) {
        return false
      }
      return document.querySelector("button[title='Edit posts']") !== null
    }
    const tick = () => {
      if (ready()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        const timing =
          globalThis.__s4waveQuickstartTiming ??
          globalThis.__s4wave_debug?.quickstartTiming ??
          null
        const body =
          document.body?.innerText?.replace(/\s+/g, ' ').slice(0, 240) ?? ''
        reject(
          new Error(
            `blog quickstart did not reach the blog viewer (hash=${window.location.hash}, timing=${JSON.stringify(timing)}, body=${body})`,
          ),
        )
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
