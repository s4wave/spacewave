// Polls the forge viewer DOM until entity type counts (CLUSTER/Cluster) appear.
// Uses requestAnimationFrame for frame-synced polling with a deadline.
export default async function (args: {
  deadlineMs: number
}): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const ready = () => {
      const viewer = document.querySelector('[data-testid="forge-viewer"]')
      if (!viewer) return false
      const text = viewer.textContent ?? ''
      return text.includes('CLUSTER') || text.includes('Cluster')
    }
    const tick = () => {
      if (ready()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        reject(new Error('forge dashboard did not show entities'))
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
