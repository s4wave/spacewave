// The Terminal viewer's handle, state watch and connection, as drawn.

type Terminal = {
  handle: string
  watch: string
  conn: string
}

export const title = 'Terminal viewer: handle, state watch and connection'

export const state = (v: Terminal) => {
  // The handle's status takes the body until it opens.
  if (v.handle === 'waiting') return 'Opening the terminal'
  if (v.handle === 'failed') return 'Could not open, Retry shown'
  const notice = v.watch === 'failed' ? ', state notice with Retry' : ''
  const card =
    v.conn === 'active'
      ? 'Screen live'
      : v.conn === 'connecting'
        ? 'Screen connecting'
        : v.conn === 'closed'
          ? 'Screen closed, Retry shown'
          : 'Screen failed, Retry shown'
  return card + notice
}

export const step = (action: string, from: Terminal, to: Terminal) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'HandleOpen') return 'handle opens'
  if (action === 'HandleFail') return 'handle fails'
  if (action === 'RetryHandle') return 'presses Retry'
  if (action === 'WatchFail') return 'state watch fails'
  if (action === 'RetryWatch') return 'retries the state'
  if (action === 'ConnStep') return 'connection changes'
  if (action === 'RetryConn') return 'retries the screen'
  return null
}
