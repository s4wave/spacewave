// Polls until the local setup banner appears.
export default async function (args: { deadlineMs: number }): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const ready = () => {
      const text = document.body.textContent ?? ''
      return text.includes('Finish setting up your account')
    }
    const tick = () => {
      if (ready()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        reject(new Error('setup banner did not appear'))
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
