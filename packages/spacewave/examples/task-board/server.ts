import { readFile } from 'node:fs/promises'
import { createServer as createHTTPServer } from 'node:http'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'
import { SyncError } from 'spacewave'
import { createServer } from 'spacewave/server'

import { schema } from './schema.ts'

// Build both browser entry points before accepting page requests.
const directory = dirname(fileURLToPath(import.meta.url))
await build({
  absWorkingDir: directory,
  entryPoints: ['vanilla.ts', 'react.tsx'],
  outdir: 'public',
  bundle: true,
  format: 'esm',
  platform: 'browser',
  target: 'es2024',
  define: { 'process.env.NODE_ENV': '"production"' },
})

// Open the persistent task dataset and admit only the local demo identity.
const server = await createServer({
  directory: process.env.DATA_DIRECTORY ?? join(directory, '.data'),
  schema,

  // Replace this verifier with the application's real authentication provider.
  authenticate: async (token) => {
    if (token !== 'local-demo') {
      throw new SyncError('AUTHENTICATION', 'Sign in again')
    }
    return { subject: 'demo-user', scope: 'demo' }
  },
  authorize: ({ scope }) => scope === 'demo',

  // Mutations read and write through one server transaction.
  mutations: {
    completeTodo: async (context, { id }) => {
      // Read the task before changing its completion state.
      const todos = context.collection('todos')
      const todo = await todos.get(id)
      if (!todo) {
        throw new SyncError('VALIDATION', 'This task no longer exists')
      }

      // Update the task within this transaction and return its value.
      const updated = { ...todo, done: true }
      await todos.put(id, updated)
      return updated
    },
  },
})

// Seed the board through the same authorized collection API used by clients.
const tasks = server
  .as({ subject: 'demo-user', scope: 'demo' })
  .collection('todos')
if (!(await tasks.get('welcome'))) {
  await tasks.put('welcome', {
    title: 'Open this board in two tabs',
    done: false,
  })
}

// Serve only the example's page, bundle, and stylesheet routes.
const files = new Map([
  ['/', ['vanilla.html', 'text/html']],
  ['/react', ['react.html', 'text/html']],
  ['/vanilla.js', ['public/vanilla.js', 'text/javascript']],
  ['/react.js', ['public/react.js', 'text/javascript']],
  ['/style.css', ['style.css', 'text/css']],
])
const http = createHTTPServer((request, response) => {
  // Resolve the request through the fixed route map.
  const path = new URL(request.url ?? '/', 'http://localhost').pathname
  const file = files.get(path)
  if (!file) {
    response.writeHead(404)
    response.end()
    return
  }

  // Send the matching asset or report a read failure.
  const [filename, contentType] = file
  void readFile(join(directory, filename)).then(
    (data) => {
      response.setHeader('Content-Type', contentType)
      response.end(data)
    },
    () => {
      response.writeHead(500)
      response.end('The page could not load')
    },
  )
})

// Share the HTTP listener between pages and the sync endpoint.
server.attach(http)
const port = Number(process.env.PORT ?? 8787)
await new Promise<void>((resolve, reject) => {
  http.once('error', reject)
  http.listen(port, '127.0.0.1', resolve)
})
const address = http.address()
if (address && typeof address !== 'string') {
  console.log(`Task board: http://127.0.0.1:${address.port}`)
}

// Repeated shutdown signals share one close operation.
let closing: Promise<void> | undefined

/** close releases sync connections and storage, then the supplied HTTP server. */
function close(): Promise<void> {
  closing ??= (async () => {
    await server.close()
    await new Promise<void>((resolve, reject) =>
      http.close((error) => (error ? reject(error) : resolve())),
    )
  })()
  return closing
}

// Let both normal interruption and service termination finish cleanup.
for (const signal of ['SIGINT', 'SIGTERM'] as const) {
  process.once(signal, () => {
    void close().catch((error: unknown) => {
      console.error(error)
      process.exitCode = 1
    })
  })
}
