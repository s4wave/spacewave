export interface ProcessStream {
  on(event: 'error', handler: (err: unknown) => void): void
}

interface ProcessLike {
  stdout?: ProcessStream
  stderr?: ProcessStream
}

let installed = false

export function isClosedProcessStreamError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'code' in err &&
    (err.code === 'EPIPE' || err.code === 'ERR_STREAM_DESTROYED')
  )
}

export function ignoreClosedProcessStreamErrors(proc: ProcessLike = process) {
  if (installed) {
    return
  }

  const handle = (err: unknown) => {
    if (!isClosedProcessStreamError(err)) {
      throw err
    }
  }
  proc.stdout?.on('error', handle)
  proc.stderr?.on('error', handle)
  installed = true
}
