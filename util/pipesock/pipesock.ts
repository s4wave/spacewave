import path from 'path'
import net from 'net'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import {
  StreamConn,
  buildPushableSink,
  combineUint8ArrayListTransform,
} from 'starpc'

/**
 * Builds a pipe name for IPC communication
 * @param rootDir - The root directory where the pipe will be created (used for unix sockets)
 * @param pipeUuid - Unique identifier for the pipe
 * @returns The platform-specific pipe path
 */
export function buildPipeName(rootDir: string, pipeUuid: string): string {
  if (process.platform === 'win32') {
    return `\\\\.\\pipe\\bldr\\${pipeUuid}`
  } else {
    // Create a path relative to the current working directory
    const absolutePath = path.join(rootDir, `.pipe-${pipeUuid}`)
    try {
      // Get the relative path from current working directory
      return path.relative(process.cwd(), absolutePath)
    } catch (err) {
      console.warn('Failed to get relative path:', err)
      // Fallback to just the filename if we can't get a relative path
      return `.pipe-${pipeUuid}`
    }
  }
}

/**
 * Socket connection with pushable streams
 */
export interface SocketConnection {
  /** Socket instance */
  socket: net.Socket
  /** Stream for sending data to the socket */
  socketTx: ReturnType<typeof pushable<Uint8Array>>
  /** Stream for receiving data from the socket */
  socketRx: ReturnType<typeof pushable<Uint8Array>>
  /** SRPC stream connection */
  streamConn: StreamConn
}

/**
 * Creates a socket connection with pushable streams
 * @param socket - The socket instance
 * @param streamConn - The SRPC stream connection
 * @returns The socket connection with pushable streams
 */
export function createSocketConnection(
  socket: net.Socket,
  streamConn: StreamConn,
): SocketConnection {
  // Set up bidirectional communication
  const socketTx = pushable<Uint8Array>({ objectMode: true })
  const socketRx = pushable<Uint8Array>({ objectMode: true })

  // Pipe data between socket and SRPC
  pipe(
    socketRx,
    streamConn,
    combineUint8ArrayListTransform(),
    buildPushableSink<Uint8Array>(socketTx),
  ).catch((err) => {
    console.error(`[pipesock] pipe error: ${err}`)
    streamConn.close(err)
  })

  // Handle socket data
  socket.on('data', (data) => {
    socketRx.push(data)
  })

  // Handle socket close
  socket.on('end', () => {
    console.log('[pipesock] socket closed')
    socketRx.end()
    streamConn.close()
  })

  // Handle socket errors
  socket.on('error', (err) => {
    console.error(`[pipesock] socket error: ${err}`)
    socketRx.end(err)
    streamConn.close(err)
  })

  return { socket, socketTx, socketRx, streamConn }
}

/**
 * Starts sending data from the socketTx stream to the socket
 * @param connection - The socket connection
 * @returns A promise that resolves when the sending is complete
 */
export async function startSocketSender(
  connection: SocketConnection,
): Promise<void> {
  try {
    for await (const data of connection.socketTx) {
      connection.socket.write(data)
    }
  } catch (err) {
    console.error(`[pipesock] send error: ${err}`)
    connection.streamConn.close(err as Error)
  }
}

/**
 * Connects to a pipe and sets up bidirectional communication
 * @param ipcPath - The path to the pipe
 * @param streamConn - The SRPC stream connection
 * @param onConnect - Callback function called when the connection is established
 * @returns The socket instance
 */
export function connectToPipe(
  ipcPath: string,
  streamConn: StreamConn,
  onConnect?: (connection: SocketConnection) => void,
): net.Socket {
  const socket = net.connect(ipcPath, () => {
    console.log(`[pipesock] connected to pipe: ${ipcPath}`)

    const connection = createSocketConnection(socket, streamConn)

    // Start sending data to socket
    startSocketSender(connection)

    // Call the onConnect callback if provided
    if (onConnect) {
      onConnect(connection)
    }
  })

  // Handle connection errors
  socket.on('error', (err) => {
    console.error(`[pipesock] connection error: ${err}`)
  })

  return socket
}
