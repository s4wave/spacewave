// Polls until the blog quickstart finishes navigation to a space route and the
// reading-mode viewer controls are visible.
export default async function (args: {
  deadlineMs: number
}): Promise<null> {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + args.deadlineMs
    const ready = () => {
      const hash = window.location.hash
      if (!hash.includes('/u/') || !hash.includes('/so/')) {
        return false
      }
      return (
        document.querySelector("button[title='Reading mode']") !== null &&
        document.querySelector("button[title='Editing mode']") !== null
      )
    }
    const tick = () => {
      if (ready()) {
        resolve(null)
        return
      }
      if (Date.now() > deadline) {
        const body =
          document.body?.innerText
            ?.replace(/\s+/g, ' ')
            .slice(0, 240) ?? ''
        reject(
          new Error(
            `blog quickstart did not reach the blog viewer (hash=${window.location.hash}, body=${body})`,
          ),
        )
        return
      }
      requestAnimationFrame(tick)
    }
    tick()
  })
}
