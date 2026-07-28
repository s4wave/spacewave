import { writeSync } from 'node:fs'

interface TuiViewOptions {
  endpoint: string
  pluginId: string
  sessionIndex: number
  sessionObjectKey?: string
  spaceName?: string
  stateStoreId: string
  signal: AbortSignal
  ready(): void
}

type RunTuiView = (options: TuiViewOptions) => Promise<void>

async function main(): Promise<void> {
  const moduleUrl = requiredEnv('SPACEWAVE_TUI_MODULE_URL')
  const exportName = requiredEnv('SPACEWAVE_TUI_EXPORT')
  const readyFd = Number(requiredEnv('SPACEWAVE_TUI_READY_FD'))
  if (!Number.isInteger(readyFd) || readyFd < 3) {
    throw new Error('SPACEWAVE_TUI_READY_FD must name an inherited descriptor')
  }

  // The module is selected per launch, so this host boundary requires a dynamic import.
  const loaded = (await import(moduleUrl)) as Record<string, unknown>
  const candidate = loaded[exportName]
  if (typeof candidate !== 'function') {
    throw new Error(`TuiView module does not export ${exportName}()`)
  }

  const controller = new AbortController()
  const abort = () => controller.abort()
  process.once('SIGINT', abort)
  process.once('SIGTERM', abort)
  let ready = false
  try {
    await (candidate as RunTuiView)({
      endpoint: requiredEnv('SPACEWAVE_TUI_ENDPOINT'),
      pluginId: requiredEnv('SPACEWAVE_TUI_PLUGIN_ID'),
      sessionIndex: parseSessionIndex(requiredEnv('SPACEWAVE_TUI_SESSION_INDEX')),
      sessionObjectKey: optionalEnv('SPACEWAVE_TUI_SESSION_OBJECT_KEY'),
      spaceName: optionalEnv('SPACEWAVE_TUI_SPACE_NAME'),
      stateStoreId: requiredEnv('SPACEWAVE_TUI_STATE_STORE_ID'),
      signal: controller.signal,
      ready: () => {
        if (ready) return
        ready = true
        writeSync(readyFd, 'TUI_READY\n')
      },
    })
    if (!ready && !controller.signal.aborted) {
      throw new Error('TuiView exited before reporting readiness')
    }
  } finally {
    process.off('SIGINT', abort)
    process.off('SIGTERM', abort)
  }
}

function requiredEnv(name: string): string {
  const value = process.env[name]?.trim()
  if (!value) throw new Error(`${name} is required`)
  return value
}

function optionalEnv(name: string): string | undefined {
  const value = process.env[name]?.trim()
  return value || undefined
}

function parseSessionIndex(value: string): number {
  const index = Number(value)
  if (!Number.isSafeInteger(index) || index < 0 || index > 0xffffffff) {
    throw new Error(`invalid SPACEWAVE_TUI_SESSION_INDEX: ${value}`)
  }
  return index
}

void main().catch((error: unknown) => {
  process.stderr.write(
    `${error instanceof Error ? error.message : String(error)}\n`,
  )
  process.exitCode = 1
})
