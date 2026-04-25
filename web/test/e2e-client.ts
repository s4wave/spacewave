/**
 * E2E test client for connecting to the Go backend via WebSocket.
 * Used by browser E2E tests to establish RPC connections.
 */

import { pipe } from 'it-pipe'
import { duplex } from '@aptre/it-ws'
import {
  Client,
  StreamConn,
  combineUint8ArrayListTransform,
  type OpenStreamFunc,
} from 'starpc'
import { LayoutHostClient } from '@s4wave/sdk/layout/layout_srpc.pb.js'
import type { LayoutHost } from '@s4wave/sdk/layout/layout_srpc.pb.js'
// Minimal WebSocket-like type used for E2E tests that run in browser and
// node environments. This intentionally only includes the members we need.
// Keep this small to avoid coupling to the `ws` type definitions.
export type TestWebSocket = {
  onopen?: (() => void) | null
  onerror?: ((ev: Event | ErrorEvent) => void) | null
  onclose?: ((ev: CloseEvent) => void) | null
  close?: () => void
  readyState?: number
}

/**
 * E2ETestClient provides a connection to the Go test server.
 */
export class E2ETestClient {
  // The E2E client runs in both browser (DOM WebSocket) and Node (ws).
  // Keep ws typed minimally to avoid coupling to a specific WebSocket lib.
  private ws: TestWebSocket | null = null
  private conn: StreamConn | null = null
  private client: Client | null = null
  private layoutHost: LayoutHost | null = null

  /**
   * Connects to the test server at the given URL.
   * @param url WebSocket URL (e.g., ws://localhost:12345/ws)
   */
  async connect(url: string): Promise<void> {
    if (this.ws) {
      throw new Error('Already connected')
    }

    // Create a WebSocket/compatible instance for the environment.
    let ws: TestWebSocket
    const GlobalWS = (
      globalThis as { WebSocket?: new (url: string) => TestWebSocket }
    ).WebSocket
    if (GlobalWS && typeof GlobalWS === 'function') {
      // Browser environment.

      ws = new GlobalWS(url)
    } else {
      // Node environment - dynamically import 'ws' to avoid require() style import.
      // Use dynamic import so TypeScript won't statically bind to Node-only types.

      const wsMod = await import('ws')
      // eslint-disable-next-line @typescript-eslint/no-explicit-any, @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-member-access
      const WS = (wsMod as any).default ?? (wsMod as any)
      // eslint-disable-next-line @typescript-eslint/no-unsafe-call
      ws = new WS(url) as TestWebSocket
    }

    // Wait for socket to open or error. Use defensive handlers compatible with
    // both DOM WebSocket and Node 'ws' implementations.
    await new Promise<void>((resolve, reject) => {
      if (ws.readyState === 1) {
        resolve()
        return
      }

      const cleanup = () => {
        try {
          ws.onopen = null
          ws.onerror = null
          ws.onclose = null
        } catch {
          // ignore
        }
      }

      const onOpen = () => {
        cleanup()
        resolve()
      }
      const onError = (ev: Event | ErrorEvent) => {
        cleanup()
        console.error('WebSocket onerror event:', ev)
        reject(new Error('WebSocket error'))
      }
      const onClose = (ev: CloseEvent) => {
        cleanup()
        reject(new Error(`WebSocket closed: ${ev?.reason ?? ''}`))
      }

      ws.onopen = onOpen
      ws.onerror = onError
      ws.onclose = onClose
    })

    // Keep this.ws typed to the minimal TestWebSocket.
    this.ws = ws

    // Create stream connection using a duplex wrapper. When possible, pass
    // the original runtime WebSocket; otherwise, cast to any so the runtime
    // wrapper from `it-ws` can adapt it.
    // eslint-disable-next-line @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-explicit-any
    const wsDuplex = duplex(ws as any)

    this.conn = new StreamConn(undefined, {
      direction: 'outbound',
      yamuxParams: {
        enableKeepAlive: false,
        maxMessageSize: 32 * 1024,
      },
    })

    // Pipe WebSocket through the connection
    pipe(wsDuplex, this.conn, combineUint8ArrayListTransform(), wsDuplex).catch(
      (err) => {
        console.error('E2E client pipe error:', err)
        const closeErr = err instanceof Error ? err : new Error(String(err))
        this.conn?.close(closeErr)
      },
    )

    // Create RPC client
    this.client = new Client(this.conn.buildOpenStreamFunc())

    // Create LayoutHost client
    this.layoutHost = new LayoutHostClient(this.client)
  }

  /**
   * Returns the LayoutHost RPC client.
   * Must be connected first.
   */
  getLayoutHost(): LayoutHost {
    if (!this.layoutHost) {
      throw new Error('Not connected')
    }
    return this.layoutHost
  }

  /**
   * Returns the raw starpc Client.
   * Must be connected first.
   */
  getClient(): Client {
    if (!this.client) {
      throw new Error('Not connected')
    }
    return this.client
  }

  /**
   * Returns the OpenStreamFunc for creating RPC streams.
   * Must be connected first.
   */
  getOpenStreamFunc(): OpenStreamFunc {
    if (!this.conn) {
      throw new Error('Not connected')
    }
    return this.conn.buildOpenStreamFunc()
  }

  /**
   * Disconnects from the server.
   */
  disconnect(): void {
    if (this.conn) {
      this.conn.close()
      this.conn = null
    }
    if (this.ws) {
      this.ws.close?.()
      this.ws = null
    }
    this.client = null
    this.layoutHost = null
  }

  /**
   * Returns true if connected.
   */
  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN
  }
}

/**
 * Creates and connects an E2E test client.
 * @param port The port number of the test server
 */
export async function createE2EClient(port: number): Promise<E2ETestClient> {
  const client = new E2ETestClient()
  await client.connect(`ws://localhost:${port}/ws`)
  return client
}

/**
 * Gets the test server port from environment variable.
 * This should be set by the test harness.
 */
export function getTestServerPort(): number {
  const port = import.meta.env.VITE_E2E_SERVER_PORT
  if (!port) {
    throw new Error(
      'VITE_E2E_SERVER_PORT environment variable not set. ' +
        'Make sure the Go test server is running.',
    )
  }
  return parseInt(port, 10)
}
