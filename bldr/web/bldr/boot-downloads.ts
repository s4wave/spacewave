// The boot download registry owns per-asset download progress accounting for
// the browser boot sequence. Producers (the boot bootstrap streaming the
// runtime wasm and app bundle, and the plugin module loader) report the
// content length and streamed bytes of each download; the loading screen is a
// plain consumer that renders the watchable snapshot. Progress propagates by a
// dispatched event, never polling, so a fresh listener reads the current array
// and every mutation replaces the array reference for change detection.

export const bootDownloadEvent = 'spacewave-boot-download'

// BootDownloadState is the lifecycle of a single tracked download.
export type BootDownloadState = 'active' | 'complete' | 'error'

// BootDownload is one tracked asset download. total is absent when the byte
// count is not observable (no content-length and no size hint), in which case
// consumers render an indeterminate bar rather than a faked percentage.
export interface BootDownload {
  id: string
  label: string
  loaded: number
  total?: number
  state: BootDownloadState
  error?: string
}

declare global {
  var __swBootDownloads: BootDownload[] | undefined
}

const emptyBootDownloads: BootDownload[] = []

// readBootDownloads returns the current snapshot; the array reference changes on
// every mutation so useSyncExternalStore-style consumers detect updates.
export function readBootDownloads(): BootDownload[] {
  return globalThis.__swBootDownloads ?? emptyBootDownloads
}

function normalizeTotal(total: number | undefined): number | undefined {
  if (total === undefined || !Number.isFinite(total) || total <= 0) {
    return undefined
  }
  return total
}

function contentLengthTotal(response: Response): number | undefined {
  const header = response.headers?.get?.('content-length')
  return normalizeTotal(header === null || header === undefined ? undefined : Number(header))
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function dispatchBootDownloads(): void {
  if (
    typeof globalThis.dispatchEvent === 'function' &&
    typeof globalThis.CustomEvent === 'function'
  ) {
    globalThis.dispatchEvent(
      new CustomEvent(bootDownloadEvent, { detail: readBootDownloads() }),
    )
  }
}

function upsertBootDownload(id: string, patch: Partial<BootDownload>): void {
  const current = readBootDownloads()
  const index = current.findIndex((download) => download.id === id)
  const base: BootDownload =
    index >= 0 ? current[index] : { id, label: id, loaded: 0, state: 'active' }
  const nextEntry: BootDownload = { ...base, ...patch, id }
  globalThis.__swBootDownloads =
    index >= 0
      ? current.map((download, i) => (i === index ? nextEntry : download))
      : [...current, nextEntry]
  dispatchBootDownloads()
}

// beginBootDownload registers a download as active with a known label, resetting
// its byte counter. total is recorded only when a positive size is known.
export function beginBootDownload(
  id: string,
  label: string,
  total?: number,
): void {
  const patch: Partial<BootDownload> = { label, loaded: 0, state: 'active' }
  const positive = normalizeTotal(total)
  if (positive !== undefined) patch.total = positive
  upsertBootDownload(id, patch)
}

// advanceBootDownload records streamed bytes so far. A positive total upgrades a
// previously unknown size (for example a late content-length read).
export function advanceBootDownload(
  id: string,
  loaded: number,
  total?: number,
): void {
  const patch: Partial<BootDownload> = {
    loaded: Math.max(0, loaded),
    state: 'active',
  }
  const positive = normalizeTotal(total)
  if (positive !== undefined) patch.total = positive
  upsertBootDownload(id, patch)
}

// completeBootDownload marks a download done, snapping loaded to total when the
// size was known so the bar lands on a full 100%.
export function completeBootDownload(id: string): void {
  const current = readBootDownloads().find((download) => download.id === id)
  const patch: Partial<BootDownload> = { state: 'complete' }
  if (current?.total !== undefined) patch.loaded = current.total
  upsertBootDownload(id, patch)
}

// failBootDownload marks a download failed with an optional message.
export function failBootDownload(id: string, error?: string): void {
  const patch: Partial<BootDownload> = { state: 'error' }
  if (error !== undefined) patch.error = error
  upsertBootDownload(id, patch)
}

// streamResponseWithBootProgress reads a fetch Response body to completion while
// reporting per-chunk byte progress under id. It is the single owner of the
// getReader accounting loop so producers never hand-roll byte counting; it
// returns the collected chunks for the caller to assemble (Blob, module URL,
// WebAssembly compile, ...). totalHint wins over content-length when a decoded
// size is known ahead of the compressed transfer length.
export async function streamResponseWithBootProgress(
  id: string,
  label: string,
  response: Response,
  totalHint?: number,
): Promise<Uint8Array[]> {
  const total = normalizeTotal(totalHint) ?? contentLengthTotal(response)
  beginBootDownload(id, label, total)
  const parts: Uint8Array[] = []
  let loaded = 0
  const body = response.body
  if (body && typeof body.getReader === 'function') {
    const reader = body.getReader()
    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        if (!value || value.byteLength === 0) continue
        parts.push(value)
        loaded += value.byteLength
        advanceBootDownload(id, loaded, total)
      }
    } catch (error) {
      failBootDownload(id, errorMessage(error))
      throw error
    } finally {
      reader.releaseLock()
    }
  } else {
    const buffer = new Uint8Array(await response.arrayBuffer())
    parts.push(buffer)
    loaded = buffer.byteLength
    advanceBootDownload(id, loaded, total)
  }
  completeBootDownload(id)
  return parts
}

// bootDownloadFraction returns 0..1 when the size is known, else undefined so
// the consumer renders an indeterminate bar instead of a faked percentage.
export function bootDownloadFraction(
  download: BootDownload,
): number | undefined {
  if (download.state === 'complete') return 1
  if (download.total === undefined) return undefined
  return Math.max(0, Math.min(1, download.loaded / download.total))
}

// subscribeBootDownloads invokes callback on every registry change.
export function subscribeBootDownloads(callback: () => void): () => void {
  if (typeof globalThis.addEventListener !== 'function') {
    return () => {}
  }
  globalThis.addEventListener(bootDownloadEvent, callback)
  return () => globalThis.removeEventListener(bootDownloadEvent, callback)
}

// resetBootDownloadsForTest clears the registry between tests.
export function resetBootDownloadsForTest(): void {
  globalThis.__swBootDownloads = undefined
}
