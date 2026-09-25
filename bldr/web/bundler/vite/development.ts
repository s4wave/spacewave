import { EventEmitter } from 'node:events'
import { existsSync, realpathSync } from 'node:fs'
import { createServer as createHTTPServer, type Server } from 'node:http'
import { isAbsolute, relative, resolve, sep } from 'node:path'
import { pushable } from 'it-pushable'
import {
  createServer,
  DevEnvironment,
  version,
  type Alias,
  type HotChannel,
  type HotPayload,
  type Plugin,
  type ViteDevServer,
} from 'vite'

import { Event, SendRequest, Session } from '../../../frontend/frontend.pb.js'
import { DevelopmentConfig, DevelopmentResult } from './vite.pb.js'
import { buildConfig, createSilentViteLogger } from './build.js'
import {
  adaptDevelopmentClient,
  bindDevelopmentImports,
} from './development-client.js'
import { goTsResolver } from './go-ts-resolver.js'
import {
  isWebPkgModule,
  readPackageServedNameMap,
  resolveNodeWebPkgRoot,
} from './plugin.js'

/**
 * excludeExternalAliases keeps aliases from rewriting external packages or
 * their subpaths. Vite applies aliases before any plugin, so an alias that
 * matched an external import would bypass the import map. Each alias becomes a
 * pattern guarded by a lookbehind that inspects the whole import; its match,
 * capture groups, and replacement are unchanged. A string alias keeps its
 * exact-or-subpath matching.
 */
