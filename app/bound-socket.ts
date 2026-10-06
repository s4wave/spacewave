import { WebSocketConn, type OpenStreamFunc } from 'starpc'

// openBoundSocketStream opens streams over a websocket to url. It dials on
// the first stream and again after the socket closes, so the Resource client
// reconnects when the daemon restarts.
export function openBoundSocketStream(url: string): OpenStreamFunc {
  let conn: Promise<WebSocketConn> | null = null
  return async () => {
    conn ??= dialBoundSocket(url).then(
      (next) => {
        next.getSocket().addEventListener('close', () => {
          conn = null
        })
        return next
      },
      (err: unknown) => {
        conn = null
        throw err
      },
    )
    return (await conn).openStream()
  }
}

// dialBoundSocket resolves once the websocket to url is open.
async function dialBoundSocket(url: string): Promise<WebSocketConn> {
  const socket = new WebSocket(url)
  socket.binaryType = 'arraybuffer'
  await new Promise<void>((resolve, reject) => {
    socket.addEventListener('open', () => resolve(), { once: true })
    socket.addEventListener(
      'error',
      () => reject(new Error('Spacewave connection failed')),
      { once: true },
    )
  })
  // The adapter uses the shared WHATWG methods; its dependency types name ws.
  return new WebSocketConn(
    socket as unknown as ConstructorParameters<typeof WebSocketConn>[0],
    'outbound',
  )
}
