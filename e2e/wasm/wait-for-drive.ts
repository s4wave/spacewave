// Polls the drive viewer DOM until current demo content
// appears. Uses requestAnimationFrame for frame-synced polling with a deadline.
export default async function (args: { deadlineMs: number }): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const hasFiles = () => {
      const root = document.querySelector('[data-testid="unixfs-browser"]')
      if (!root) {
        return false
      }
      const text = root.textContent ?? ''
      return text.includes('getting-started.md')
    }
    const tick = () => {
      if (hasFiles()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        reject(new Error('drive viewer demo content did not appear'))
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