export function excludeExternalAliases(
  aliases: Alias[],
  external: string[],
): Alias[] {
  if (external.length === 0) return aliases
  const pkgs = external.map(escapeRegExp).join('|')
  const guard = `(?<=^(?!(?:${pkgs})(?:/|$))[\\s\\S]*)`
  return aliases.map((alias) => {
    const pattern =
      typeof alias.find === 'string'
        ? `^${escapeRegExp(alias.find)}(?=/|$)`
        : alias.find.source
    const flags = typeof alias.find === 'string' ? '' : alias.find.flags
    return { ...alias, find: new RegExp(`${guard}(?:${pattern})`, flags) }
  })
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** DevelopmentEnvironment retains one Vite graph behind the compiler RPC. */
export class DevelopmentEnvironment {
  private readonly messages = new EventEmitter()
  private readonly clients = new EventEmitter()
  private sequence = 0n
  private server?: ViteDevServer
  private listener?: Server

  /** session is immutable for this environment's lifetime. */
  public readonly session: Session

  constructor(private readonly config: DevelopmentConfig) {
    // Reject malformed namespaces and source attachments at their owner.
    if (!/^[a-zA-Z0-9-]+$/.test(config.sessionId ?? '')) {
      throw new Error('Bldr frontend: invalid session ID')
    }
    for (const entry of config.entrypoints ?? []) {
      const path = relative(
        resolve(config.rootDir!),
        resolve(config.rootDir!, entry),
      )
      if (
        isAbsolute(entry) ||
        !path ||
        path === '..' ||
        path.startsWith('..' + sep)
      ) {
        throw new Error(
          `Bldr frontend: entrypoint escapes the project: ${entry}`,
        )
      }
    }

    // Only Bldr's route namespace crosses into browser-visible source.
    const routePrefix = config.routePrefix || '/b/fe/'
    if (!/^\/b\/fe\/(?:rpc\/[a-zA-Z0-9_-]+\/)?$/.test(routePrefix)) {
      throw new Error('Bldr frontend: invalid route prefix')
    }
    this.session = Session.create({
      id: config.sessionId,
      routePrefix: `${routePrefix}${config.sessionId}/`,
      entrypoints: config.entrypoints,
      viteVersion: version,
    })
  }

  /** start publishes readiness after middleware and dependency preparation. */
  public async start(): Promise<DevelopmentResult> {
    // Load the project's ordinary plugins in an isolated compiler process.
    const root = resolve(this.config.rootDir!)
    const dist = resolve(this.config.distDir!)
    const dependencies = resolve(root, 'node_modules')
    process.env.BLDR_PROJECT_ROOT = root
    process.env.BLDR_DIST_ROOT = dist
    process.env.NODE_ENV = 'development'
    const distSource = existsSync(resolve(dist, 'bldr'))
      ? resolve(dist, 'bldr')
      : dist
    const configPaths = [
      resolve(distSource, 'web/bundler/vite/vite-base.config.ts'),
      ...(this.config.configPaths ?? []).map((value) => resolve(root, value)),
    ]
    if (!this.config.disableProjectConfig) {
      for (const extension of ['ts', 'js', 'mjs', 'cjs']) {
        const path = resolve(root, `vite.config.${extension}`)
        if (existsSync(path) && !configPaths.includes(path))
          configPaths.push(path)
      }
    }
    const config = await buildConfig(
      { command: 'serve', mode: 'development' },
      ...configPaths,
    )
    const webPkgIDs = this.config.webPkgIds ?? []
    const servedNameMaps: Record<string, Map<string, string>> = {}
    for (const pkgID of webPkgIDs) {
      const pkgRoot = resolveNodeWebPkgRoot(pkgID, root)
      if (pkgRoot) servedNameMaps[pkgID] = readPackageServedNameMap(pkgRoot)
    }
    const external = [...(this.config.externalPkgs ?? []), ...webPkgIDs]
    const isExternal = (source: string) =>
      external.some((pkg) => source === pkg || source.startsWith(pkg + '/'))
    const aliases = Array.isArray(config.resolve?.alias)
      ? config.resolve.alias
      : Object.entries(config.resolve?.alias ?? {}).map(
          ([find, replacement]) => ({ find, replacement }),
        )
    const refreshPath = `${this.session.routePrefix}@react-refresh`
    const sessionID = this.session.id!

    // Keep canonical import-map modules external before Vite resolves aliases.
    const adapter: Plugin = {
      name: 'bldr-frontend',
      enforce: 'pre',
      async resolveId(source, importer) {
        if (source === '/@react-refresh')
          return { id: refreshPath, external: true }
        if (!isExternal(source)) return null
        if (isWebPkgModule(source)) return { id: source, external: true }

        // External package aliases still own stylesheet and asset locations.
        // Resolve those files normally, without remapping them to shared JS.
        const alias = aliases.find(({ find }) =>
          typeof find === 'string'
            ? source === find || source.startsWith(find + '/')
            : find.test(source),
        )
        if (alias)
          return this.resolve(
            source.replace(alias.find, alias.replacement),
            importer,
            {
              skipSelf: true,
            },
          )
        return null
      },
      transform(code, id) {
        if (id.split('?')[0].endsWith('/vite/dist/client/client.mjs')) {
          return { code: adaptDevelopmentClient(code, sessionID), map: null }
        }
        return null
      },
    }
    const canonicalImports: Plugin = {
      name: 'bldr-frontend-imports',
      enforce: 'post',
      transform: {
        order: 'post',
        handler: (code) => {
          const bound = bindDevelopmentImports(
            code,
            this.session.routePrefix!,
            external,
            refreshPath,
            webPkgIDs,
            servedNameMaps,
          )
          return bound === code ? null : { code: bound, map: null }
        },
      },
    }
    const channel: HotChannel = {
      send: (payload) => this.publish(payload),
      on: this.clients.on.bind(this.clients),
      off: this.clients.off.bind(this.clients),
    }

    // Vite's listener is private; every browser module crosses Bldr Fetch.
    this.server = await createServer({
      ...config,
      configFile: false,
      root,
      cacheDir: this.config.cacheDir,
      base: this.session.routePrefix,
      appType: 'custom',
      mode: 'development',
      clearScreen: false,
      oxc: config.oxc === false ? false : { target: 'es2022', ...config.oxc },
      customLogger: createSilentViteLogger(),
      plugins: [
        adapter,
        ...(config.plugins ?? []),
        goTsResolver(root, dist),
        canonicalImports,
      ],
      resolve: {
        ...config.resolve,
        alias: excludeExternalAliases(aliases, external),
      },
      optimizeDeps: {
        ...config.optimizeDeps,
        entries: this.config.entrypoints,
        exclude: [
          ...new Set([...(config.optimizeDeps?.exclude ?? []), ...external]),
        ],
      },
      server: {
        middlewareMode: true,
        ws: false,
        hmr: true,
        watch: {
          ...config.server?.watch,
          ignored: config.server?.watch?.ignored ?? [
            '**/.bldr*/**',
            '**/.tmp/**',
            '**/vendor/**',
          ],
        },
        fs: {
          strict: true,
          allow: [
            root,
            dist,
            ...(config.server?.fs?.allow ?? []),
            ...(existsSync(dependencies) ? [realpathSync(dependencies)] : []),
          ],
        },
      },
      environments: {
        client: {
          dev: {
            createEnvironment: (name, resolved) =>
              new DevEnvironment(name, resolved, {
                hot: true,
                transport: channel,
              }),
          },
        },
      },
    })
    const server = this.server
    server.restart = async () => {
      await this.close()
      process.exit(0)
    }
    this.listener = createHTTPServer((request, response) => {
      server.middlewares(request, response, () => {
        response.writeHead(404)
        response.end('Frontend module not found')
      })
    })
    const listener = this.listener
    // Bun can expire keep-alive while a completed write is still draining to
    // Bldr's RPC proxy. The compiler lifetime owns these private connections;
    // close() releases them explicitly after cancellation or replacement.
    listener.keepAliveTimeout = 0
    await new Promise<void>((resolve, reject) => {
      listener.once('error', reject)
      listener.listen(0, '127.0.0.1', () => {
        listener.off('error', reject)
        resolve()
      })
    })
    const address = listener.address()
    if (!address || typeof address === 'string')
      throw new Error('Bldr frontend listener has no TCP address')

    // Dependency preparation is admitted by the native caller, not idle time.
    await server.environments.client.depsOptimizer?.scanProcessing
    const refresh =
      await server.environments.client.pluginContainer.load('/@react-refresh')
    return DevelopmentResult.create({
      session: this.session,
      privateUrl: `http://127.0.0.1:${address.port}`,
      refreshRuntime: typeof refresh === 'string' ? refresh : refresh?.code,
    })
  }

  /** watch emits a snapshot and bounded updates; a gap forces client recovery. */
  public async *watch(signal?: AbortSignal): AsyncGenerator<Event> {
    // Subscribe synchronously before yielding the current sequence.
    const queue = pushable<Event>({ objectMode: true })
    const receive = (event: Event) => {
      if (queue.readableLength >= 64) {
        queue.end(new Error('Bldr frontend update consumer fell behind'))
        return
      }
      queue.push(event)
    }
    const abort = () => queue.end()
    this.messages.on('update', receive)
    this.messages.on('close', abort)
    signal?.addEventListener('abort', abort, { once: true })
    if (signal?.aborted) abort()

    try {
      yield Event.create({ session: this.session, sequence: this.sequence })
      this.clients.emit('connection')
      yield* queue
    } finally {
      this.messages.off('update', receive)
      this.messages.off('close', abort)
      signal?.removeEventListener('abort', abort)
      queue.end()
    }
  }

  /** send accepts upstream custom events only for the current environment. */
  public send(request: SendRequest): void {
    // Vite owns this opaque protocol; narrow its client event at the adapter.
    if (request.sessionId !== this.session.id)
      throw new Error('Bldr frontend session expired')
    const payload: unknown = JSON.parse(request.payload ?? '')
    if (
      !payload ||
      typeof payload !== 'object' ||
      !('type' in payload) ||
      payload.type !== 'custom' ||
      !('event' in payload) ||
      typeof payload.event !== 'string'
    ) {
      throw new Error('Bldr frontend: expected a Vite custom event')
    }
    const data = 'data' in payload ? payload.data : undefined
    this.clients.emit(payload.event, data, {
      send: (reply: HotPayload) => this.publish(reply),
    })
  }

  /** close releases the listener, graph, watchers, and pending subscriptions. */
  public async close(): Promise<void> {
    // Close subscriptions before awaiting Vite's outstanding transforms.
    this.messages.emit('close')
    const listener = this.listener
    this.listener = undefined
    const closed =
      listener &&
      new Promise<void>((resolve, reject) =>
        listener.close((error) => {
          if (
            error &&
            (error as NodeJS.ErrnoException).code !== 'ERR_SERVER_NOT_RUNNING'
          )
            reject(error)
          else resolve()
        }),
      )
    listener?.closeAllConnections()
    await Promise.all([this.server?.close(), closed])
  }

  /** publish assigns ordering at the compiler graph's notification boundary. */
  private publish(payload: HotPayload): void {
    this.sequence++
    this.messages.emit(
      'update',
      Event.create({
        sequence: this.sequence,
        payload: JSON.stringify(payload),
      }),
    )
  }
}
