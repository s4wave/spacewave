// Polls until the plan selection page is ready for the local path.
export default async function (args: { deadlineMs: number }): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const ready = () => {
      const text = document.body.textContent ?? ''
      return (
        text.includes('Welcome to Spacewave') &&
        text.includes('Start with Cloud') &&
        text.includes('Continue with local storage')
      )
    }
    const tick = () => {
      if (ready()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        reject(new Error('plan page did not appear'))
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
